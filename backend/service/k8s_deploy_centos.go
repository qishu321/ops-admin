package service

import (
	"ops-admin/backend/model"
	"strings"
)

// Static official releases avoid installing EL8/EL9 binaries against EL7's glibc.
// Digests are pinned to the upstream release assets, not downloaded from the same endpoint at run time.
const centOSContainerdVersion = "1.7.36"
const centOSContainerdSHA256 = "21862bcd43b4cd583f1b4e96005f6d8e12cf91d0b44780c98624ec46131ecf36"
const centOSRuncVersion = "1.3.6"
const centOSRuncSHA256 = "3f3921dbbee7723e9868f97e88e51ffc910206e3ba55646e74d93d24ea76023c"

func deployCentOSPrepareScript(c model.K8sDeployConfig, n model.K8sDeployNode) string {
	minor := strings.Join(strings.Split(c.Version, ".")[:2], ".")
	base := aliyunKubernetesPackages + "v" + minor + "/rpm/"
	repo := "[ops-admin-kubernetes]\nname=Kubernetes " + minor + " (Ops Admin)\nbaseurl=" + base + "\nenabled=0\ngpgcheck=1\nrepo_gpgcheck=1\nsslverify=1\ngpgkey=" + base + "repodata/repomd.xml.key\nexclude=kubelet kubeadm kubectl cri-tools kubernetes-cni\n"
	script := `export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export LC_ALL=C
install -d -m 700 /var/lib/ops-admin/k8s-backup
for f in /etc/fstab /etc/hostname /etc/hosts; do [ ! -f "$f" ] || cp -n "$f" "/var/lib/ops-admin/k8s-backup/$(basename "$f")"; done
` + deployCentOSRepositoryScript(c.CustomSource) + deployWriteFile("/etc/yum.repos.d/ops-admin-kubernetes.repo", repo) + `
ops_yum_args+=(--enablerepo=ops-admin-kubernetes --disableexcludes=ops-admin-kubernetes)
yum "${ops_yum_args[@]}" install -y ca-certificates curl openssl conntrack-tools socat iptables iproute iputils ebtables ethtool
swapoff -a
sed -ri '/\sswap\s/s/^([^#])/# ops-admin \1/' /etc/fstab
printf '%s\n' overlay br_netfilter > /etc/modules-load.d/ops-admin-k8s.conf
modprobe overlay; modprobe br_netfilter
printf '%s\n' 'net.bridge.bridge-nf-call-iptables = 1' 'net.bridge.bridge-nf-call-ip6tables = 1' 'net.ipv4.ip_forward = 1' > /etc/sysctl.d/99-ops-admin-k8s.conf
sysctl -p /etc/sysctl.d/99-ops-admin-k8s.conf
tmp=$(mktemp -d /tmp/ops-admin-runtime-XXXXXX)
trap 'rm -f "$tmp/containerd.tar.gz" "$tmp/runc"; rmdir "$tmp"' EXIT
`
	script += "curl -fL --retry 3 --connect-timeout 15 --max-time 600 " + shellQuote("https://github.com/containerd/containerd/releases/download/v"+centOSContainerdVersion+"/containerd-static-"+centOSContainerdVersion+"-linux-amd64.tar.gz") + " -o \"$tmp/containerd.tar.gz\"\n"
	script += "printf '%s  %s\\n' " + shellQuote(centOSContainerdSHA256) + " \"$tmp/containerd.tar.gz\" | sha256sum -c -\n"
	script += "curl -fL --retry 3 --connect-timeout 15 --max-time 300 " + shellQuote("https://github.com/opencontainers/runc/releases/download/v"+centOSRuncVersion+"/runc.amd64") + " -o \"$tmp/runc\"\n"
	script += "printf '%s  %s\\n' " + shellQuote(centOSRuncSHA256) + " \"$tmp/runc\" | sha256sum -c -\n"
	containerdRoot := "/usr/local/lib/ops-admin/containerd/" + centOSContainerdVersion
	runcPath := "/usr/local/lib/ops-admin/runc/" + centOSRuncVersion + "/runc"
	script += "install -d " + shellQuote(containerdRoot) + " " + shellQuote("/usr/local/lib/ops-admin/runc/"+centOSRuncVersion) + "\ntar -xzf \"$tmp/containerd.tar.gz\" -C " + shellQuote(containerdRoot) + "\ninstall -m 755 \"$tmp/runc\" " + shellQuote(runcPath) + "\n"
	script += "export PATH=" + shellQuote(containerdRoot+"/bin") + ":\"$PATH\"\ncontainerd --version\n" + shellQuote(runcPath) + " --version\ninstall -d /etc/containerd\nif [ -f /etc/containerd/config.toml ]; then cp -n /etc/containerd/config.toml /var/lib/ops-admin/k8s-backup/containerd.toml; fi\ncontainerd config default > /etc/containerd/config.toml\nsed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml\n"
	script += "sed -i " + shellQuote(`s|BinaryName = ""|BinaryName = "`+runcPath+`"|`) + " /etc/containerd/config.toml\n"
	if c.RuntimeDirectory != "" {
		script += "install -d " + shellQuote(c.RuntimeDirectory) + "\nsed -i " + shellQuote(`s|^root = .*|root = "`+c.RuntimeDirectory+`"|`) + " /etc/containerd/config.toml\n"
	}
	if c.AutoHostname {
		script += "hostnamectl set-hostname " + shellQuote(n.Name) + "\n"
	}
	script += deploySELinuxScript(c) + deployFirewalldScript(c)
	if c.ProxyMode == "ipvs" {
		script += `yum "${ops_yum_args[@]}" install -y ipvsadm ipset
for module in ip_vs ip_vs_rr ip_vs_wrr ip_vs_sh nf_conntrack; do modprobe "$module"; echo "$module" >> /etc/modules-load.d/ops-admin-k8s.conf; done
`
	}
	unit := "[Unit]\nDescription=containerd container runtime (Ops Admin)\nAfter=network.target\n[Service]\nEnvironment=\"PATH=" + containerdRoot + "/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"\nExecStart=" + containerdRoot + "/bin/containerd\nDelegate=yes\nKillMode=process\nRestart=always\nRestartSec=5\nLimitNOFILE=1048576\nLimitNPROC=infinity\nLimitCORE=infinity\n[Install]\nWantedBy=multi-user.target\n"
	script += deployWriteFile("/etc/systemd/system/containerd.service", unit) + "systemctl daemon-reload\nsystemctl enable containerd\nsystemctl start containerd\n"
	script += "yum \"${ops_yum_args[@]}\" install -y " + shellQuote("kubelet-"+c.Version) + " " + shellQuote("kubeadm-"+c.Version) + " " + shellQuote("kubectl-"+c.Version) + "\nsystemctl enable kubelet\n"
	script += deploySandboxImageScript(c)
	return script
}

func deploySELinuxScript(c model.K8sDeployConfig) string {
	if !c.DisableSELinux {
		return ""
	}
	return `if command -v getenforce >/dev/null 2>&1; then
  if [ "$(getenforce)" = Enforcing ]; then setenforce 0; fi
  if [ -f /etc/selinux/config ]; then cp -n /etc/selinux/config /var/lib/ops-admin/k8s-backup/selinux-config; sed -i 's/^SELINUX=.*/SELINUX=disabled/' /etc/selinux/config; fi
fi
`
}

func deployFirewalldScript(c model.K8sDeployConfig) string {
	if !c.DisableFirewalld {
		return ""
	}
	return `if systemctl list-unit-files firewalld.service --no-legend 2>/dev/null | grep -q firewalld; then
  systemctl stop firewalld
  systemctl disable firewalld
fi
`
}
