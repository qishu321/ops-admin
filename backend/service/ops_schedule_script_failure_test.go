package service

import (
	"testing"

	"ops-admin/backend/model"
)

func TestScriptFailureNotifiesOnceUntilSuccess(t *testing.T) {
	task := model.OpsScheduleTask{ScriptFailureThreshold: 2}
	checks := []struct {
		result string
		notify bool
		streak int
	}{
		{"failed", false, 1},
		{"failed", true, 2},
		{"failed", false, 3},
		{"failed", false, 4},
		{"success", false, 0},
		{"failed", false, 1},
		{"failed", true, 2},
	}
	for index, check := range checks {
		decision := advanceScriptFailure(&task, check.result, true)
		if decision.Notify != check.notify || task.ScriptFailureStreak != check.streak {
			t.Fatalf("step %d: decision=%+v streak=%d, want notify=%v streak=%d", index, decision, task.ScriptFailureStreak, check.notify, check.streak)
		}
	}
}

func TestScriptFailureReminderUsesAdditionalFailures(t *testing.T) {
	task := model.OpsScheduleTask{ScriptFailureThreshold: 2, ScriptReminderFailures: 3}
	for streak := 1; streak <= 9; streak++ {
		decision := advanceScriptFailure(&task, "failed", true)
		want := streak == 2 || streak == 5 || streak == 8
		if decision.Notify != want || (decision.Notify && decision.Reminder != (streak > 2)) {
			t.Fatalf("failure %d: decision=%+v, want notify=%v", streak, decision, want)
		}
	}
}

func TestScriptFailureDoesNotMarkUndeliverableAlertAsSent(t *testing.T) {
	task := model.OpsScheduleTask{ScriptFailureThreshold: 2}
	advanceScriptFailure(&task, "failed", false)
	advanceScriptFailure(&task, "failed", false)
	if task.ScriptNotifiedAtStreak != 0 {
		t.Fatal("undeliverable failure must not be marked notified")
	}
	if decision := advanceScriptFailure(&task, "failed", true); !decision.Notify || decision.FailureStreak != 3 {
		t.Fatalf("first routable failure should notify: %+v", decision)
	}
}
