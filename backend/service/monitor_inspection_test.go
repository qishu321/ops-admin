package service

import (
	"testing"

	"ops-admin/backend/model"
)

func TestReduceInspectionResultUsesWholeWindow(t *testing.T) {
	result := &PromQueryResult{}
	result.Data.Result = []PromMetricSample{
		{Values: [][]any{{1, "10"}, {2, "20"}, {3, "30"}}},
		{Values: [][]any{{1, "5"}, {2, "15"}, {3, "25"}}},
	}

	value, stats, seriesCount, sampleCount, ok := reduceInspectionResult(result, "increase", "gte")
	if !ok || value != 20 || stats.Max != 30 || seriesCount != 2 || sampleCount != 6 {
		t.Fatalf("unexpected reduction: ok=%v value=%v max=%v series=%d samples=%d", ok, value, stats.Max, seriesCount, sampleCount)
	}
	value, _, _, _, _ = reduceInspectionResult(result, "p95", "gte")
	if value != 30 {
		t.Fatalf("expected p95 30, got %v", value)
	}
}

func TestInspectionRuleThresholdsAndLegacyDefaults(t *testing.T) {
	rule := inspectionRuleFor(model.MonitorDashboardPanel{Title: "CPU 高负载"})
	if rule.reducer != "p95" || inspectionStatus(85, rule) != "warning" || inspectionStatus(95, rule) != "danger" {
		t.Fatalf("unexpected CPU inspection rule: %#v", rule)
	}

	lowRule := inspectionRule{kind: "rule", reducer: "min", operator: "lte", warning: 2, critical: 1, configured: true}
	if inspectionStatus(1, lowRule) != "danger" || inspectionStatus(1.5, lowRule) != "warning" || inspectionStatus(3, lowRule) != "healthy" {
		t.Fatal("low-is-bad thresholds were not evaluated in the expected order")
	}
}

func TestFormatInspectionValueKeepsIntegerZeros(t *testing.T) {
	if got := formatInspectionValue(100, "%"); got != "100%" {
		t.Fatalf("expected 100%%, got %q", got)
	}
	if got := formatInspectionValue(0, "个"); got != "0个" {
		t.Fatalf("expected 0个, got %q", got)
	}
}
