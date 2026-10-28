package service

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

const aliyunKubernetesPackages = "https://mirrors.aliyun.com/kubernetes-new/core/stable/"

func deployAliyunAPTSourceScript() string {
	type profile struct{ os, version, arch, codename, base, components, keyring string }
	profiles := []profile{}
	for _, osVersion := range []struct{ version, codename string }{{"22.04", "jammy"}, {"24.04", "noble"}} {
		for _, arch := range []string{"x86_64", "aarch64"} {
			base := "ubuntu"
			if arch == "aarch64" {
				base = "ubuntu-ports"
			}
			profiles = append(profiles, profile{"ubuntu", osVersion.version, arch, osVersion.codename, base, "main restricted universe multiverse", "ubuntu-archive-keyring.gpg"})
		}
	}
	for _, osVersion := range []struct{ version, codename string }{{"12", "bookworm"}, {"13", "trixie"}} {
		for _, arch := range []string{"x86_64", "aarch64"} {
			profiles = append(profiles, profile{"debian", osVersion.version, arch, osVersion.codename, "debian", "main contrib non-free non-free-firmware", "debian-archive-keyring.gpg"})
		}
	}
	script := "case \"$ID:$VERSION_ID:$(uname -m)\" in\n"
	for _, p := range profiles {
		options := "[signed-by=/usr/share/keyrings/" + p.keyring + "]"
		line := func(base, suite string) string {
			return "deb " + options + " https://mirrors.aliyun.com/" + base + " " + suite + " " + p.components + "\n"
		}
		source := line(p.base, p.codename) + line(p.base, p.codename+"-updates")
		if p.os == "ubuntu" {
			source += line(p.base, p.codename+"-backports") + line(p.base, p.codename+"-security")
		} else {
			source += line("debian-security", p.codename+"-security")
		}
		script += "  " + p.os + ":" + p.version + ":" + p.arch + ")\n"
		// Bootstrap only the distro's signed CA package over the public mirror when an image has no trust bundle.
		// Never use trusted=yes, allow-unauthenticated or a permanent HTTPS verification bypass.
		script += "if [ ! -s /etc/ssl/certs/ca-certificates.crt ]; then\necho '证书包缺失：先从阿里云签名软件源初始化 CA，再切换 HTTPS'\n" + deployAPTSourceScript(strings.ReplaceAll(source, "https://mirrors.aliyun.com/", "http://mirrors.aliyun.com/")) + "apt-get update\napt-get -o APT::Get::AllowUnauthenticated=false install -y --no-install-recommends ca-certificates\nfi\n" + deployAPTSourceScript(source) + ";;\n"
	}
	return script + "  *) echo '无法为当前系统与架构匹配阿里云 APT 源' >&2; exit 1 ;;\nesac\n"
}

func validateDeployCustomSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	if !strings.HasPrefix(source, "[") {
		for _, line := range strings.Split(source, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !regexp.MustCompile(`^deb(-src)? https?://[^\s]+ [a-zA-Z0-9 ./_-]+$`).MatchString(line) {
				return errors.New("APT 源仅接受 deb/deb-src 行；Yum 源请使用 ops-admin-centos7- 前缀的 repo 配置")
			}
		}
		return nil
	}
	sections := map[string]map[string]string{}
	current := ""
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !regexp.MustCompile(`^\[ops-admin-centos7-[a-zA-Z0-9_-]+\]$`).MatchString(line) {
				return errors.New("Yum 仓库 ID 必须以 ops-admin-centos7- 开头")
			}
			current = line[1 : len(line)-1]
			if sections[current] != nil {
				return errors.New("Yum 仓库 ID 重复")
			}
			sections[current] = map[string]string{}
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || current == "" {
			return errors.New("Yum repo 配置格式错误")
		}
		switch key {
		case "name", "baseurl", "enabled", "gpgcheck", "gpgkey", "sslverify", "repo_gpgcheck":
		default:
			return errors.New("Yum repo 包含不支持的配置项：" + key)
		}
		if _, ok := sections[current][key]; ok {
			return errors.New("Yum repo 配置项重复")
		}
		sections[current][key] = value
	}
	validHTTPS := func(value string) bool {
		u, e := url.Parse(value)
		return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(value, " \t\r\n")
	}
	for _, section := range sections {
		if section["enabled"] != "0" || section["gpgcheck"] != "1" || section["sslverify"] != "1" || !validHTTPS(section["baseurl"]) {
			return errors.New("每个 Yum 仓库必须配置 HTTPS baseurl、enabled=0、gpgcheck=1、sslverify=1")
		}
		key := section["gpgkey"]
		if key != "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7" && !validHTTPS(key) {
			return errors.New("Yum gpgkey 必须为 HTTPS URL 或系统 CentOS 7 签名公钥")
		}
		if v, ok := section["repo_gpgcheck"]; ok && v != "1" {
			return errors.New("显式配置 repo_gpgcheck 时必须设为 1")
		}
	}
	if len(sections) == 0 {
		return errors.New("缺少有效的 Yum 仓库")
	}
	return nil
}

func deployAPTSourceScript(source string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(source), "[") {
		return "echo 'Ubuntu/Debian 需要 APT 格式的软件源，不能使用 Yum repo 配置' >&2; exit 1\n"
	}
	return `install -d -m 700 /var/lib/ops-admin/k8s-backup/apt
if [ -f /etc/apt/sources.list ]; then cp -n /etc/apt/sources.list /var/lib/ops-admin/k8s-backup/apt/sources.list; fi
for f in /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; do
  [ -f "$f" ] || continue
  cp -n "$f" "/var/lib/ops-admin/k8s-backup/apt/$(basename "$f")"
  mv -f "$f" "$f.ops-admin-disabled"
done
` + deployWriteFile("/etc/apt/sources.list", source) + "chmod 644 /etc/apt/sources.list\n"
}
