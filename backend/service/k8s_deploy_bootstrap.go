package service

import (
	"strings"
	"time"

	"ops-admin/backend/model"
)

// Prerequisite installation is tracked separately from Kubernetes installation.
// A successful precheck may release its DB ownership; the remote flock still
// prevents a second package manager operation if a timed-out SSH child survives.
func (s *Service) deployDependencyStep(run model.K8sDeployRun, host model.AssetHost) error {
	return s.deployStep(run, host, "基础依赖准备", false, func(log func(string)) error {
		if err := s.claimDeployPhase(run.ID, "dependencies_prepared"); err != nil {
			return err
		}
		_, err := s.deployCommand(host, deployBootstrapScript(run.Config), int(deployDependencyTimeout/time.Second), log)
		return err
	})
}

func deployBootstrapScript(c model.K8sDeployConfig) string {
	script := `export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export LC_ALL=C
test -r /etc/os-release || { echo '无法识别操作系统' >&2; exit 1; }
. /etc/os-release
case "$ID:$VERSION_ID" in
  ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) family=apt ;;
  centos:7|centos:7.9|centos:7.9.2009)
    grep -Eq 'release 7\.9\.2009([[:space:]]|$)' /etc/centos-release || { echo 'CentOS 分支仅支持 7.9.2009' >&2; exit 1; }
    [ "$(uname -m)" = x86_64 ] || { echo 'CentOS 7.9 当前仅支持 x86_64' >&2; exit 1; }
    family=yum ;;
  *) echo "不支持的系统 $ID $VERSION_ID" >&2; exit 1 ;;
esac
for target in /etc/kubernetes/admin.conf /etc/kubernetes/kubelet.conf /etc/kubernetes/manifests/kube-apiserver.yaml /var/lib/etcd/member; do
  [ ! -e "$target" ] || { echo '发现现有 Kubernetes/etcd 数据，未准备依赖' >&2; exit 1; }
done
for service in docker containerd kubelet; do
  if systemctl is-active --quiet "$service"; then echo "已有运行中的 $service，未准备依赖" >&2; exit 1; fi
done
if ! command -v flock >/dev/null; then
  if [ "$family" = apt ]; then
    # OPS_ADMIN_APT_SOURCE
    export DEBIAN_FRONTEND=noninteractive
    apt-get update; apt-get install -y --no-install-recommends util-linux
  else
` + deployCentOSRepositoryScript(c.CustomSource) + `
    yum "${ops_yum_args[@]}" install -y util-linux
  fi
fi
exec 9>/run/ops-admin-k8s-prepare.lock
flock -n 9 || { echo '另一个依赖准备进程尚未退出，请稍后再试' >&2; exit 1; }
if [ "$family" = apt ]; then
  # OPS_ADMIN_APT_SOURCE
  export DEBIAN_FRONTEND=noninteractive
  missing=()
  for package in python3 iproute2 ca-certificates curl gnupg openssl tar gzip util-linux; do
    dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -q 'install ok installed' || missing+=("$package")
  done
  if [ "${#missing[@]}" -gt 0 ]; then apt-get update; apt-get install -y --no-install-recommends "${missing[@]}"; fi
else
` + deployCentOSRepositoryScript(c.CustomSource) + `
  missing=()
  for package in python3 iproute ca-certificates curl gnupg2 openssl tar gzip util-linux; do rpm -q "$package" >/dev/null 2>&1 || missing+=("$package"); done
  if [ "${#missing[@]}" -gt 0 ]; then yum "${ops_yum_args[@]}" install -y "${missing[@]}"; fi
fi
command -v python3 >/dev/null && command -v ip >/dev/null && command -v curl >/dev/null || { echo '基础依赖安装后仍不可用' >&2; exit 1; }
if systemctl is-active --quiet systemd-timesyncd || systemctl is-active --quiet chronyd || systemctl is-active --quiet chrony || systemctl is-active --quiet ntpd; then
  echo '保留现有时间同步服务'
else
  if [ "$family" = apt ]; then
    if ! dpkg-query -W -f='${Status}' chrony 2>/dev/null | grep -q 'install ok installed'; then apt-get update; apt-get install -y --no-install-recommends chrony; fi
    systemctl enable chrony; systemctl start chrony
  else
    rpm -q chrony >/dev/null 2>&1 || yum "${ops_yum_args[@]}" install -y chrony
    systemctl enable chronyd; systemctl start chronyd
  fi
fi
for attempt in $(seq 1 15); do
  if chronyc tracking 2>/dev/null | grep -Eq 'Leap status[[:space:]]*:[[:space:]]*Normal'; then break; fi
  if timedatectl status 2>/dev/null | grep -Eq '(NTP synchronized|System clock synchronized):[[:space:]]*yes'; then break; fi
  sleep 2
done
echo '基础依赖准备完成；时间同步结果将在环境预检中核验。'
`
	apt := deployAliyunAPTSourceScript()
	if strings.TrimSpace(c.CustomSource) != "" {
		apt = deployAPTSourceScript(c.CustomSource)
	}
	return strings.ReplaceAll(script, "# OPS_ADMIN_APT_SOURCE", apt)
}

// Scoped repositories leave existing .repo files untouched and keep RPM signature checks enabled.
func deployCentOSRepositoryScript(custom string) string {
	if strings.TrimSpace(custom) != "" && !strings.HasPrefix(strings.TrimSpace(custom), "[") {
		return "echo 'CentOS 7.9 需要 Yum repo 格式的软件源，不能使用 APT deb 行' >&2; exit 1\n"
	}
	content := `[ops-admin-centos7-base]
name=CentOS 7.9.2009 Base (Ops Admin)
baseurl=https://mirrors.aliyun.com/centos-vault/7.9.2009/os/$basearch/
enabled=0
gpgcheck=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7
sslverify=1
[ops-admin-centos7-updates]
name=CentOS 7.9.2009 Updates (Ops Admin)
baseurl=https://mirrors.aliyun.com/centos-vault/7.9.2009/updates/$basearch/
enabled=0
gpgcheck=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7
sslverify=1
[ops-admin-centos7-extras]
name=CentOS 7.9.2009 Extras (Ops Admin)
baseurl=https://mirrors.aliyun.com/centos-vault/7.9.2009/extras/$basearch/
enabled=0
gpgcheck=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7
sslverify=1
`
	defaultRepo := content
	if strings.TrimSpace(custom) != "" && strings.HasPrefix(strings.TrimSpace(custom), "[") {
		content = custom
	}
	return `test -f /etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7 || { echo 'CentOS RPM 签名公钥缺失，请恢复 centos-release 提供的公钥' >&2; exit 1; }
ops_yum_args=(--disablerepo='*' --enablerepo='ops-admin-centos7-*' --setopt=timeout=30 --setopt=retries=3)
if [ ! -s /etc/pki/tls/certs/ca-bundle.crt ]; then
  echo '证书包缺失：先从阿里云 RPM 签名源初始化 CA，再切换 HTTPS'
` + deployWriteFile("/etc/yum.repos.d/ops-admin-centos7.repo", strings.ReplaceAll(defaultRepo, "https://mirrors.aliyun.com/", "http://mirrors.aliyun.com/")) + `yum "${ops_yum_args[@]}" install -y ca-certificates
fi
` + deployWriteFile("/etc/yum.repos.d/ops-admin-centos7.repo", content)
}

// Keep the duration visible to recovery/status callers without waiting on a package manager in HTTP handlers.
const deployDependencyTimeout = 15 * time.Minute
