package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func TestMonitorContinuityDoesNotCountDowntime(t *testing.T) {
	now := time.Now()
	started := now.Add(-2 * time.Hour)
	if got := monitorContinuousSince(&started, now.Add(-time.Minute), now, 60); !got.Equal(started) {
		t.Fatal("healthy continuity reset")
	}
	if got := monitorContinuousSince(&started, now.Add(-time.Hour), now, 60); !got.Equal(now) {
		t.Fatal("downtime counted toward duration")
	}
	if got := monitorContinuousSince(nil, now.Add(-time.Minute), now, 60); !got.Equal(now) {
		t.Fatal("unknown interval counted toward duration")
	}
}

func TestMonitorRecoveryRequiresEvidenceAfterHistoryExpires(t *testing.T) {
	rule := model.MonitorAlertRule{Comparator: "==", Threshold: 0, RecoveryQuery: "ready", RecoveryComparator: "==", RecoveryThreshold: 1, MissingDataPolicy: "alert"}
	// Empty historical and current results must never recover an existing event,
	// regardless of how long it has been open or whether the backend restarted.
	for _, missing := range []map[string]PromMetricSample{nil, {}} {
		if ok, _ := monitorRecoveryDecision(rule, false, 0, missing, "pod"); ok {
			t.Fatal("missing series recovered")
		}
	}
	for _, tc := range []struct {
		value     string
		recovered bool
	}{{"0", false}, {"1", true}, {"NaN", false}, {"+Inf", false}} {
		results := map[string]PromMetricSample{"pod": {Value: []any{1, tc.value}}}
		if ok, _ := monitorRecoveryDecision(rule, true, 0, results, "pod"); ok != tc.recovered {
			t.Fatalf("%s: recovered=%v", tc.value, ok)
		}
	}
}

func TestMonitorThresholdRecoveryAndMissingPolicy(t *testing.T) {
	rule := model.MonitorAlertRule{Comparator: ">", Threshold: 85}
	for _, tc := range []struct {
		present bool
		value   float64
		want    bool
	}{{true, 90, false}, {true, 85, true}, {false, 0, false}, {true, 0, true}} {
		if got, _ := monitorRecoveryDecision(rule, tc.present, tc.value, nil, ""); got != tc.want {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
	rule.MissingDataPolicy = "recover"
	if got, _ := monitorRecoveryDecision(rule, false, 0, nil, ""); !got {
		t.Fatal("explicit compatibility policy ignored")
	}
}

func TestMonitorIdentityRecreationAndDatasourceIsolation(t *testing.T) {
	rule := model.MonitorAlertRule{ID: 1, DatasourceID: 2, IdentityLabels: "cluster,namespace,pod,container"}
	old := map[string]string{"namespace": "game", "pod": "world-0", "container": "world", "uid": "old", "__name__": "waiting"}
	next := map[string]string{"namespace": "game", "pod": "world-0", "container": "world", "uid": "new", "__name__": "ready"}
	a := monitorTargetFingerprint(rule, old)
	if a != monitorTargetFingerprint(rule, next) {
		t.Fatal("replacement UID or recovery metric split the target")
	}
	rule.DatasourceID = 3
	if a == monitorTargetFingerprint(rule, next) {
		t.Fatal("different datasource matched")
	}
	rule.DatasourceID = 2
	next["pod"] = "world-1"
	if a == monitorTargetFingerprint(rule, next) {
		t.Fatal("unrelated pod matched")
	}
}

func TestMonitorQueryFailureDoesNotEnterRecovery(t *testing.T) {
	for _, response := range []struct {
		code int
		body string
	}{{500, "failure"}, {200, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"NaN"]}]}}`}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(response.code)
			w.Write([]byte(response.body))
		}))
		// No DB: a failed/invalid query must return before any event mutation.
		_, err := (&Service{}).evaluateMonitorDatasource(model.MonitorAlertRule{PromQL: "ready"}, model.MonitorDatasource{URL: server.URL})
		server.Close()
		if err == nil {
			t.Fatal("failure was treated as a successful evaluation")
		}
	}
}

func TestMonitorMissingHealthDoesNotCreatePodFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()
	_, err := (&Service{}).evaluateMonitorDatasource(model.MonitorAlertRule{PromQL: "ready", HealthQuery: "up"}, model.MonitorDatasource{URL: server.URL})
	if err == nil {
		t.Fatal("empty health query allowed synthetic pod failures")
	}
}

func TestMonitorRuleLabelsAndPolicyValidation(t *testing.T) {
	labels := monitorEventLabels(model.MonitorAlertRule{LabelsJSON: `{"category":"szfc","pod":"wrong"}`, DatasourceName: "Shanghai"}, map[string]string{"pod": "world-0"})
	if labels["category"] != "szfc" || labels["pod"] != "world-0" {
		t.Fatal(labels)
	}
	if validateMonitorRecoveryPolicy(MonitorAlertRulePayload{MissingDataPolicy: "alert"}) == nil {
		t.Fatal("unsafe missing policy accepted")
	}
	p := MonitorAlertRulePayload{MissingDataPolicy: "alert", Comparator: "==", Threshold: 0, RecoveryQuery: "ready", RecoveryComparator: "==", RecoveryThreshold: 1, IdentityLabels: "namespace,pod,container", HealthQuery: "up", DiscoveryWindowSeconds: 3600}
	if err := validateMonitorRecoveryPolicy(p); err != nil {
		t.Fatal(err)
	}
}
