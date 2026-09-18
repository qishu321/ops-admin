package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"ops-admin/backend/model"
)

const maxAssetTerminalUploadSize int64 = 500 * 1024 * 1024

type AssetTerminalDownload struct {
	Client  io.Closer
	Session interface {
		Close() error
		Wait() error
	}
	Reader   io.Reader
	Filename string
}

// AssetTerminalFile is deliberately limited to regular files and directories.
// It is the data model used by the web-terminal file browser.
type AssetTerminalFile struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updatedAt"`
}

// UploadAssetTerminalFile copies one browser-uploaded file into a host over a
// separate SSH channel. It deliberately avoids rz/sz so the interactive PTY
// remains intact, and publishes only after a successful temporary-file rename.
func (s *Service) UploadAssetTerminalFile(hostID uint, directory, filename string, size int64, content io.Reader) (map[string]any, error) {
	directory = strings.TrimSpace(directory)
	filename = strings.TrimSpace(filename)
	if hostID == 0 || directory == "" {
		return nil, errors.New("invalid host upload target")
	}
	if filename == "" || filename == "." || filename == ".." || path.Base(filename) != filename || strings.ContainsAny(filename, "\\\x00\r\n") {
		return nil, errors.New("invalid upload file name")
	}
	if size < 0 || size > maxAssetTerminalUploadSize {
		return nil, errors.New("单个文件不能超过 500 MB")
	}

	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, hostID).Error; err != nil {
		return nil, err
	}
	client, err := s.newSSHClientWithTimeout(host, 20*time.Second)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Expand a prompt-style ~ directory in the remote shell, then write to a
	// hidden sibling file and rename it only after stdin reaches EOF.
	script := `set -eu; dir="$1"; name="$2"; case "$dir" in '~') dir="$HOME" ;; '~/'*) dir="$HOME/${dir#\~/}" ;; esac; test -d "$dir"; test -w "$dir"; tmp="$dir/.${name}.ops-upload-$$"; trap 'rm -f "$tmp"' EXIT HUP INT TERM; cat > "$tmp"; mv -f "$tmp" "$dir/$name"; trap - EXIT; printf '%s' "$dir/$name"`
	command := "sh -c " + shellQuote(script) + " -- " + shellQuote(directory) + " " + shellQuote(filename)
	if err := session.Start(command); err != nil {
		return nil, err
	}
	if _, err := io.Copy(stdin, io.LimitReader(content, maxAssetTerminalUploadSize+1)); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	// ssh.Session.StdinPipe may report io.EOF when the remote `cat` has already
	// consumed stdin and exited. That is the expected completion path, not an
	// upload failure.
	if err := stdin.Close(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err := session.Wait(); err != nil && !errors.Is(err, io.EOF) {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("上传到主机失败：%s", message)
		}
		return nil, err
	}
	destination := strings.TrimSpace(stdout.String())
	return map[string]any{"hostID": hostID, "filename": filename, "directory": directory, "destination": destination, "size": size}, nil
}

func (s *Service) OpenAssetTerminalFileDownload(hostID uint, remotePath string) (*AssetTerminalDownload, error) {
	remotePath = strings.TrimSpace(remotePath)
	if hostID == 0 || remotePath == "" || strings.ContainsAny(remotePath, "\x00\r\n") {
		return nil, errors.New("invalid host download target")
	}
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, hostID).Error; err != nil {
		return nil, err
	}
	client, err := s.newSSHClientWithTimeout(host, 20*time.Second)
	if err != nil {
		return nil, err
	}
	// Validate and canonicalise the target before HTTP headers are written. A
	// missing file used to fail only after the browser had received attachment
	// headers, which made Chrome save a misleading 0 B file.
	resolvedPath, err := resolveAssetTerminalFile(client, remotePath)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr
	script := `set -eu; test -f "$1"; cat "$1"`
	if err := session.Start("sh -c " + shellQuote(script) + " -- " + shellQuote(resolvedPath)); err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, err
	}
	return &AssetTerminalDownload{Client: client, Session: session, Reader: stdout, Filename: path.Base(resolvedPath)}, nil
}

// ListAssetTerminalFiles reads exactly one directory on the remote host. The
// server canonicalises the directory, so navigating to ".." never depends on
// parsing the interactive shell prompt in the browser.
func (s *Service) ListAssetTerminalFiles(hostID uint, directory string) (map[string]any, error) {
	directory = strings.TrimSpace(directory)
	if hostID == 0 || directory == "" || strings.ContainsAny(directory, "\x00\r\n") {
		return nil, errors.New("invalid host directory")
	}
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, hostID).Error; err != nil {
		return nil, err
	}
	client, err := s.newSSHClientWithTimeout(host, 20*time.Second)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	resolvedDirectory, err := resolveAssetTerminalDirectory(client, directory)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	// The separator is only a transport format; unusual file names containing a
	// tab or newline are skipped instead of producing an unsafe browser entry.
	script := `set -eu; find "$1" -mindepth 1 -maxdepth 1 \( -type d -o -type f \) -printf '%f\t%y\t%s\t%TY-%Tm-%Td %TH:%TM\n'`
	if err := session.Run("sh -c " + shellQuote(script) + " -- " + shellQuote(resolvedDirectory)); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("读取远端目录失败：%s", message)
		}
		return nil, err
	}
	items := make([]AssetTerminalFile, 0)
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 || parts[0] == "" || strings.ContainsAny(parts[0], "\t\r\n") {
			continue
		}
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		items = append(items, AssetTerminalFile{Name: parts[0], Path: path.Join(resolvedDirectory, parts[0]), Directory: parts[1] == "d", Size: size, UpdatedAt: parts[3]})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Directory != items[j].Directory {
			return items[i].Directory
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	parent := path.Dir(resolvedDirectory)
	if resolvedDirectory == "/" {
		parent = ""
	}
	return map[string]any{"path": resolvedDirectory, "parent": parent, "items": items}, nil
}

func resolveAssetTerminalDirectory(client *ssh.Client, directory string) (string, error) {
	script := `set -eu; dir="$1"; case "$dir" in '~') dir="$HOME" ;; '~/'*) dir="$HOME/${dir#\~/}" ;; esac; cd "$dir"; pwd -P`
	return runAssetTerminalPathCommand(client, script, directory)
}

func resolveAssetTerminalFile(client *ssh.Client, remotePath string) (string, error) {
	script := `set -eu; file="$1"; case "$file" in '~') file="$HOME" ;; '~/'*) file="$HOME/${file#\~/}" ;; esac; test -f "$file"; cd "$(dirname "$file")"; printf '%s/%s' "$(pwd -P)" "$(basename "$file")"`
	return runAssetTerminalPathCommand(client, script, remotePath)
}

func runAssetTerminalPathCommand(client *ssh.Client, script, input string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run("sh -c " + shellQuote(script) + " -- " + shellQuote(input)); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", fmt.Errorf("远端路径不可用：%s", message)
		}
		return "", errors.New("远端路径不存在、不可访问或类型不匹配")
	}
	resolved := strings.TrimSpace(stdout.String())
	if resolved == "" || !strings.HasPrefix(resolved, "/") {
		return "", errors.New("无法解析远端路径")
	}
	return resolved, nil
}
