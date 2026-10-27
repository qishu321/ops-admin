package service

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"ops-admin/backend/model"
)

func deployPrepareScript(c model.K8sDeployConfig, n model.K8sDeployNode) string {
	minor := strings.Join(strings.Split(c.Version, ".")[:2], ".")
	script := `set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
install -d -m 700 /var/lib/ops-admin/k8s-backup
for f in /etc/fstab /etc/hostname /etc/hosts /etc/apt/sources.list; do
  if [ -f "$f" ]; then cp -n "$f" "/var/lib/ops-admin/k8s-backup/$(basename "$f")"; fi
done
apt-get update
apt-get install -y ca-certificates curl gpg containerd conntrack socat iptables iproute2 iputils-ping openssl
swapoff -a
sed -ri '/\sswap\s/s/^([^#])/# ops-admin \1/' /etc/fstab
printf '%s\n' overlay br_netfilter > /etc/modules-load.d/ops-admin-k8s.conf
modprobe overlay
modprobe br_netfilter
printf '%s\n' 'net.bridge.bridge-nf-call-iptables = 1' 'net.bridge.bridge-nf-call-ip6tables = 1' 'net.ipv4.ip_forward = 1' > /etc/sysctl.d/99-ops-admin-k8s.conf
sysctl --system
install -d /etc/containerd /etc/apt/keyrings
if [ -f /etc/containerd/config.toml ]; then cp -n /etc/containerd/config.toml /var/lib/ops-admin/k8s-backup/containerd.toml; fi
containerd config default > /etc/containerd/config.toml
sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml
`
	if c.CustomSource != "" {
		// Isolated apt source file, backed up existing sources and restored on failure by the operator.
		prefix := `install -d -m 700 /var/lib/ops-admin/k8s-backup/apt
cp -a /etc/apt/sources.list.d /var/lib/ops-admin/k8s-backup/apt/ 2>/dev/null || true
find /etc/apt/sources.list.d -maxdepth 1 -type f \( -name '*.list' -o -name '*.sources' \) -exec sh -c 'mv "$1" "$1.ops-admin-disabled"' sh {} \;
`
		script = strings.Replace(script, "apt-get update", prefix+deployWriteFile("/etc/apt/sources.list", c.CustomSource)+"apt-get update", 1)
	}
	if c.AutoHostname {
		script += "hostnamectl set-hostname " + shellQuote(n.Name) + "\n"
	}
	if c.DisableSELinux {
		script += `if command -v getenforce >/dev/null 2>&1; then
  if [ "$(getenforce)" = Enforcing ]; then setenforce 0; fi
  if [ -f /etc/selinux/config ]; then cp -n /etc/selinux/config /var/lib/ops-admin/k8s-backup/selinux-config; sed -i 's/^SELINUX=.*/SELINUX=disabled/' /etc/selinux/config; fi
fi
`
	}
	if c.ProxyMode == "ipvs" {
		script += `apt-get install -y ipvsadm ipset
for module in ip_vs ip_vs_rr ip_vs_wrr ip_vs_sh nf_conntrack; do modprobe "$module"; echo "$module" >> /etc/modules-load.d/ops-admin-k8s.conf; done
`
	}
	if c.RuntimeDirectory != "" {
		script += "install -d " + shellQuote(c.RuntimeDirectory) + "\nsed -i " + shellQuote("s|^root = .*|root = \""+c.RuntimeDirectory+"\"|") + " /etc/containerd/config.toml\n"
	}
	script += "systemctl enable --now containerd\nsystemctl restart containerd\n"
	script += "curl -fsSL --retry 3 " + shellQuote("https://pkgs.k8s.io/core:/stable:/v"+minor+"/deb/Release.key") + " | gpg --dearmor --yes -o /etc/apt/keyrings/ops-admin-kubernetes.gpg\n"
	script += "chmod 644 /etc/apt/keyrings/ops-admin-kubernetes.gpg\n"
	script += "printf '%s\\n' " + shellQuote("deb [signed-by=/etc/apt/keyrings/ops-admin-kubernetes.gpg] https://pkgs.k8s.io/core:/stable:/v"+minor+"/deb/ /") + " > /etc/apt/sources.list.d/ops-admin-kubernetes.list\napt-get update\n"
	script += "package_version=$(apt-cache madison kubeadm | awk '{print $3}' | grep -F " + shellQuote(c.Version+"-") + " | head -n 1)\n[ -n \"$package_version\" ] || { echo '软件源未找到指定 Kubernetes 版本' >&2; exit 1; }\n"
	script += "apt-get install -y kubelet=\"$package_version\" kubeadm=\"$package_version\" kubectl=\"$package_version\"\napt-mark hold kubelet kubeadm kubectl\nsystemctl enable kubelet\n"
	script += "sandbox_image=$(kubeadm config images list --kubernetes-version " + shellQuote("v"+c.Version) + " --image-repository " + shellQuote(c.ImageRepository) + " | grep '/pause:')\nsed -i \"s|sandbox_image = .*|sandbox_image = \\\"$sandbox_image\\\"|\" /etc/containerd/config.toml\nsystemctl restart containerd\n"
	return script
}

