package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"

	"github.com/robfig/cron/v3"
)

const (
	k8sScalingHPA       = "hpa"
	k8sScalingScheduled = "scheduled"
)

type K8sScalingPolicyPayload struct {
	ID                uint                     `json:"id"`
	Name              string                   `json:"name"`
	PolicyType        string                   `json:"policyType"`
	ClusterID         uint                     `json:"clusterId"`
	Namespace         string                   `json:"namespace"`
	Targets           []model.K8sScalingTarget `json:"targets"`
	MinReplicas       int                      `json:"minReplicas"`
	MaxReplicas       int                      `json:"maxReplicas"`
	CPUEnabled        bool                     `json:"cpuEnabled"`
	CPUUtilization    int                      `json:"cpuUtilization"`
	MemoryEnabled     bool                     `json:"memoryEnabled"`
	MemoryUtilization int                      `json:"memoryUtilization"`
	CronExpr          string                   `json:"cronExpr"`
	ScheduledReplicas int                      `json:"scheduledReplicas"`
}

type K8sScalingPolicyStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type K8sScalingPolicyBatchPayload struct {
	IDs    []uint `json:"ids"`
	Status int    `json:"status"`
}

type K8sScalingScheduler struct {
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[uint]cron.EntryID
}

func normalizeK8sScalingPolicyType(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), k8sScalingScheduled) {
		return k8sScalingScheduled
	}
	return k8sScalingHPA
}

func normalizeK8sScalingStatus(value int) int {
	if value == 1 {
		return 1
	}
	return 2
}

func decodeK8sScalingTargets(raw string) []model.K8sScalingTarget {
	var targets []model.K8sScalingTarget
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return nil
	}
	return targets
}

func normalizeK8sScalingTargets(targets []model.K8sScalingTarget) []model.K8sScalingTarget {
	result := make([]model.K8sScalingTarget, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		typeName := strings.ToLower(strings.TrimSpace(target.WorkloadType))
		if typeName != "deployment" && typeName != "statefulset" {
			continue
		}
		name := Trimmed(target.WorkloadName)
		if name == "" {
			continue
		}
		key := typeName + "/" + name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model.K8sScalingTarget{WorkloadType: typeName, WorkloadName: name})
	}
	return result
}

func (s *Service) initK8sScalingScheduler() {
	s.k8sScalingOnce.Do(func() {
		s.k8sScalingScheduler = &K8sScalingScheduler{
			cron: cron.New(
				cron.WithSeconds(),
				cron.WithLocation(time.Local),
				cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
			),
			entries: map[uint]cron.EntryID{},
		}
		s.k8sScalingScheduler.cron.Start()
		s.reloadK8sScalingPolicies()
	})
}

func (s *Service) reloadK8sScalingPolicies() {
	var policies []model.K8sScalingPolicy
	if err := s.db.Where("policy_type = ? AND status = ?", k8sScalingScheduled, 1).Find(&policies).Error; err != nil {
		return
	}
	for _, policy := range policies {
		_ = s.registerK8sScalingPolicy(policy)
	}
}

func (s *Service) registerK8sScalingPolicy(policy model.K8sScalingPolicy) error {
	if s.k8sScalingScheduler == nil {
		return nil
	}
	expr := normalizeCronExpr(policy.CronExpr)
	if _, err := parseCronExpr(expr); err != nil {
		return err
	}
	s.removeK8sScalingPolicy(policy.ID)
	entryID, err := s.k8sScalingScheduler.cron.AddFunc(expr, func() {
		s.executeK8sScalingPolicy(policy.ID)
	})
	if err != nil {
		return err
	}
	next := parseCronNext(expr, time.Now())
	s.k8sScalingScheduler.mu.Lock()
	s.k8sScalingScheduler.entries[policy.ID] = entryID
	s.k8sScalingScheduler.mu.Unlock()
	return s.db.Model(&model.K8sScalingPolicy{}).Where("id = ?", policy.ID).Updates(map[string]any{
		"cron_expr":   expr,
		"next_run_at": next,
	}).Error
}

func parseCronNext(expr string, now time.Time) *time.Time {
	schedule, err := parseCronExpr(expr)
	if err != nil {
		return nil
	}
	next := schedule.Next(now)
	return &next
}

