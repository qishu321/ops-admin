package service

import "testing"

func TestResolveK8sNetworkCIDRsReadsACKTerwayConfig(t *testing.T) {
	configMaps := []kubeConfigMap{{
		Metadata: kubeMetadata{Name: "eni-config", Namespace: "kube-system"},
		Data:     map[string]string{"eni_conf": `{"service_cidr":"172.21.0.0/20","vswitches":{"cn-shanghai-a":["vsw-z","vsw-a"],"cn-shanghai-b":["vsw-a"]}}`},
	}}

	serviceCIDR, podNetwork := resolveK8sNetworkCIDRs(nil, configMaps)
	if serviceCIDR != "172.21.0.0/20" {
		t.Fatalf("service CIDR = %q, want %q", serviceCIDR, "172.21.0.0/20")
	}
	if podNetwork != "Terway · Pod 虚拟交换机：vsw-a、vsw-z" {
		t.Fatalf("pod network = %q", podNetwork)
	}
}