func deployNodeRegistration(n model.K8sDeployNode) map[string]any {
	return map[string]any{"name": n.Name, "criSocket": "unix:///var/run/containerd/containerd.sock", "kubeletExtraArgs": []map[string]string{{"name": "node-ip", "value": n.IP}}}
}

func deployInitYAML(c model.K8sDeployConfig, n model.K8sDeployNode, endpoint string) (string, error) {
	init := map[string]any{"apiVersion": "kubeadm.k8s.io/v1beta4", "kind": "InitConfiguration", "localAPIEndpoint": map[string]any{"advertiseAddress": n.IP, "bindPort": 6443}, "nodeRegistration": deployNodeRegistration(n)}
	sans := []string{n.IP}
	if c.APIServer != "" {
		sans = append(sans, c.APIServer)
	}
	cluster := map[string]any{"apiVersion": "kubeadm.k8s.io/v1beta4", "kind": "ClusterConfiguration", "kubernetesVersion": "v" + c.Version, "controlPlaneEndpoint": endpoint, "imageRepository": c.ImageRepository, "networking": map[string]any{"podSubnet": c.PodCIDR, "serviceSubnet": c.ServiceCIDR, "dnsDomain": "cluster.local"}, "apiServer": map[string]any{"certSANs": sans}}
	if c.CustomCertificates {
		cluster["certificateValidityPeriod"] = fmt.Sprintf("%dh", c.CertificateYears*365*24)
		cluster["caCertificateValidityPeriod"] = fmt.Sprintf("%dh", c.CAYears*365*24)
	}
	kubelet, err := deployKubeletOverrides(c.KubeletConfig)
	if err != nil {
		return "", err
	}
	kubelet["apiVersion"] = "kubelet.config.k8s.io/v1beta1"
	kubelet["kind"] = "KubeletConfiguration"
	kubelet["cgroupDriver"] = "systemd"
	proxy := map[string]any{"apiVersion": "kubeproxy.config.k8s.io/v1alpha1", "kind": "KubeProxyConfiguration", "mode": c.ProxyMode}
	return deployYAMLDocuments(init, cluster, kubelet, proxy)
}

func deployJoinYAML(c model.K8sDeployConfig, n model.K8sDeployNode, endpoint, token, caHash, certificateKey string) (string, error) {
	config := map[string]any{"apiVersion": "kubeadm.k8s.io/v1beta4", "kind": "JoinConfiguration", "nodeRegistration": deployNodeRegistration(n), "discovery": map[string]any{"bootstrapToken": map[string]any{"apiServerEndpoint": endpoint, "token": token, "caCertHashes": []string{"sha256:" + caHash}}}}
	if n.Role == "master" {
		config["controlPlane"] = map[string]any{"localAPIEndpoint": map[string]any{"advertiseAddress": n.IP, "bindPort": 6443}, "certificateKey": certificateKey}
	}
	return deployYAMLDocuments(config)
}

func deployYAMLDocuments(documents ...map[string]any) (string, error) {
	var result strings.Builder
	for _, doc := range documents {
		data, err := yaml.Marshal(doc)
		if err != nil {
			return "", err
		}
		if result.Len() > 0 {
			result.WriteString("---\n")
		}
		result.Write(data)
	}
	return result.String(), nil
}