func (s *Service) removeK8sScalingPolicy(policyID uint) {
	if s.k8sScalingScheduler == nil {
		return
	}
	s.k8sScalingScheduler.mu.Lock()
	entryID, ok := s.k8sScalingScheduler.entries[policyID]
	if ok {
		delete(s.k8sScalingScheduler.entries, policyID)
	}
	s.k8sScalingScheduler.mu.Unlock()
	if ok {
		s.k8sScalingScheduler.cron.Remove(entryID)
	}
	_ = s.db.Model(&model.K8sScalingPolicy{}).Where("id = ?", policyID).Update("next_run_at", nil).Error
}

func (s *Service) executeK8sScalingPolicy(policyID uint) {
	var policy model.K8sScalingPolicy
	if err := s.db.First(&policy, policyID).Error; err != nil || policy.Status != 1 || policy.PolicyType != k8sScalingScheduled {
		return
	}
	targets := decodeK8sScalingTargets(policy.TargetsJSON)
	if len(targets) == 0 {
		return
	}
	failed := make([]string, 0)
	for _, target := range targets {
		_, err := s.ScaleK8sWorkload(model.K8sWorkloadActionPayload{
			ClusterID: policy.ClusterID, Namespace: policy.Namespace,
			WorkloadType: target.WorkloadType, WorkloadName: target.WorkloadName,
			Replicas: policy.ScheduledReplicas,
		})
		if err != nil {
			failed = append(failed, target.WorkloadName+": "+err.Error())
		}
	}
	now := time.Now()
	status, summary := "success", fmt.Sprintf("已将 %d 个工作负载调整为 %d 个副本", len(targets), policy.ScheduledReplicas)
	if len(failed) > 0 {
		status = "failed"
		summary = "部分工作负载执行失败：" + strings.Join(failed, "；")
	}
	updates := map[string]any{"last_status": status, "last_summary": summary, "last_run_at": &now}
	if next := parseCronNext(policy.CronExpr, now); next != nil {
		updates["next_run_at"] = next
	}
	_ = s.db.Model(&model.K8sScalingPolicy{}).Where("id = ?", policyID).Updates(updates).Error
}

func (s *Service) ListK8sScalingPolicies(policyType string) ([]map[string]any, error) {
	query := s.db.Model(&model.K8sScalingPolicy{}).Order("id desc")
	if value := normalizeK8sScalingPolicyType(policyType); strings.TrimSpace(policyType) != "" {
		query = query.Where("policy_type = ?", value)
	}
	var policies []model.K8sScalingPolicy
	if err := query.Find(&policies).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		result = append(result, mapK8sScalingPolicy(policy))
	}
	return result, nil
}

func mapK8sScalingPolicy(policy model.K8sScalingPolicy) map[string]any {
	targets := decodeK8sScalingTargets(policy.TargetsJSON)
	return map[string]any{
		"id": policy.ID, "name": policy.Name, "policyType": policy.PolicyType,
		"clusterId": policy.ClusterID, "namespace": policy.Namespace, "targets": targets,
		"status": policy.Status, "minReplicas": policy.MinReplicas, "maxReplicas": policy.MaxReplicas,
		"cpuEnabled": policy.CPUEnabled, "cpuUtilization": policy.CPUUtilization,
		"memoryEnabled": policy.MemoryEnabled, "memoryUtilization": policy.MemoryUtilization,
		"cronExpr": policy.CronExpr, "scheduledReplicas": policy.ScheduledReplicas,
		"lastStatus": policy.LastStatus, "lastSummary": policy.LastSummary,
		"lastRunAt": policy.LastRunAt, "nextRunAt": policy.NextRunAt,
		"createTime": policy.CreatedAt, "updateTime": policy.UpdatedAt,
	}
}

func (s *Service) CreateK8sScalingPolicy(payload K8sScalingPolicyPayload) (map[string]any, error) {
	policy, err := s.prepareK8sScalingPolicy(payload, 0)
	if err != nil {
		return nil, err
	}
	policy.Status = 2
	if err := s.db.Create(&policy).Error; err != nil {
		return nil, err
	}
	return mapK8sScalingPolicy(policy), nil
}

