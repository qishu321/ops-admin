package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ops-admin/backend/model"
)

func validateMonitorRecoveryPolicy(p MonitorAlertRulePayload) error {
	if p.MissingDataPolicy != "" && p.MissingDataPolicy != "keep" && p.MissingDataPolicy != "recover" && p.MissingDataPolicy != "alert" {
		return errors.New("无数据策略无效")
	}
	if p.RecoveryForSeconds < 0 || p.RecoveryForSeconds > 86400 || p.DiscoveryWindowSeconds < 0 || p.DiscoveryWindowSeconds > 86400 {
		return errors.New("恢复持续时间和历史窗口须在 0 到 86400 秒之间")
	}
	if p.RecoveryQuery != "" {
		switch p.RecoveryComparator {
		case "==", "!=", ">", ">=", "<", "<=":
		default:
			return errors.New("请选择恢复比较符")
		}
		if strings.TrimSpace(p.IdentityLabels) == "" {
			return errors.New("独立恢复查询需要指定对象关联标签")
		}
	}
	if p.MissingDataPolicy == "alert" && (strings.TrimSpace(p.HealthQuery) == "" || p.DiscoveryWindowSeconds == 0 || strings.TrimSpace(p.RecoveryQuery) == "" || p.Comparator != "==" || p.Threshold != 0) {
		return errors.New("缺失告警仅适用于值为 0 时异常的可用性规则，需配置采集健康查询、历史窗口及明确恢复查询")
	}
	for _, key := range strings.Split(p.IdentityLabels, ",") {
		key = strings.TrimSpace(key)
		if key == "__name__" || key == "datasource" || key == "uid" {
			return errors.New("对象关联标签不能使用指标名、数据源名或 UID")
		}
	}
	return nil
}

// Explicit labels allow trigger and recovery queries to use different metrics.
// Datasource IDs, rather than names, keep unrelated clusters isolated.
func monitorTargetFingerprint(rule model.MonitorAlertRule, labels map[string]string) string {
	if strings.TrimSpace(rule.IdentityLabels) == "" {
		copied := map[string]string{}
		for k, v := range labels {
			copied[k] = v
		}
		copied["datasource"] = rule.DatasourceName
		return monitorFingerprint(rule.ID, copied)
	}
	identity := map[string]string{"datasource_id": fmt.Sprint(rule.DatasourceID)}
	for _, key := range strings.Split(rule.IdentityLabels, ",") {
		if key = strings.TrimSpace(key); key != "" {
			identity[key] = labels[key]
		}
	}
	return monitorFingerprint(rule.ID, identity)
}

func monitorEventLabels(rule model.MonitorAlertRule, sample map[string]string) map[string]string {
	labels := decodeLabelMap(rule.LabelsJSON)
	if labels == nil {
		labels = map[string]string{}
	}
	for k, v := range sample {
		labels[k] = v
	}
	labels["datasource"] = rule.DatasourceName
	return labels
}

// Do not count scheduler downtime or missed evaluations as verified continuity.
func monitorContinuousSince(since *time.Time, last time.Time, now time.Time, interval int) *time.Time {
	if since == nil || last.IsZero() || now.Sub(last) > 2*time.Duration(normalizeEvalInterval(interval))*time.Second {
		return &now
	}
	return since
}

func (s *Service) markMonitorSourceUnknown(rule model.MonitorAlertRule) {
	s.db.Model(&model.MonitorAlertEvent{}).Where("rule_id = ? AND datasource_id = ? AND status IN ?", rule.ID, rule.DatasourceID, []string{"pending", "firing", "claimed", "silenced"}).Updates(map[string]any{"observation_state": "unknown", "pending_since": nil, "recovery_since": nil, "last_evaluated_at": time.Now()})
}

func monitorSampleMap(rule model.MonitorAlertRule, samples []PromMetricSample) (map[string]PromMetricSample, error) {
	result := map[string]PromMetricSample{}
	for _, sample := range samples {
		if strings.TrimSpace(rule.IdentityLabels) != "" {
			found := false
			for _, key := range strings.Split(rule.IdentityLabels, ",") {
				if _, ok := sample.Metric[strings.TrimSpace(key)]; ok {
					found = true
				}
			}
			if !found {
				return nil, errors.New("查询结果没有对象关联标签")
			}
		}
		value, valid := promSampleValue(sample)
		if !valid {
			return nil, errors.New("查询包含无效数值")
		}
		fp := monitorTargetFingerprint(rule, sample.Metric)
		if previous, ok := result[fp]; ok {
			previousValue, _ := promSampleValue(previous)
			if value != previousValue {
				return nil, errors.New("对象关联标签不唯一，请补充集群或实例标签")
			}
		}
		result[fp] = sample
	}
	return result, nil
}

