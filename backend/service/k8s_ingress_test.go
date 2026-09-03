package service

import (
	"encoding/json"
	"testing"

	"ops-admin/backend/model"
)

func TestIngressClassName(t *testing.T) {
	tests := []struct {
		name       string
		specClass  string
		annotation string
		want       string
	}{
		{name: "spec field", specClass: "nginx", annotation: "legacy", want: "nginx"},
		{name: "legacy annotation", annotation: "traefik", want: "traefik"},
		{name: "missing", want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ingress kubeIngress
			ingress.Spec.IngressClassName = tt.specClass
			if tt.annotation != "" {
				ingress.Metadata.Annotations = map[string]string{"kubernetes.io/ingress.class": tt.annotation}
			}

			if got := ingressClassName(ingress); got != tt.want {
				t.Fatalf("ingressClassName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildNetworkSectionIncludesIngressClassName(t *testing.T) {
	var ingress kubeIngress
	ingress.Metadata.Name = "demo"
	ingress.Metadata.Namespace = "default"
	ingress.Spec.IngressClassName = "nginx"
	ingress.Spec.DefaultBackend.Service.Name = "fallback-service"
	ingress.Spec.DefaultBackend.Service.Port.Number = 8080
	ingress.Spec.Rules = append(ingress.Spec.Rules, kubeIngressRule{})
	var pathBackend kubeIngressPath
	pathBackend.Backend.Service.Name = "api-service"
	pathBackend.Backend.Service.Port.Name = "http"
	ingress.Spec.Rules[0].HTTP.Paths = append(ingress.Spec.Rules[0].HTTP.Paths, pathBackend)

	section := buildNetworkSection(nil, []kubeIngress{ingress}, nil, nil)
	if len(section.Ingresses) != 1 {
		t.Fatalf("len(Ingresses) = %d, want 1", len(section.Ingresses))
	}
	if got := section.Ingresses[0].ClassName; got != "nginx" {
		t.Fatalf("ClassName = %q, want %q", got, "nginx")
	}
	if got := section.Ingresses[0].Backend; got != "fallback-service:8080, api-service:http" {
		t.Fatalf("Backend = %q, want %q", got, "fallback-service:8080, api-service:http")
	}
}

func TestBuildIngressRuleSupportsNamedPort(t *testing.T) {
	rule, err := buildIngressRule(model.K8sIngressRuleSpec{
		Host:        "api.example.com",
		Path:        "/api",
		PathType:    "Exact",
		ServiceName: "api-service",
		ServicePort: "http",
	})
	if err != nil {
		t.Fatalf("buildIngressRule() error = %v", err)
	}
	if got := rule["host"]; got != "api.example.com" {
		t.Fatalf("host = %v, want api.example.com", got)
	}
	httpRule := rule["http"].(map[string]any)
	paths := httpRule["paths"].([]map[string]any)
	if got := paths[0]["pathType"]; got != "Exact" {
		t.Fatalf("pathType = %v, want Exact", got)
	}
	backend := paths[0]["backend"].(map[string]any)
	service := backend["service"].(map[string]any)
	port := service["port"].(map[string]any)
	if got := port["name"]; got != "http" {
		t.Fatalf("port name = %v, want http", got)
	}
}

func TestBuildIngressRuleRejectsInvalidValues(t *testing.T) {
	tests := []model.K8sIngressRuleSpec{
		{ServicePort: "80"},
		{ServiceName: "api", ServicePort: "0"},
		{ServiceName: "api", ServicePort: "80", PathType: "Unknown"},
	}
	for _, rule := range tests {
		if _, err := buildIngressRule(rule); err == nil {
			t.Fatalf("buildIngressRule(%+v) expected error", rule)
		}
	}
}

func TestIngressRuleSpecsIncludesDefaultBackend(t *testing.T) {
	var ingress kubeIngress
	ingress.Spec.DefaultBackend.Service.Name = "nginx-demo"
	ingress.Spec.DefaultBackend.Service.Port.Number = 80

	rules := ingressRuleSpecs(ingress)
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	if !rules[0].DefaultBackend || rules[0].ServiceName != "nginx-demo" || rules[0].ServicePort != "80" {
		t.Fatalf("default backend rule = %+v", rules[0])
	}
}

func TestIngressRuleSpecsIncludesHTTPBackend(t *testing.T) {
	var ingress kubeIngress
	if err := json.Unmarshal([]byte(`{"spec":{"rules":[{"host":"","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"nginx-demo","port":{"number":80}}}}]}}]}}`), &ingress); err != nil {
		t.Fatalf("unmarshal ingress: %v", err)
	}

	rules := ingressRuleSpecs(ingress)
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	if rules[0].DefaultBackend || rules[0].Path != "/" || rules[0].ServiceName != "nginx-demo" || rules[0].ServicePort != "80" {
		t.Fatalf("HTTP backend rule = %+v", rules[0])
	}
}

func TestBuildNetworkSectionIncludesServicePortSpecs(t *testing.T) {
	var service kubeService
	if err := json.Unmarshal([]byte(`{"metadata":{"name":"nginx-demo","namespace":"demo"},"spec":{"type":"ClusterIP","clusterIP":"10.0.0.10","ports":[{"name":"http","port":80,"protocol":"TCP","targetPort":8080}]}}`), &service); err != nil {
		t.Fatalf("unmarshal service: %v", err)
	}

	section := buildNetworkSection([]kubeService{service}, nil, nil, nil)
	if len(section.Services) != 1 || len(section.Services[0].PortSpecs) != 1 {
		t.Fatalf("services = %+v", section.Services)
	}
	port := section.Services[0].PortSpecs[0]
	if port.Name != "http" || port.Port != 80 || port.TargetPort != "8080" {
		t.Fatalf("port spec = %+v", port)
	}
}

func TestBuildNetworkSectionIncludesIngressClasses(t *testing.T) {
	var ingressClass kubeIngressClass
	ingressClass.Metadata.Name = "nginx"
	ingressClass.Spec.Controller = "k8s.io/ingress-nginx"
	ingressClass.Spec.Parameters.Kind = "IngressParameters"
	ingressClass.Spec.Parameters.Name = "nginx-parameters"

	section := buildNetworkSection(nil, nil, []kubeIngressClass{ingressClass}, nil)
	if len(section.IngressClasses) != 1 {
		t.Fatalf("len(IngressClasses) = %d, want 1", len(section.IngressClasses))
	}
	item := section.IngressClasses[0]
	if item.Name != "nginx" || item.Controller != "k8s.io/ingress-nginx" || item.Parameters != "IngressParameters/nginx-parameters" {
		t.Fatalf("ingress class = %+v", item)
	}
}