func (s *Service) prepareK8sScalingPolicy(payload K8sScalingPolicyPayload, excludingID uint) (model.K8sScalingPolicy, error) {
	policyType := normalizeK8sScalingPolicyType(payload.PolicyType)
	name := Trimmed(payload.Name)
	namespace := Trimmed(payload.Namespace)
	targets := normalizeK8sScalingTargets(payload.Targets)
	if name == "" || payload.ClusterID == 0 || namespace == "" || len(targets) == 0 {
		return model.K8sScalingPolicy{}, errors.New("名称、集群、命名空间和工作负载不能为空")
	}
	if len([]rune(name)) > 128 {
		return model.K8sScalingPolicy{}, errors.New("策略名称不能超过 128 个字符")
	}
	if policyType == k8sScalingHPA {
		if payload.MinReplicas < 1 || payload.MaxReplicas < payload.MinReplicas || payload.MaxReplicas == 0 {
			return model.K8sScalingPolicy{}, errors.New("HPA 副本范围无效")
		}
		if !payload.CPUEnabled && !payload.MemoryEnabled {
			return model.K8sScalingPolicy{}, errors.New("至少启用 CPU 或内存一个伸缩指标")
		}
		if payload.CPUEnabled && (payload.CPUUtilization < 1 || payload.CPUUtilization > 100) {
			return model.K8sScalingPolicy{}, errors.New("CPU 目标使用率必须为 1-100")
		}
		if payload.MemoryEnabled && (payload.MemoryUtilization < 1 || payload.MemoryUtilization > 100) {
			return model.K8sScalingPolicy{}, errors.New("内存目标使用率必须为 1-100")
		}
	} else {
		if payload.ScheduledReplicas < 0 {
			return model.K8sScalingPolicy{}, errors.New("定时副本数不能小于 0")
		}
		if _, err := parseCronExpr(payload.CronExpr); err != nil {
			return model.K8sScalingPolicy{}, err
		}
	}
	if err := s.ensureK8sScalingTargetAvailability(payload.ClusterID, namespace, targets, excludingID); err != nil {
		return model.K8sScalingPolicy{}, err
	}
	targetJSON, _ := json.Marshal(targets)
	return model.K8sScalingPolicy{
		Name: name, PolicyType: policyType, ClusterID: payload.ClusterID, Namespace: namespace,
		TargetsJSON: string(targetJSON), MinReplicas: payload.MinReplicas, MaxReplicas: payload.MaxReplicas,
		CPUEnabled: payload.CPUEnabled, CPUUtilization: payload.CPUUtilization,
		MemoryEnabled: payload.MemoryEnabled, MemoryUtilization: payload.MemoryUtilization,
		CronExpr: normalizeCronExpr(payload.CronExpr), ScheduledReplicas: payload.ScheduledReplicas,
	}, nil
}

func (s *Service) UpdateK8sScalingPolicy(payload K8sScalingPolicyPayload) (map[string]any, error) {
	if payload.ID == 0 {
		return nil, errors.New("伸缩策略不存在")
	}
	var previous model.K8sScalingPolicy
	if err := s.db.First(&previous, payload.ID).Error; err != nil {
		return nil, errors.New("伸缩策略不存在")
	}
	next, err := s.prepareK8sScalingPolicy(payload, previous.ID)
	if err != nil {
		return nil, err
	}
	next.ID = previous.ID
	next.Status = previous.Status
	next.CreatedAt = previous.CreatedAt
	next.LastStatus = previous.LastStatus
	next.LastSummary = previous.LastSummary
	next.LastRunAt = previous.LastRunAt
	next.NextRunAt = previous.NextRunAt

	if previous.Status == 1 {
		if previous.PolicyType == k8sScalingHPA {
			if err := s.disableK8sHPA(previous); err != nil {
				return nil, err
			}
		} else {
			s.removeK8sScalingPolicy(previous.ID)
		}
	}
	if err := s.db.Model(&model.K8sScalingPolicy{}).Where("id = ?", previous.ID).Updates(map[string]any{
		"name": next.Name, "policy_type": next.PolicyType, "cluster_id": next.ClusterID, "namespace": next.Namespace,
		"targets_json": next.TargetsJSON, "min_replicas": next.MinReplicas, "max_replicas": next.MaxReplicas,
		"cpu_enabled": next.CPUEnabled, "cpu_utilization": next.CPUUtilization,
		"memory_enabled": next.MemoryEnabled, "memory_utilization": next.MemoryUtilization,
		"cron_expr": next.CronExpr, "scheduled_replicas": next.ScheduledReplicas,
	}).Error; err != nil {
		return nil, err
	}
	if previous.Status == 1 {
		if next.PolicyType == k8sScalingHPA {
			err = s.enableK8sHPA(next)
		} else {
			err = s.registerK8sScalingPolicy(next)
		}
		if err != nil {
			return nil, fmt.Errorf("策略已保存，但重新启用失败：%w", err)
		}
	}
	var updated model.K8sScalingPolicy
	if err := s.db.First(&updated, previous.ID).Error; err != nil {
		return nil, err
	}
	return mapK8sScalingPolicy(updated), nil
}

