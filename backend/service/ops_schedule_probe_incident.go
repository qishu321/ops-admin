package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ops-admin/backend/model"
)

// A probe incident is stored on the task, not inferred from retained run logs.
// Log retention must not make an ongoing outage look like a new outage.
type probeIncidentDecision struct {
	Event             string
	Reminder          bool
	FailureStreak     int
	SuccessStreak     int
	RecoveryThreshold int
}

func advanceProbeIncident(task *model.OpsScheduleTask, result string, now time.Time) probeIncidentDecision {
	decision := probeIncidentDecision{}
	canNotify := task.NotifyEnabled && task.NotifyRuleID > 0
	switch strings.ToLower(result) {
	case "failed":
		task.ProbeSuccessStreak = 0
		if task.ProbeFailureStreak < 1_000_000 {
			task.ProbeFailureStreak++
		}
		if !task.ProbeIncidentOpen && task.ProbeFailureStreak >= normalizeProbeThreshold(task.ProbeFailureThreshold) {
			task.ProbeIncidentOpen = true
			task.ProbeIncidentStartedAt = &now
		}
		if task.ProbeIncidentOpen && canNotify {
			if !task.ProbeIncidentNotified {
				decision.Event = "failed"
				task.ProbeIncidentNotified = true
				task.ProbeLastNotifyAt = &now
			} else if minutes := normalizeProbeReminderMinutes(task.ProbeReminderMinutes); minutes > 0 &&
				(task.ProbeLastNotifyAt == nil || now.Sub(*task.ProbeLastNotifyAt) >= time.Duration(minutes)*time.Minute) {
				decision.Event = "failed"
				decision.Reminder = true
				task.ProbeLastNotifyAt = &now
			}
		}
	case "success":
		task.ProbeFailureStreak = 0
		if task.ProbeIncidentOpen {
			task.ProbeSuccessStreak++
			if task.ProbeSuccessStreak >= normalizeProbeThreshold(task.ProbeRecoveryThreshold) {
				if task.ProbeIncidentNotified && canNotify {
					decision.Event = "recovered"
				}
				task.ProbeIncidentOpen = false
				task.ProbeIncidentNotified = false
				task.ProbeIncidentStartedAt = nil
				task.ProbeLastNotifyAt = nil
				task.ProbeSuccessStreak = 0
			}
		} else {
			task.ProbeSuccessStreak = 0
		}
	}
	decision.FailureStreak = task.ProbeFailureStreak
	decision.SuccessStreak = task.ProbeSuccessStreak
	decision.RecoveryThreshold = normalizeProbeThreshold(task.ProbeRecoveryThreshold)
	return decision
}

// Compare-and-swap the persisted state. This serializes overlapping cron runs
// across processes and ignores a late result from an older execution.
func (s *Service) recordScheduledProbeResult(snapshot model.OpsScheduleTask, logID uint, result, summary string, now time.Time, nextRunAt *time.Time) (probeIncidentDecision, error) {
	if logID == 0 || (result != "success" && result != "failed") {
		return probeIncidentDecision{}, nil
	}
	for attempt := 0; attempt < 8; attempt++ {
		var current model.OpsScheduleTask
		if err := s.db.First(&current, snapshot.ID).Error; err != nil {
			return probeIncidentDecision{}, err
		}
		if current.ProbeLastProcessedLogID >= logID || current.ProbeStateEpoch != snapshot.ProbeStateEpoch ||
			current.Status != 1 || current.TaskType != "http" || current.URL != snapshot.URL ||
			current.HTTPMethod != snapshot.HTTPMethod || current.ExpectedStatus != snapshot.ExpectedStatus ||
			current.NotifyRuleID != snapshot.NotifyRuleID || current.NotifyEnabled != snapshot.NotifyEnabled {
			return probeIncidentDecision{}, nil
		}
		// Do not mark an incident as notified when its rule cannot route a
		// failure. Otherwise a later recovery could become an orphan message.
		if current.NotifyEnabled && current.NotifyRuleID > 0 {
			var route model.NotifyRule
			if err := s.db.Select("id", "status", "scope", "events_json").First(&route, current.NotifyRuleID).Error; err != nil {
				return probeIncidentDecision{}, err
			}
			if route.Status != 1 || (normalizeNotifyScope(route.Scope) != "all" && normalizeNotifyScope(route.Scope) != "schedule") ||
				!notifyEventMatch(decodeStringList(route.EventsJSON), "failed", "failed") {
				current.NotifyEnabled = false
			}
		}
		previousLogID := current.ProbeLastProcessedLogID
		decision := advanceProbeIncident(&current, result, now)
		updates := map[string]any{
			"last_status":                 result,
			"last_summary":                summary,
			"last_run_at":                 &now,
			"next_run_at":                 nextRunAt,
			"probe_failure_streak":        current.ProbeFailureStreak,
			"probe_success_streak":        current.ProbeSuccessStreak,
			"probe_incident_open":         current.ProbeIncidentOpen,
			"probe_incident_notified":     current.ProbeIncidentNotified,
			"probe_incident_started_at":   current.ProbeIncidentStartedAt,
			"probe_last_notify_at":        current.ProbeLastNotifyAt,
			"probe_last_processed_log_id": logID,
		}
		update := s.db.Model(&model.OpsScheduleTask{}).
			Where("id = ? AND probe_last_processed_log_id = ? AND probe_state_epoch = ?", snapshot.ID, previousLogID, snapshot.ProbeStateEpoch).
			Updates(updates)
		if update.Error != nil {
			return probeIncidentDecision{}, update.Error
		}
		if update.RowsAffected == 1 {
			return decision, nil
		}
	}
	return probeIncidentDecision{}, errors.New("HTTP 探针告警状态更新冲突")
}

func probeNotifySummary(task model.OpsScheduleTask, decision probeIncidentDecision, runSummary string) string {
	switch {
	case decision.Event == "recovered":
		return fmt.Sprintf("%s 已恢复：连续 %d 次探针成功", task.Name, decision.RecoveryThreshold)
	case decision.Reminder:
		return fmt.Sprintf("%s 持续故障：连续 %d 次探针失败；%s", task.Name, decision.FailureStreak, runSummary)
	case decision.Event == "failed":
		return fmt.Sprintf("%s 故障触发：连续 %d 次探针失败；%s", task.Name, decision.FailureStreak, runSummary)
	default:
		return runSummary
	}
}
