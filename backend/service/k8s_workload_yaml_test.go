package service

import (
	"strings"
	"testing"

	ksyaml "sigs.k8s.io/yaml"
)

func TestMarshalK8sYAMLPreservesKubernetesJSONFieldNames(t *testing.T) {
	manifest := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{map[string]any{
						"name": "home",
						"env": []any{map[string]any{
							"name":      "POD_NAME",
							"valueFrom": map[string]any{"fieldRef": map[string]any{"fieldPath": "metadata.name"}},
						}},
					}},
				},
			},
		},
	}

	yamlBody := marshalK8sYAML(manifest)
	if strings.Contains(yamlBody, "valuefrom") || strings.Contains(yamlBody, "fieldref") {
		t.Fatalf("YAML changed Kubernetes field casing: %s", yamlBody)
	}
	if !strings.Contains(yamlBody, "valueFrom:") || !strings.Contains(yamlBody, "fieldRef:") {
		t.Fatalf("YAML lost Kubernetes field names: %s", yamlBody)
	}
	if _, err := ksyaml.YAMLToJSON([]byte(yamlBody)); err != nil {
		t.Fatalf("generated YAML is invalid: %v", err)
	}
}