func deployCNIScript(c model.K8sDeployConfig) string {
	script := "set -euo pipefail\nexport KUBECONFIG=/etc/kubernetes/admin.conf\n"
	if c.NetworkPlugin == "calico" {
		script += `curl -fsSL --retry 3 https://raw.githubusercontent.com/projectcalico/calico/v3.31.2/manifests/calico.yaml -o /etc/kubernetes/ops-admin-cni.yaml
`
		// Set the pool before the first calico-node starts; changing it after apply can race pool creation.
		script += "sed -i " + shellQuote(`s|# - name: CALICO_IPV4POOL_CIDR|- name: CALICO_IPV4POOL_CIDR|`) + " /etc/kubernetes/ops-admin-cni.yaml\n"
		script += "sed -i " + shellQuote(`s|#   value: "192.168.0.0/16"|  value: "`+c.PodCIDR+`"|`) + " /etc/kubernetes/ops-admin-cni.yaml\n"
		script += "kubectl apply --server-side -f /etc/kubernetes/ops-admin-cni.yaml\nkubectl set env daemonset/calico-node -n kube-system IP_AUTODETECTION_METHOD=kubernetes-internal-ip\n"
	} else {
		script += `curl -fsSL --retry 3 https://raw.githubusercontent.com/flannel-io/flannel/v0.27.4/Documentation/kube-flannel.yml -o /etc/kubernetes/ops-admin-cni.yaml
`
		script += "sed -i " + shellQuote("s|10.244.0.0/16|"+c.PodCIDR+"|g") + " /etc/kubernetes/ops-admin-cni.yaml\nkubectl apply -f /etc/kubernetes/ops-admin-cni.yaml\n"
		script += "kubectl patch daemonset kube-flannel-ds -n kube-flannel --type strategic -p " + shellQuote(`{"spec":{"template":{"spec":{"containers":[{"name":"kube-flannel","args":["--ip-masq","--kube-subnet-mgr","--iface=$(NODE_IP)"],"env":[{"name":"NODE_IP","valueFrom":{"fieldRef":{"fieldPath":"status.hostIP"}}}]}]}}}}`) + "\n"
	}
	return script
}

func deployLBScript(c model.K8sDeployConfig, nodes []model.K8sDeployNode, index int) string {
	masters := []model.K8sDeployNode{}
	for _, n := range nodes {
		if n.Role == "master" {
			masters = append(masters, n)
		}
	}
	self := masters[index]
	haproxy := fmt.Sprintf("global\n  log /dev/log local0\n  daemon\n\ndefaults\n  mode tcp\n  timeout connect 5s\n  timeout client 50s\n  timeout server 50s\n\nfrontend kubernetes\n  bind *:%d\n  default_backend apiservers\n\nbackend apiservers\n  balance roundrobin\n  option tcp-check\n", c.APIPort)
	peers := ""
	for i, n := range masters {
		haproxy += fmt.Sprintf("  server cp%d %s:6443 check\n", i+1, n.IP)
		if i != index {
			peers += "    " + n.IP + "\n"
		}
	}
	keepalived := fmt.Sprintf("global_defs {\n  enable_script_security\n  script_user root\n}\nvrrp_script check_apiserver_lb {\n  script \"/usr/bin/pgrep -x haproxy\"\n  interval 2\n  fall 2\n  rise 2\n}\nvrrp_instance KUBERNETES {\n  state BACKUP\n  interface %s\n  virtual_router_id %d\n  priority %d\n  advert_int 1\n  unicast_src_ip %s\n  unicast_peer {\n%s  }\n  virtual_ipaddress {\n    %s/32 dev %s\n  }\n  track_script {\n    check_apiserver_lb\n  }\n}\n", self.Interface, c.RouterID, 100-index, self.IP, peers, c.APIServer, self.Interface)
	return "set -euo pipefail\nexport DEBIAN_FRONTEND=noninteractive\napt-get install -y haproxy keepalived\ninstall -d -m 700 /var/lib/ops-admin/k8s-backup/lb\ncp -n /etc/haproxy/haproxy.cfg /var/lib/ops-admin/k8s-backup/lb/haproxy.cfg\nif [ -f /etc/keepalived/keepalived.conf ]; then cp -n /etc/keepalived/keepalived.conf /var/lib/ops-admin/k8s-backup/lb/keepalived.conf; fi\n" + deployWriteFile("/etc/haproxy/haproxy.cfg", haproxy) + deployWriteFile("/etc/keepalived/keepalived.conf", keepalived) + "haproxy -c -f /etc/haproxy/haproxy.cfg\nkeepalived -t -f /etc/keepalived/keepalived.conf\nsystemctl enable haproxy keepalived\nsystemctl restart haproxy keepalived\n"
}