func (s *Service) ensureK8sScalingTargetAvailability(clusterID uint, namespace string, targets []model.K8sScalingTarget, excludingID uint) error {
	var policies []model.K8sScalingPolicy
	query := s.db.Where("cluster_id = ? AND namespace = ? AND status = ?", clusterID, namespace, 1)
	if excludingID > 0 {
		query = query.Where("id <> ?", excludingID)
	}
	if err := query.Find(&policies).Error; err != nil {
		return err
	}
	requested := map[string]struct{}{}
	for _, target := range targets {
		requested[strings.ToLower(target.WorkloadType)+"/"+target.WorkloadName] = struct{}{}
	}
	for _, policy := range policies {
		for _, target := range decodeK8sScalingTargets(policy.TargetsJSON) {
			if _, ok := requested[strings.ToLower(target.WorkloadType)+"/"+target.WorkloadName]; ok {
				return fmt.Errorf("工作负载 %s 已被启用的伸缩策略 %q 管理", target.WorkloadName, policy.Name)
			}
		}
	}
	return nil
}

func (s *Service) UpdateK8sScalingPolicyStatus(payload K8sScalingPolicyStatusPayload) error {
	var policy model.K8sScalingPolicy
	if err := s.db.First(&policy, payload.ID).Error; err != nil {
		return errors.New("伸缩策略不存在")
	}
	status := normalizeK8sScalingStatus(payload.Status)
	if status == 1 {
		if err := s.ensureK8sScalingTargetAvailability(policy.ClusterID, policy.Namespace, decodeK8sScalingTargets(policy.TargetsJSON), policy.ID); err != nil {
			return err
		}
		if policy.PolicyType == k8sScalingHPA {
			if err := s.enableK8sHPA(policy); err != nil {
				return err
			}
		} else if err := s.registerK8sScalingPolicy(policy); err != nil {
			return err
		}
	} else {
		if policy.PolicyType == k8sScalingHPA {
			if err := s.disableK8sHPA(policy); err != nil {
				return err
			}
		} else {
			s.removeK8sScalingPolicy(policy.ID)
		}
	}
	return s.db.Model(&policy).Updates(map[string]any{"status": status}).Error
}

func normalizeK8sScalingPolicyIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := map[uint]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *Service) BatchUpdateK8sScalingPolicyStatus(payload K8sScalingPolicyBatchPayload) (int, error) {
	ids := normalizeK8sScalingPolicyIDs(payload.IDs)
	if len(ids) == 0 {
		return 0, errors.New("请选择至少一条伸缩策略")
	}
	updated := 0
	for _, id := range ids {
		if err := s.UpdateK8sScalingPolicyStatus(K8sScalingPolicyStatusPayload{ID: id, Status: payload.Status}); err != nil {
			return updated, fmt.Errorf("已处理 %d 条策略，策略 #%d 操作失败：%w", updated, id, err)
		}
		updated++
	}
	return updated, nil
}

func (s *Service) DeleteK8sScalingPolicy(id uint) error {
	var policy model.K8sScalingPolicy
	if err := s.db.First(&policy, id).Error; err != nil {
		return errors.New("伸缩策略不存在")
	}
	if policy.Status == 1 {
		if err := s.UpdateK8sScalingPolicyStatus(K8sScalingPolicyStatusPayload{ID: id, Status: 2}); err != nil {
			return err
		}
	}
	s.removeK8sScalingPolicy(id)
	return s.db.Delete(&policy).Error
}

func (s *Service) BatchDeleteK8sScalingPolicies(ids []uint) (int, error) {
	policyIDs := normalizeK8sScalingPolicyIDs(ids)
	if len(policyIDs) == 0 {
		return 0, errors.New("请选择至少一条伸缩策略")
	}
	deleted := 0
	for _, id := range policyIDs {
		if err := s.DeleteK8sScalingPolicy(id); err != nil {
			return deleted, fmt.Errorf("已删除 %d 条策略，策略 #%d 删除失败：%w", deleted, id, err)
		}
		deleted++
	}
	return deleted, nil
}

