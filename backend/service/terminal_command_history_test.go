package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"ops-admin/backend/model"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const historyPrompt = "testvm@u-057:~/tmp$ "

func historyFilter() *TerminalCommandFilter {
	return &TerminalCommandFilter{check: func(command string) error {
		return matchCommandFilters([]model.CommandFilter{{Name: "删除保护", MatchType: "keyword", Pattern: "rm、mkfs", Scopes: []string{"terminal"}, Enabled: true}}, "terminal", command)
	}}
}
func historyPromptReady(f *TerminalCommandFilter) {
	f.ObserveOutput([]byte("\r\n\x1b[32mtestvm@u-057\x1b[0m:~/tmp$ \x1b[?2004h"))
}
func TestSSHHistoryUsesRemoteEchoInsteadOfArrowBytes(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	out, notices := f.Feed([]byte("\x1b[A"))
	if string(out) != "\x1b[A" || len(notices) != 0 {
		t.Fatal("history key itself was blocked")
	}
	f.ObserveOutput([]byte("rm -f 111.txt"))
	out, notices = f.Feed([]byte("\r"))
	if len(notices) != 1 || string(out) != "\x03" {
		t.Fatalf("dangerous history executed: %q %v", out, notices)
	}
	historyPromptReady(f)
	f.Feed([]byte("\x1b[A"))
	f.ObserveOutput([]byte("history"))
	out, notices = f.Feed([]byte("\r"))
	if len(notices) != 0 || string(out) != "\r" {
		t.Fatalf("safe history rejected: %q %v", out, notices)
	}
}
func TestSSHHistoryDownAndEditedRecallReplaceStaleCommand(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	f.Feed([]byte("\x1b[A"))
	f.ObserveOutput([]byte("rm -rf 111.txt/"))
	f.Feed([]byte("\x1b[B"))
	f.ObserveOutput([]byte("\r" + historyPrompt + "ls\x1b[K"))
	out, notices := f.Feed([]byte("\r"))
	if string(out) != "\r" || len(notices) != 0 {
		t.Fatalf("old history poisoned safe selection: %q %v", out, notices)
	}
	historyPromptReady(f)
	f.Feed([]byte("\x1b[A"))
	f.ObserveOutput([]byte("echo -f 111.txt"))
	f.Feed([]byte("\x01\x1b[3~\x1b[3~\x1b[3~\x1b[3~rm"))
	out, notices = f.Feed([]byte("\r"))
	if len(notices) != 1 {
		t.Fatalf("editing recalled command bypassed filter: %q %v", out, notices)
	}
}
func TestSSHCompletionRemainsUsableAndMatchesCompletedCommand(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	f.Feed([]byte("r"))
	f.ObserveOutput([]byte("r"))
	f.Feed([]byte("\t"))
	f.ObserveOutput([]byte("m "))
	out, notices := f.Feed([]byte("-f 111.txt\r"))
	if len(notices) != 1 || strings.Contains(string(out), "\r") {
		t.Fatalf("completed command escaped: %q %v", out, notices)
	}
	historyPromptReady(f)
	f.Feed([]byte("cd tm"))
	f.ObserveOutput([]byte("cd tm"))
	f.Feed([]byte("\t"))
	f.ObserveOutput([]byte("p/"))
	out, notices = f.Feed([]byte("\r"))
	if string(out) != "\r" || len(notices) != 0 {
		t.Fatalf("safe completion broken: %q %v", out, notices)
	}
}
func TestSSHHistoryEchoPreservesSplitUTF8AndReadlineErase(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	f.Feed([]byte("\x1b[A"))
	for _, b := range []byte("\r\x1b[K" + historyPrompt + "rm -f 中文.txt") {
		f.ObserveOutput([]byte{b})
	}
	out, notices := f.Feed([]byte("\r"))
	if len(notices) != 1 || string(out) != "\x03" {
		t.Fatalf("split output lost recalled command: %q %v", out, notices)
	}
}

type terminalInputWriterFunc func([]byte) (int, error)

func (fn terminalInputWriterFunc) Write(p []byte) (int, error) { return fn(p) }
func TestSSHArrowAndEnterInSameFrameWaitForEchoBeforeSubmitting(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	var wire strings.Builder
	finished := make(chan struct{})
	writer := terminalInputWriterFunc(func(data []byte) (int, error) {
		wire.Write(data)
		if string(data) == "\x1b[A" {
			go func() { time.Sleep(10 * time.Millisecond); f.ObserveOutput([]byte("rm -r 111.txt/")); close(finished) }()
		}
		return len(data), nil
	})
	notices, err := f.WriteInput([]byte("\x1b[A\r"), writer)
	<-finished
	if err != nil || len(notices) != 1 || strings.Contains(wire.String(), "\r") {
		t.Fatalf("Enter reached SSH before history check: %q %v %v", wire.String(), notices, err)
	}
}

// Exercise real encrypted SSH stdin/stdout transport. The server is a fake
// Readline shell and never executes a host command or deletes a file.
func TestSSHHistoryFilterWithLiveSSHTransport(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	executed := make(chan string, 4)
	ready := make(chan struct{})
	var recalled atomic.Value
	recalled.Store("rm -f 111.txt")
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for channel := range channels {
			stream, requests, err := channel.Accept()
			if err != nil {
				return
			}
			go func() {
				for request := range requests {
					request.Reply(true, nil)
					if request.Type == "shell" {
						io.WriteString(stream, historyPrompt)
					}
				}
			}()
			var line, escape string
			buf := make([]byte, 4096)
			for {
				n, err := stream.Read(buf)
				if err != nil {
					return
				}
				for _, b := range buf[:n] {
					if escape != "" {
						escape += string(b)
						if escape == "\x1b[A" {
							line = recalled.Load().(string)
							io.WriteString(stream, line)
							escape = ""
						}
						continue
					}
					switch b {
					case 27:
						escape = "\x1b"
					case 3:
						line = ""
						io.WriteString(stream, "^C\r\n"+historyPrompt)
					case '\r', '\n':
						executed <- line
						line = ""
						io.WriteString(stream, "\r\n"+historyPrompt)
					default:
						line += string(b)
						stream.Write([]byte{b})
					}
				}
			}
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()
	filter := historyFilter()
	go func() {
		buf := make([]byte, 4096)
		once := false
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				filter.ObserveOutput(buf[:n])
				if !once {
					close(ready)
					once = true
				}
			}
			if err != nil {
				return
			}
		}
	}()
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("SSH prompt timeout")
	}
	notices, err := filter.WriteInput([]byte("\x1b[A\r"), stdin)
	if err != nil || len(notices) != 1 {
		t.Fatalf("remote history not blocked: %v %v", notices, err)
	}
	select {
	case command := <-executed:
		t.Fatalf("forbidden command reached shell submission: %q", command)
	case <-time.After(30 * time.Millisecond):
	}
	recalled.Store("ls -la")
	notices, err = filter.WriteInput([]byte("\x1b[A\r"), stdin)
	if err != nil || len(notices) != 0 {
		t.Fatalf("safe history blocked: %v %v", notices, err)
	}
	select {
	case command := <-executed:
		if command != "ls -la" {
			t.Fatalf("wrong history executed: %q", command)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("safe history was not submitted")
	}
}
