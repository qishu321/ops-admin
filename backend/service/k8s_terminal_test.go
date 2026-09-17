package service

import (
	"testing"

	"k8s.io/client-go/rest"
)

func TestK8sSPDYConfigThroughGatewayUsesTunnelAndKeepsTLSIdentity(t *testing.T) {
	original := &rest.Config{
		Host: "https://10.1.23.45:6443",
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte("cluster-ca"),
		},
	}

	got, target, err := k8sSPDYConfigThroughGateway(original, "127.0.0.1:43210")
	if err != nil {
		t.Fatalf("prepare gateway SPDY config: %v", err)
	}
	if target != "10.1.23.45:6443" {
		t.Fatalf("target = %q, want %q", target, "10.1.23.45:6443")
	}
	if got.Host != "https://127.0.0.1:43210" {
		t.Fatalf("host = %q, want local tunnel", got.Host)
	}
	if got.TLSClientConfig.ServerName != "10.1.23.45" {
		t.Fatalf("server name = %q, want original API server identity", got.TLSClientConfig.ServerName)
	}
	if got.Dial != nil {
		t.Fatal("SPDY config must dial the local tunnel directly")
	}
	if original.Host != "https://10.1.23.45:6443" || original.TLSClientConfig.ServerName != "" {
		t.Fatal("original REST config was mutated")
	}
}

func TestK8sSPDYConfigThroughGatewayAddsDefaultHTTPSPort(t *testing.T) {
	got, target, err := k8sSPDYConfigThroughGateway(&rest.Config{Host: "https://kube.internal"}, "127.0.0.1:43210")
	if err != nil {
		t.Fatalf("prepare gateway SPDY config: %v", err)
	}
	if target != "kube.internal:443" {
		t.Fatalf("target = %q, want %q", target, "kube.internal:443")
	}
	if got.TLSClientConfig.ServerName != "kube.internal" {
		t.Fatalf("server name = %q", got.TLSClientConfig.ServerName)
	}
}
