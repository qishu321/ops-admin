package service

import "testing"

func TestBuildPodItemsReportsContainerReadinessSeparatelyFromPhase(t *testing.T) {
	var pod kubePod
	pod.Metadata.Name = "home-0"
	pod.Metadata.Namespace = "default"
	pod.Status.Phase = "Running"
	pod.Spec.Containers = []kubeContainer{{Name: "home"}}
	pod.Status.ContainerStatuses = []struct {
		Name         string `json:"name"`
		RestartCount int    `json:"restartCount"`
		Ready        bool   `json:"ready"`
	}{{Name: "home", Ready: false}}

	items := buildPodItems([]kubePod{pod})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != "Running" || items[0].ReadyContainers != 0 || items[0].TotalContainers != 1 {
		t.Fatalf("pod summary = %+v, want Running with readiness 0/1", items[0])
	}
}
