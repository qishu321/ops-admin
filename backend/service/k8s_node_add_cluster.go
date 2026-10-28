package service

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"ops-admin/backend/model"
)

type nodeAddCluster struct {
	Cluster                              model.K8sCluster
	Config                               model.K8sDeployConfig
	Endpoint, CAHash, UID, Hash, Network string
	Nodes                                []corev1.Node
	Client                               kubernetes.Interface
	Cleanup                              func()
}

func nodeAddCAHash(data []byte) (string, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return "", errors.New("集群 CA 证书缺失或格式无效")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !cert.IsCA {
		return "", errors.New("集群 CA 证书无效")
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) nodeAddClusterSnapshot(id uint) (*nodeAddCluster, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	c, err := s.GetK8sCluster(id)
	if err != nil {
		return nil, err
	}
	rc, cleanup, err := s.k8sRESTConfigForCluster(c)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			cleanup()
		}
	}()
	if rc.Insecure {
		return nil, errors.New("添加节点要求 kubeconfig 启用 CA 证书校验，请先修复集群 TLS 配置")
	}
	rc.Timeout = 10 * time.Second
	client, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cm, err := client.CoreV1().ConfigMaps("kube-system").Get(ctx, "kubeadm-config", metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("无法读取 kubeadm-config：当前仅支持 kubeadm 集群，并需要读取系统配置的权限")
	}
	var cfg struct {
		Version    string `yaml:"kubernetesVersion"`
		Endpoint   string `yaml:"controlPlaneEndpoint"`
		Repository string `yaml:"imageRepository"`
		Networking struct {
			Pod     string `yaml:"podSubnet"`
			Service string `yaml:"serviceSubnet"`
		} `yaml:"networking"`
	}
	if err = yaml.Unmarshal([]byte(cm.Data["ClusterConfiguration"]), &cfg); err != nil {
		return nil, errors.New("kubeadm 集群配置无法解析")
	}
	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return nil, errors.New("无法读取集群版本")
	}
	v := strings.TrimPrefix(version.GitVersion, "v")
	if !nodeAddVersionSupported(v) {
		return nil, fmt.Errorf("当前安装器支持 Kubernetes 1.31–1.34 的标准版本，目标版本为 %s", v)
	}
	if cfg.Version != "v"+v {
		return nil, errors.New("集群正在升级或 kubeadm 配置版本不一致，请完成升级后再添加节点")
	}
	if cfg.Repository == "" {
		cfg.Repository = "registry.k8s.io"
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,254}$`).MatchString(cfg.Repository) {
		return nil, errors.New("集群镜像仓库地址不受当前安装器支持")
	}
	for _, cidr := range []string{cfg.Networking.Pod, cfg.Networking.Service} {
		if cidr == "" && cfg.Networking.Pod == "" && cfg.Networking.Service != "" {
			continue
		}
		if _, _, e := net.ParseCIDR(cidr); e != nil || strings.Contains(cidr, ":") || strings.Contains(cidr, ",") {
			return nil, errors.New("当前添加节点仅支持 IPv4 网络，且集群需记录有效的 Service CIDR")
		}
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		parsed, e := url.Parse(rc.Host)
		if e != nil {
			return nil, e
		}
		endpoint = parsed.Host
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || isLoopbackK8sAPIHost(host) {
		return nil, errors.New("集群 API 入口必须是节点可达的主机名或 IP，并包含端口")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return nil, errors.New("API 入口端口无效")
	}
	info, err := client.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("无法读取 kube-public/cluster-info，请核对 kubeadm bootstrap 配置")
	}
	public, err := clientcmd.Load([]byte(info.Data["kubeconfig"]))
	if err != nil || len(public.Clusters) != 1 {
		return nil, errors.New("cluster-info 中的集群发现配置无效")
	}
	var hash string
	for _, cluster := range public.Clusters {
		hash, err = nodeAddCAHash(cluster.CertificateAuthorityData)
	}
	if err != nil {
		return nil, err
	}
	ns, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("无法确认目标集群身份")
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.New("无法读取现有节点列表")
	}
	if len(nodes.Items) == 0 {
		return nil, errors.New("目标集群没有可核对的现有节点")
	}
	for _, n := range nodes.Items {
		if !strings.HasPrefix(n.Status.NodeInfo.ContainerRuntimeVersion, "containerd://") {
			return nil, errors.New("当前添加节点安装器仅支持 containerd 集群")
		}
	}
	for _, verb := range []string{"create", "delete"} {
		review, e := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authv1.SelfSubjectAccessReview{Spec: authv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authv1.ResourceAttributes{Namespace: "kube-system", Verb: verb, Resource: "secrets"}}}, metav1.CreateOptions{})
		if e != nil || !review.Status.Allowed {
			return nil, errors.New("集群凭据缺少创建或撤销短期 bootstrap token 的权限")
		}
	}
	for _, name := range []string{"kubeadm:kubelet-bootstrap", "kubeadm:node-autoapprove-bootstrap"} {
		if _, e := client.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{}); e != nil {
			return nil, fmt.Errorf("缺少 kubeadm 引导角色绑定 %s，请先核对集群 bootstrap 配置", name)
		}
	}
	config := model.K8sDeployConfig{Version: v, ImageRepository: cfg.Repository, PodCIDR: cfg.Networking.Pod, ServiceCIDR: cfg.Networking.Service, APIServer: host, APIPort: p, ProxyMode: "iptables"}
	kubeletConfig, e := client.CoreV1().ConfigMaps("kube-system").Get(ctx, "kubelet-config", metav1.GetOptions{})
	if e != nil {
		return nil, errors.New("无法读取集群 kubelet-config，不能确认 cgroup 配置")
	}
	var kubelet struct {
		Driver string `yaml:"cgroupDriver"`
		FailV1 bool   `yaml:"failCgroupV1"`
	}
	if yaml.Unmarshal([]byte(kubeletConfig.Data["kubelet"]), &kubelet) != nil {
		return nil, errors.New("集群 kubelet cgroup 配置无法解析")
	}
	if kubelet.Driver != "" && kubelet.Driver != "systemd" {
		return nil, errors.New("当前添加节点模板要求目标集群使用 systemd cgroup 驱动")
	}
	config.RequireCgroupV2 = kubelet.FailV1
	if proxy, e := client.CoreV1().ConfigMaps("kube-system").Get(ctx, "kube-proxy", metav1.GetOptions{}); e == nil {
		var settings struct {
			Mode string `yaml:"mode"`
		}
		if yaml.Unmarshal([]byte(proxy.Data["config.conf"]), &settings) != nil {
			return nil, errors.New("集群 kube-proxy 配置无法解析")
		}
		if settings.Mode == "ipvs" {
			config.ProxyMode = "ipvs"
		}
	}
	// The existing worker joins the cluster's CNI; no CNI is installed or changed.
	snap := &nodeAddCluster{Cluster: c, Config: config, Endpoint: endpoint, CAHash: hash, UID: string(ns.UID), Nodes: nodes.Items, Client: client, Cleanup: cleanup, Network: "沿用现有 CNI"}
	data, _ := json.Marshal([]any{snap.UID, cfg, config, hash, c.KubeConfig, c.ConnectionMode, c.GatewayID})
	sum := sha256.Sum256(data)
	snap.Hash = hex.EncodeToString(sum[:])
	failed = false
	return snap, nil
}

func nodeAddVersionSupported(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 || parts[0] != "1" {
		return false
	}
	minor, e := strconv.Atoi(parts[1])
	if e != nil || minor < 31 || minor > 34 {
		return false
	}
	patch, e := strconv.Atoi(parts[2])
	return e == nil && patch >= 0 && strconv.Itoa(patch) == parts[2]
}