func (s *Service) evaluateMonitorDatasource(rule model.MonitorAlertRule, ds model.MonitorDatasource) (int, error) {
	now := time.Now()
	query := func(expr string) (map[string]PromMetricSample, error) {
		result, err := s.prometheusQuery(ds, expr, now)
		if err != nil {
			return nil, err
		}
		return monitorSampleMap(rule, result.Data.Result)
	}
	if strings.TrimSpace(rule.HealthQuery) != "" {
		result, err := s.prometheusQuery(ds, rule.HealthQuery, now)
		if err != nil || len(result.Data.Result) == 0 {
			return 0, errors.New("无法确认采集健康状态")
		}
		for _, sample := range result.Data.Result {
			value, ok := promSampleValue(sample)
			if !ok || value != 1 {
				return 0, errors.New("采集不健康")
			}
		}
	}
	var current map[string]PromMetricSample
	var err error
	if isMonitorLogAlertType(rule.AlertType) {
		value, sample, e := s.monitorLogAlertValue(rule, ds)
		if e != nil {
			return 0, e
		}
		sample.Value = []any{now.Unix(), fmt.Sprint(value)}
		current, err = monitorSampleMap(rule, []PromMetricSample{sample})
	} else {
		current, err = query(rule.PromQL)
	}
	if err != nil {
		return 0, err
	}
	recovery := current
	if rule.RecoveryQuery != "" {
		recovery, err = query(rule.RecoveryQuery)
		if err != nil {
			return 0, err
		}
	}
	history := map[string]PromMetricSample{}
	if rule.MissingDataPolicy == "alert" {
		history, err = query(fmt.Sprintf("max_over_time((%s)[%ds:%ds])", rule.PromQL, rule.DiscoveryWindowSeconds, normalizeEvalInterval(rule.EvalIntervalSeconds)))
		if err != nil {
			return 0, err
		}
	}
	var events []model.MonitorAlertEvent
	if err = s.db.Where("rule_id = ? AND datasource_id = ? AND status IN ?", rule.ID, ds.ID, []string{"pending", "firing", "claimed", "silenced"}).Find(&events).Error; err != nil {
		return 0, err
	}
	existing := map[string]model.MonitorAlertEvent{}
	keep := map[string]bool{}
	for _, event := range events {
		fp := event.Fingerprint
		if rule.IdentityLabels != "" {
			fp = monitorTargetFingerprint(rule, decodeLabelMap(event.LabelsJSON))
		}
		// Reconcile older UID-based identities before upserting the same target.
		if fp != event.Fingerprint {
			if err := s.db.Model(&event).Update("fingerprint", fp).Error; err != nil {
				return 0, err
			}
			event.Fingerprint = fp
		}
		if _, duplicate := existing[fp]; duplicate {
			if err := s.db.Model(&event).Updates(map[string]any{"status": "resolved", "resolved_at": now, "resolve_note": "对象关联标签更新，重复事件合并至同一对象的活动事件"}).Error; err != nil {
				return 0, err
			}
			continue
		}
		existing[fp] = event
		keep[fp] = true
		if rule.MissingDataPolicy == "alert" {
			history[fp] = PromMetricSample{Metric: decodeLabelMap(event.LabelsJSON)}
		}
	}
	missing := map[string]bool{}
	for fp, sample := range history {
		if _, ok := current[fp]; !ok {
			sample.Value = []any{now.Unix(), "0"}
			current[fp] = sample
			missing[fp] = true
		}
	}
	matched := 0
	for fp, sample := range current {
		value, _ := promSampleValue(sample)
		if compareFloat(value, rule.Comparator, rule.Threshold) {
			matched++
			keep[fp] = true
			s.upsertMonitorAlertEvent(rule, sample, fp, value, missing[fp])
		}
	}
	for fp, event := range existing {
		sample, present := current[fp]
		value, _ := promSampleValue(sample)
		if present && compareFloat(value, rule.Comparator, rule.Threshold) {
			continue
		}
		observation := "observed"
		if !present {
			observation = "missing"
		}
		recovered, recoveryValue := monitorRecoveryDecision(rule, present, value, recovery, fp)
		updates := map[string]any{"observation_state": observation, "last_evaluated_at": now}
		if present {
			updates["current_value"] = value
			updates["last_observed_at"] = now
		}
		if !recovered {
			updates["recovery_since"] = nil
			updates["pending_since"] = nil
		} else {
			lastEvaluation := time.Time{}
			if event.LastEvaluatedAt != nil {
				lastEvaluation = *event.LastEvaluatedAt
			}
			since := monitorContinuousSince(event.RecoverySince, lastEvaluation, now, rule.EvalIntervalSeconds)
			updates["recovery_since"] = since
			if now.Sub(*since) >= time.Duration(rule.RecoveryForSeconds)*time.Second {
				delete(keep, fp)
				if rule.RecoveryQuery != "" || present {
					updates["current_value"] = recoveryValue
					updates["observation_state"] = "observed"
					updates["last_observed_at"] = now
				}
			}
		}
		if err := s.db.Model(&event).Updates(updates).Error; err != nil {
			return matched, err
		}
	}
	s.recoverInactiveMonitorEvents(rule, keep)
	return matched, nil
}

func monitorRecoveryDecision(rule model.MonitorAlertRule, present bool, value float64, recovery map[string]PromMetricSample, fp string) (bool, float64) {
	if rule.RecoveryQuery != "" {
		sample, found := recovery[fp]
		recoveredValue, valid := promSampleValue(sample)
		return found && valid && compareFloat(recoveredValue, rule.RecoveryComparator, rule.RecoveryThreshold), recoveredValue
	}
	if present {
		return !compareFloat(value, rule.Comparator, rule.Threshold), value
	}
	return rule.MissingDataPolicy == "recover", 0
}
