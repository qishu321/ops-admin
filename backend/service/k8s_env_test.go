package service

import "testing"

func TestFormatK8sEnvSourceFieldRef(t *testing.T) {
	source := formatK8sEnvSource(map[string]any{
		"fieldRef": map[string]any{"apiVersion": "v1", "fieldPath": "metadata.uid"},
	})
	if source != "fieldRef: metadata.uid" {
		t.Fatalf("unexpected fieldRef source: %q", source)
	}
}

func TestBuildPodItemsIncludesUID(t *testing.T) {
	pod := kubePod{}
	pod.Metadata.Name = "szfc-world-0"
	pod.Metadata.Namespace = "szfc-inner-51"
	pod.Metadata.UID = "pod-uid-123"

	items := buildPodItems([]kubePod{pod})
	if len(items) != 1 || items[0].UID != pod.Metadata.UID {
		t.Fatalf("expected pod UID in workload detail, got %+v", items)
	}
}
