package service

import (
	"errors"
	"fmt"
	"strings"

	"ops-admin/backend/model"
)

type scriptFailureDecision struct {
	Notify        bool
	Reminder      bool
	FailureStreak int
}

func normalizeScriptReminderFailures(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func advanceScriptFailure(task *model.OpsScheduleTask, result string, canNotify bool) scriptFailureDecision {
	decision := scriptFailureDecision{}
	if strings.EqualFold(result, "success") {
		task.ScriptFailureStreak = 0
		task.ScriptNotifiedAtStreak = 0
		return decision
	}
	if !strings.EqualFold(result, "failed") {
		return decision
	}
	if task.ScriptFailureStreak < 1_000_000 {
		task.ScriptFailureStreak++
	}
	decision.FailureStreak = task.ScriptFailureStreak
	if !canNotify || task.ScriptFailureStreak < normalizeProbeThreshold(task.ScriptFailureThreshold) {
		return decision
	}
	if task.ScriptNotifiedAtStreak == 0 {
		decision.Notify = true
	} else if interval := normalizeScriptReminderFailures(task.ScriptReminderFailures); interval > 0 &&
		task.ScriptFailureStreak-task.ScriptNotifiedAtStreak >= interval {
		decision.Notify = true
		decision.Reminder = true
	}
	if decision.Notify {
		task.ScriptNotifiedAtStreak = task.ScriptFailureStreak
	}
	return decision
}

// Persist the notification decision atomically so overlapping runs cannot
// produce duplicate alerts. Older completions never rewind the streak.
func (s *Service) recordScheduledScriptResult(snapshot model.OpsScheduleTask, logID uint, result string) (scriptFailureDecision, error) {
	if logID == 0 || (result != "success" && result != "failed") {
		return scriptFailureDecision{}, nil
	}
	for attempt := 0; attempt < 8; attempt++ {
		var current model.OpsScheduleTask
		if err := s.db.First(&current, snapshot.ID).Error; err != nil {
			return scriptFailureDecision{}, err
		}
		if current.ScriptLastProcessedLogID >= logID || current.ScriptStateEpoch != snapshot.ScriptStateEpoch ||
			current.Status != 1 || current.TaskType != "script" || current.ScriptID != snapshot.ScriptID ||
			current.NotifyEnabled != snapshot.NotifyEnabled || current.NotifyRuleID != snapshot.NotifyRuleID ||
			current.NotifyOnFailureOnly != snapshot.NotifyOnFailureOnly {
			return scriptFailureDecision{}, nil
		}
		canNotify := current.NotifyEnabled && current.NotifyOnFailureOnly && current.NotifyRuleID > 0
		if canNotify {
			var route model.NotifyRule
			if err := s.db.Select("id", "status", "scope", "events_json").First(&route, current.NotifyRuleID).Error; err != nil {
				return scriptFailureDecision{}, err
			}
			canNotify = route.Status == 1 && (normalizeNotifyScope(route.Scope) == "all" || normalizeNotifyScope(route.Scope) == "schedule") &&
				notifyEventMatch(decodeStringList(route.EventsJSON), "failed", "failed")
		}
		previousLogID := current.ScriptLastProcessedLogID
		decision := advanceScriptFailure(&current, result, canNotify)
		update := s.db.Model(&model.OpsScheduleTask{}).
			Where("id = ? AND script_last_processed_log_id = ? AND script_state_epoch = ?", snapshot.ID, previousLogID, snapshot.ScriptStateEpoch).
			Updates(map[string]any{
				"script_failure_streak":        current.ScriptFailureStreak,
				"script_notified_at_streak":    current.ScriptNotifiedAtStreak,
				"script_last_processed_log_id": logID,
			})
		if update.Error != nil {
			return scriptFailureDecision{}, update.Error
		}
		if update.RowsAffected == 1 {
			return decision, nil
		}
	}
	return scriptFailureDecision{}, errors.New("脚本任务失败通知状态更新冲突")
}

func scriptFailureSummary(task model.OpsScheduleTask, decision scriptFailureDecision, runSummary string) string {
	if decision.Reminder {
		return fmt.Sprintf("%s 持续失败提醒：连续 %d 次执行失败；%s", task.Name, decision.FailureStreak, runSummary)
	}
	return fmt.Sprintf("%s 连续失败告警：连续 %d 次执行失败；%s", task.Name, decision.FailureStreak, runSummary)
}
