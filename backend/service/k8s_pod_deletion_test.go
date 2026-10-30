package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ops-admin/backend/model"
)

func TestPodStatusTerminationTakesPrecedenceOverRunningAndWaiting(t *testing.T) {
	var p kubePod
	if err := json.Unmarshal([]byte(`{"metadata":{"uid":"old","deletionTimestamp":"2026-10-30T01:00:00Z","finalizers":["example/cleanup"]},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running","containerStatuses":[{"name":"app","ready":false,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`), &p); err != nil {
		t.Fatal(err)
	}
	items := buildPodItems([]kubePod{p})
	if items[0].Status != "Terminating" || items[0].Phase != "Running" || len(items[0].Finalizers) != 1 {
		t.Fatalf("incorrect summary: %+v", items[0])
	}
	p.Metadata.DeletionTimestamp = ""
	if podDisplayStatus(p) != "CrashLoopBackOff" {
		t.Fatal("waiting reason was lost")
	}
}

func TestK8sWorkloadVersionConflictsDoNotOverwriteNewerResources(t *testing.T) {
	current := map[string]any{"metadata": map[string]any{"uid": "new", "resourceVersion": "43"}}
	for _, meta := range []map[string]any{{"uid": "new", "resourceVersion": "42"}, {"uid": "old", "resourceVersion": "43"}} {
		if err := applyK8sResourceVersion(map[string]any{"metadata": meta}, current); err == nil {
			t.Fatalf("stale edit accepted: %+v", meta)
		}
	}
	matching := map[string]any{"metadata": map[string]any{"uid": "new", "resourceVersion": "43"}}
	if err := applyK8sResourceVersion(matching, current); err != nil {
		t.Fatal(err)
	}
}

func TestPodDeleteRequiresUIDAndExplicitZeroGraceConfirmation(t *testing.T) {
	zero := int64(0)
	for _, payload := range []model.K8sResourceDeletePayload{{ResourceType: "pod"}, {ResourceType: "pod", UID: "old", Force: true}, {ResourceType: "pod", UID: "old", GracePeriodSeconds: &zero}, {ResourceType: "service", Force: true}} {
		if _, err := buildK8sDeleteOptions(payload); err == nil {
			t.Fatalf("unsafe deletion accepted: %+v", payload)
		}
	}
	body, err := buildK8sDeleteOptions(model.K8sResourceDeletePayload{ResourceType: "pod", UID: "old", Force: true, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	var options map[string]any
	json.Unmarshal(body, &options)
	if options["gracePeriodSeconds"] != float64(0) || options["preconditions"].(map[string]any)["uid"] != "old" {
		t.Fatalf("unsafe delete options: %s", body)
	}
	if _, ok := options["finalizers"]; ok {
		t.Fatal("must never remove finalizers implicitly")
	}
	ordinary, _ := buildK8sDeleteOptions(model.K8sResourceDeletePayload{ResourceType: "pod", UID: "old"})
	var graceful map[string]any
	json.Unmarshal(ordinary, &graceful)
	if _, ok := graceful["gracePeriodSeconds"]; ok {
		t.Fatal("default grace period overridden")
	}
}

func TestK8sDryRunAndDeleteOptionsReachAPIServer(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodPost && (r.URL.Query().Get("dryRun") != "All" || r.URL.Query().Get("fieldValidation") != "Strict") {
			t.Error("dry-run validation omitted")
		}
		if r.Method == http.MethodDelete {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["preconditions"].(map[string]any)["uid"] != "old" {
				t.Error("UID precondition omitted")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	runtime := kubeClusterRuntime{Server: server.URL}
	if err := k8sDoJSON(server.Client(), runtime, http.MethodPost, "/api/v1/pods", k8sDryRunQuery(true), []byte(`{}`), "application/json", nil); err != nil {
		t.Fatal(err)
	}
	body, _ := buildK8sDeleteOptions(model.K8sResourceDeletePayload{ResourceType: "pod", UID: "old", Force: true, Confirmed: true})
	if err := k8sDoJSON(server.Client(), runtime, http.MethodDelete, "/api/v1/namespaces/dev/pods/app", nil, body, "application/json", nil); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("unexpected calls: %d", requests)
	}
}
