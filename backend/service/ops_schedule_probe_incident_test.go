package service

import (
	"strings"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func probeIncidentTestTask() model.OpsScheduleTask {
	return model.OpsScheduleTask{
		Name: "核心接口", TaskType: "http", NotifyEnabled: true, NotifyRuleID: 1,
		ProbeFailureThreshold: 2, ProbeRecoveryThreshold: 2,
	}
}

func TestProbeIncidentNotifiesOncePerOutageAndOnceOnRecovery(t *testing.T) {
	task := probeIncidentTestTask()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.Local)
	checks := []struct {
		result string
		event  string
		open   bool
	}{
		{"failed", "", false},
		{"failed", "failed", true},
		{"failed", "", true},
		{"success", "", true},
		{"failed", "", true},
		{"success", "", true},
		{"success", "recovered", false},
		{"success", "", false},
		{"failed", "", false},
		{"failed", "failed", true},
	}
	for index, check := range checks {
		decision := advanceProbeIncident(&task, check.result, now.Add(time.Duration(index)*5*time.Minute))
		if decision.Event != check.event || task.ProbeIncidentOpen != check.open {
			t.Fatalf("check %d (%s): event=%q open=%v, want event=%q open=%v", index+1, check.result, decision.Event, task.ProbeIncidentOpen, check.event, check.open)
		}
	}
}

func TestProbeIncidentReminderIsOptInAndRateLimited(t *testing.T) {
	task := probeIncidentTestTask()
	task.ProbeReminderMinutes = 60
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.Local)
	advanceProbeIncident(&task, "failed", now)
	if decision := advanceProbeIncident(&task, "failed", now.Add(5*time.Minute)); decision.Event != "failed" || decision.Reminder {
		t.Fatalf("second failure should open incident: %+v", decision)
	}
	if decision := advanceProbeIncident(&task, "failed", now.Add(55*time.Minute)); decision.Event != "" {
		t.Fatalf("reminder sent too early: %+v", decision)
	}
	if decision := advanceProbeIncident(&task, "failed", now.Add(65*time.Minute)); decision.Event != "failed" || !decision.Reminder {
		t.Fatalf("expected one reminder after interval: %+v", decision)
	}
	if decision := advanceProbeIncident(&task, "failed", now.Add(70*time.Minute)); decision.Event != "" {
		t.Fatalf("reminder repeated before next interval: %+v", decision)
	}
	task.ProbeReminderMinutes = 0
	if decision := advanceProbeIncident(&task, "failed", now.Add(24*time.Hour)); decision.Event != "" {
		t.Fatalf("disabled reminder should not send: %+v", decision)
	}
}

func TestProbeIncidentDoesNotSendOrphanRecovery(t *testing.T) {
	task := probeIncidentTestTask()
	task.NotifyEnabled = false
	now := time.Now()
	advanceProbeIncident(&task, "failed", now)
	advanceProbeIncident(&task, "failed", now.Add(time.Minute))
	if !task.ProbeIncidentOpen || task.ProbeIncidentNotified {
		t.Fatalf("unnotified incident state unexpected: %+v", task)
	}
	advanceProbeIncident(&task, "success", now.Add(2*time.Minute))
	if decision := advanceProbeIncident(&task, "success", now.Add(3*time.Minute)); decision.Event != "" || task.ProbeIncidentOpen {
		t.Fatalf("unnotified incident must close silently: %+v", decision)
	}
}

func TestProbeNotificationPreviewAllowsRecoveryButNotSuccess(t *testing.T) {
	payload := OpsScheduleTaskPayload{TaskType: "http", Name: "核心接口", NotifyOnFailureOnly: true, ProbeRecoveryThreshold: 2, ExpectedStatus: 200}
	if _, err := buildOpsScheduleNotifyPreviewEvent(payload, "success"); err == nil {
		t.Fatal("HTTP probe must not preview a per-run success notification")
	}
	event, err := buildOpsScheduleNotifyPreviewEvent(payload, "recovered")
	if err != nil {
		t.Fatal(err)
	}
	if event.Event != "recovered" || event.Status != "recovered" || !strings.Contains(event.Summary, "连续 2 次") {
		t.Fatalf("unexpected recovery preview: %+v", event)
	}
}