func (s *Service) enableK8sHPA(policy model.K8sScalingPolicy) error {
	_, runtime, client, err := s.k8sClientForCluster(policy.ClusterID)
	if err != nil {
		return err
	}
	created := make([]string, 0)
	for _, target := range decodeK8sScalingTargets(policy.TargetsJSON) {
		name := k8sHPAName(policy.ID, target.WorkloadName)
		collectionPath := fmt.Sprintf("/apis/autoscaling/v2/namespaces/%s/horizontalpodautoscalers", policy.Namespace)
		resourcePath := collectionPath + "/" + name
		body := buildK8sHPABody(policy, target, name)
		if err := k8sDoJSON(client, runtime, http.MethodPost, collectionPath, nil, mustJSON(body), "application/json", nil); err != nil {
			// Re-enabling a policy should update the HPA it owns instead of
			// failing with AlreadyExists. Merge patch avoids requiring the
			// resourceVersion that a full PUT would need.
			if isK8sConflictError(err) {
				if patchErr := k8sPatchJSON(client, runtime, resourcePath, body, "application/merge-patch+json", nil); patchErr == nil {
					continue
				} else {
					err = patchErr
				}
			}
			for _, createdName := range created {
				_ = k8sDoJSON(client, runtime, http.MethodDelete, fmt.Sprintf("/apis/autoscaling/v2/namespaces/%s/horizontalpodautoscalers/%s", policy.Namespace, createdName), nil, nil, "application/json", nil)
			}
			return fmt.Errorf("创建 HPA %s 失败：%w", name, err)
		}
		created = append(created, name)
	}
	return nil
}

func isK8sConflictError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "unexpected status: 409") || strings.Contains(message, "AlreadyExists")
}

func (s *Service) disableK8sHPA(policy model.K8sScalingPolicy) error {
	_, runtime, client, err := s.k8sClientForCluster(policy.ClusterID)
	if err != nil {
		return err
	}
	for _, target := range decodeK8sScalingTargets(policy.TargetsJSON) {
		path := fmt.Sprintf("/apis/autoscaling/v2/namespaces/%s/horizontalpodautoscalers/%s", policy.Namespace, k8sHPAName(policy.ID, target.WorkloadName))
		if err := k8sDoJSON(client, runtime, http.MethodDelete, path, nil, nil, "application/json", nil); err != nil && !isK8sNotFoundError(err) {
			return fmt.Errorf("删除 HPA %s 失败：%w", target.WorkloadName, err)
		}
	}
	return nil
}

var k8sHPASlugPattern = regexp.MustCompile(`[^a-z0-9-]+`)

func k8sHPAName(policyID uint, workloadName string) string {
	slug := strings.ToLower(strings.TrimSpace(workloadName))
	slug = k8sHPASlugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	name := "ops-hpa-" + strconv.FormatUint(uint64(policyID), 10) + "-" + slug
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

func buildK8sHPABody(policy model.K8sScalingPolicy, target model.K8sScalingTarget, name string) map[string]any {
	metrics := make([]map[string]any, 0, 2)
	if policy.CPUEnabled {
		metrics = append(metrics, map[string]any{"type": "Resource", "resource": map[string]any{"name": "cpu", "target": map[string]any{"type": "Utilization", "averageUtilization": policy.CPUUtilization}}})
	}
	if policy.MemoryEnabled {
		metrics = append(metrics, map[string]any{"type": "Resource", "resource": map[string]any{"name": "memory", "target": map[string]any{"type": "Utilization", "averageUtilization": policy.MemoryUtilization}}})
	}
	workloadKind := "Deployment"
	if strings.EqualFold(target.WorkloadType, "statefulset") {
		workloadKind = "StatefulSet"
	}
	return map[string]any{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]any{"name": name, "namespace": policy.Namespace, "labels": map[string]string{"ops-admin-scaling-policy": strconv.FormatUint(uint64(policy.ID), 10)}},
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": workloadKind, "name": target.WorkloadName},
			"minReplicas":    policy.MinReplicas, "maxReplicas": policy.MaxReplicas, "metrics": metrics,
		},
	}
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}
