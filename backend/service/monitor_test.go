package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func TestShouldNotifyMonitorRecoveryOnlyAfterTriggerNotification(t *testing.T) {
	rule := model.MonitorAlertRule{
		NotifyEnabled:         true,
		NotifyRuleID:          1,
		NotifyRecoveryEnabled: true,
	}
	if shouldNotifyMonitorRecovery(rule, model.MonitorAlertEvent{Status: "pending"}) {
		t.Fatal("a pending event that recovered before firing must not send a recovery notification")
	}

	notifiedAt := time.Now()
	if !shouldNotifyMonitorRecovery(rule, model.MonitorAlertEvent{Status: "firing", LastNotifyAt: &notifiedAt, NotifyCount: 1}) {
		t.Fatal("an event with a sent firing notification should send a recovery notification")
	}
}

func TestNormalizeMonitorDatasourceTypeJaeger(t *testing.T) {
	for _, value := range []string{"jaeger", "Jaeger-Query", "tracing"} {
		if got := normalizeMonitorDatasourceType(value); got != "jaeger" {
			t.Fatalf("normalizeMonitorDatasourceType(%q) = %q, want jaeger", value, got)
		}
	}
	if !isMonitorTraceDatasource("jaeger") {
		t.Fatal("Jaeger should be recognized as a trace datasource")
	}
}

func TestJaegerHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/services" {
			t.Fatalf("unexpected Jaeger health path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":["ops-admin"]}`))
	}))
	defer server.Close()

	if err := (&Service{}).jaegerHealth(model.MonitorDatasource{URL: server.URL}); err != nil {
		t.Fatalf("Jaeger health check failed: %v", err)
	}
}

func TestNormalizeMonitorLogPagination(t *testing.T) {
	if page, size := normalizeMonitorLogPagination(0, 0); page != 1 || size != 200 {
		t.Fatalf("default pagination = (%d, %d), want (1, 200)", page, size)
	}
	if page, size := normalizeMonitorLogPagination(3, 1200); page != 3 || size != 1000 {
		t.Fatalf("capped pagination = (%d, %d), want (3, 1000)", page, size)
	}
}

func TestFormatMonitorOpLogContentHidesTransportFields(t *testing.T) {
	raw := `{"@timestamp":"2026-07-15T01:41:18.013Z","actionType":14,"source_type":"kafka","timestamp":"2026-07-15T01:41:18.013Z","ts":"2026-07-15T09:41:18.013+08:00","kafka_topic":"app.op.log.0"}`
	content := formatMonitorOpLogContent(map[string]any{"kafka_topic": "app.op.log.0", "message": raw}, "app-oplog", raw)
	var document map[string]any
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("expected valid compact JSON, got %q: %v", content, err)
	}
	for _, field := range []string{"@timestamp", "source_type", "timestamp", "ts"} {
		if _, exists := document[field]; exists {
			t.Fatalf("field %s should be hidden: %#v", field, document)
		}
	}
	if document["actionType"] != float64(14) || document["kafka_topic"] != "app.op.log.0" {
		t.Fatalf("business fields should be preserved: %#v", document)
	}
}

func TestParseMonitorLogMessage(t *testing.T) {
	fields := parseMonitorLogMessage("2026-07-10 14:20:56.431 INFO 1 --- [worker-5] c.example.CrossTransportHandler : keepalive received")
	if fields["level"] != "INFO" || fields["thread"] != "worker-5" {
		t.Fatalf("unexpected parsed fields: %#v", fields)
	}
	if fields["logger"] != "c.example.CrossTransportHandler" || fields["content"] != "keepalive received" {
		t.Fatalf("message content was not split correctly: %#v", fields)
	}
}

func TestParseMonitorLogMessageFallsBackToRawContent(t *testing.T) {
	fields := parseMonitorLogMessage("plain log message")
	if fields["content"] != "plain log message" {
		t.Fatalf("expected raw message fallback, got %#v", fields)
	}
}

func TestApplyMonitorEventAggregationUpdatesPersistedAndInMemoryState(t *testing.T) {
	event := model.MonitorAlertEvent{}
	updates := map[string]any{}
	aggregation := model.MonitorAggregationRule{ID: 7, Name: "按实例收敛"}

	applyMonitorEventAggregation(&event, updates, aggregation, "aggregation=7|instance=10.0.0.1:9100")

	if event.AggregationKey == "" || event.AggregateRuleID != 7 || event.AggregateRuleName != aggregation.Name {
		t.Fatalf("in-memory aggregation state was not updated: %#v", event)
	}
	if updates["aggregation_key"] != event.AggregationKey || updates["aggregate_rule_id"] != aggregation.ID {
		t.Fatalf("database updates and in-memory state differ: %#v", updates)
	}
}

func TestAggregationLookbackCoversRepeatInterval(t *testing.T) {
	aggregation := model.MonitorAggregationRule{WindowSeconds: 300, RepeatIntervalSeconds: 1800}
	if got := aggregationLookbackSeconds(aggregation); got != 1800 {
		t.Fatalf("expected repeat interval lookback, got %d", got)
	}
}

func TestAggregationLookbackCoversConvergenceWindow(t *testing.T) {
	aggregation := model.MonitorAggregationRule{WindowSeconds: 900, RepeatIntervalSeconds: 300}
	if got := aggregationLookbackSeconds(aggregation); got != 900 {
		t.Fatalf("expected convergence window lookback, got %d", got)
	}
}

func TestMonitorRuleEvaluationCannotOverlap(t *testing.T) {
	service := &Service{serviceRuntime: &serviceRuntime{monitorScheduler: &MonitorScheduler{running: map[uint]bool{}}}}
	if !service.beginMonitorAlertRuleEvaluation(9) {
		t.Fatal("first evaluation should start")
	}
	if service.beginMonitorAlertRuleEvaluation(9) {
		t.Fatal("overlapping evaluation should be rejected")
	}
	service.endMonitorAlertRuleEvaluation(9)
	if !service.beginMonitorAlertRuleEvaluation(9) {
		t.Fatal("evaluation should start after the previous run finishes")
	}
}
