package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"ops-admin/backend/model"
)

type OpsJobPayload struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Status         int    `json:"status"`
	TemplateID     uint   `json:"templateId"`
	NotifyEnabled  bool   `json:"notifyEnabled"`
	NotifyRuleID   uint   `json:"notifyRuleId"`
	GraphJSON      string `json:"graphJson"`
	DefinitionJSON string `json:"definitionJson"`
}

type OpsJobTemplatePayload struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Status         int    `json:"status"`
	GraphJSON      string `json:"graphJson"`
	DefinitionJSON string `json:"definitionJson"`
}

type OpsJobStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type OpsJobHistoryApprovalPayload struct {
	HistoryID uint   `json:"historyId"`
	StepID    string `json:"stepId"`
	Note      string `json:"note"`
}

type OpsJobDefinition struct {
	Nodes []OpsJobNode `json:"nodes"`
	Edges []OpsJobEdge `json:"edges"`
}

type OpsJobNode struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Label  string         `json:"label"`
	Config map[string]any `json:"config"`
	Meta   map[string]any `json:"meta"`
}

type OpsJobEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func normalizeJobStatus(value int) int {
	if value == 2 {
		return 2
	}
	return 1
}

func normalizeJobNodeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "file":
		return "file"
	case "approval":
		return "approval"
	case "notify":
		return "notify"
	default:
		return "script"
	}
}

func parseOpsJobDefinition(raw string) (*OpsJobDefinition, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("作业编排定义不能为空")
	}
	var definition OpsJobDefinition
	if err := json.Unmarshal([]byte(value), &definition); err != nil {
		return nil, errors.New("作业编排定义格式不正确")
	}
	if len(definition.Nodes) == 0 {
		return nil, errors.New("请至少添加一个作业步骤")
	}
	for index := range definition.Nodes {
		definition.Nodes[index].Type = normalizeJobNodeType(definition.Nodes[index].Type)
		if strings.TrimSpace(definition.Nodes[index].ID) == "" {
			return nil, errors.New("存在缺少 ID 的作业步骤")
		}
		if strings.TrimSpace(definition.Nodes[index].Label) == "" {
			definition.Nodes[index].Label = fmt.Sprintf("步骤 %d", index+1)
		}
		if definition.Nodes[index].Config == nil {
			definition.Nodes[index].Config = map[string]any{}
		}
	}
	return &definition, nil
}

func stringConfigMap(value any) map[string]string {
	result := map[string]string{}
	values, ok := value.(map[string]any)
	if !ok {
		return result
	}
	for key, raw := range values {
		if text, ok := raw.(string); ok {
			result[key] = text
		}
	}
	return result
}

func (s *Service) normalizeOpsJobDefinitionVariables(raw, existingRaw string) (string, error) {
	definition, err := parseOpsJobDefinition(raw)
	if err != nil {
		return "", err
	}
	existingNodes := map[string]OpsJobNode{}
	if strings.TrimSpace(existingRaw) != "" {
		if existing, parseErr := parseOpsJobDefinition(existingRaw); parseErr == nil {
			for _, node := range existing.Nodes {
				existingNodes[node.ID] = node
			}
		}
	}
	for index := range definition.Nodes {
		node := &definition.Nodes[index]
		if node.Type != "script" {
			continue
		}
		scriptID := uint(numberConfig(node.Config, "scriptId"))
		if scriptID == 0 {
			continue
		}
		script, err := s.GetOpsScript(scriptID)
		if err != nil {
			return "", err
		}
		var existingValues model.OpsScriptVariableValues
		if existing, ok := existingNodes[node.ID]; ok {
			existingValues = model.OpsScriptVariableValues(stringConfigMap(existing.Config["variables"]))
		}
		stored, _, err := resolveScheduleScriptVariables(script, stringConfigMap(node.Config["variables"]), existingValues)
		if err != nil {
			return "", fmt.Errorf("步骤“%s”：%w", node.Label, err)
		}
		delete(node.Config, "parameters")
		node.Config["variables"] = map[string]string(stored)
	}
	data, err := json.Marshal(definition)
	return string(data), err
}

func (s *Service) jobDefinitionForView(raw string) string {
	definition, err := parseOpsJobDefinition(raw)
	if err != nil {
		return raw
	}
	for index := range definition.Nodes {
		node := &definition.Nodes[index]
		if node.Type != "script" || uint(numberConfig(node.Config, "scriptId")) == 0 {
			continue
		}
		if script, err := s.GetOpsScript(uint(numberConfig(node.Config, "scriptId"))); err == nil {
			node.Config["variables"] = scheduleVariableResponse(script, model.OpsScriptVariableValues(stringConfigMap(node.Config["variables"])))
		}
		delete(node.Config, "parameters")
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return raw
	}
	return string(data)
}

func (s *Service) ListOpsJobs(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsJob{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsJob
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetOpsJob(id uint) (*model.OpsJob, error) {
	var item model.OpsJob
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) GetOpsJobForView(id uint) (*model.OpsJob, error) {
	item, err := s.GetOpsJob(id)
	if err != nil {
		return nil, err
	}
	item.DefinitionJSON = s.jobDefinitionForView(item.DefinitionJSON)
	return item, nil
}

func (s *Service) CreateOpsJob(payload OpsJobPayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("作业名称不能为空")
	}
	definitionJSON, err := s.normalizeOpsJobDefinitionVariables(payload.DefinitionJSON, "")
	if err != nil {
		return err
	}
	item := model.OpsJob{
		Name:           Trimmed(payload.Name),
		Description:    Trimmed(payload.Description),
		Status:         normalizeJobStatus(payload.Status),
		TemplateID:     payload.TemplateID,
		NotifyEnabled:  payload.NotifyEnabled,
		NotifyRuleID:   payload.NotifyRuleID,
		GraphJSON:      strings.TrimSpace(payload.GraphJSON),
		DefinitionJSON: definitionJSON,
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateOpsJob(payload OpsJobPayload) error {
	if payload.ID == 0 {
		return errors.New("作业 ID 不能为空")
	}
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("作业名称不能为空")
	}
	var existing model.OpsJob
	if err := s.db.First(&existing, payload.ID).Error; err != nil {
		return err
	}
	definitionJSON, err := s.normalizeOpsJobDefinitionVariables(payload.DefinitionJSON, existing.DefinitionJSON)
	if err != nil {
		return err
	}
	return s.db.Model(&model.OpsJob{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"name":            Trimmed(payload.Name),
		"description":     Trimmed(payload.Description),
		"status":          normalizeJobStatus(payload.Status),
		"template_id":     payload.TemplateID,
		"notify_enabled":  payload.NotifyEnabled,
		"notify_rule_id":  payload.NotifyRuleID,
		"graph_json":      strings.TrimSpace(payload.GraphJSON),
		"definition_json": definitionJSON,
	}).Error
}

func (s *Service) DeleteOpsJob(id uint) error {
	return s.db.Delete(&model.OpsJob{}, id).Error
}

func (s *Service) UpdateOpsJobStatus(payload OpsJobStatusPayload) error {
	if payload.ID == 0 {
		return errors.New("作业 ID 不能为空")
	}
	return s.db.Model(&model.OpsJob{}).Where("id = ?", payload.ID).Update("status", normalizeJobStatus(payload.Status)).Error
}

func (s *Service) ListOpsJobTemplates(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsJobTemplate{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsJobTemplate
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) ListOpsJobTemplateOptions() ([]model.OpsJobTemplate, error) {
	var list []model.OpsJobTemplate
	if err := s.db.Where("status = ?", 1).Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetOpsJobTemplate(id uint) (*model.OpsJobTemplate, error) {
	var item model.OpsJobTemplate
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) GetOpsJobTemplateForView(id uint) (*model.OpsJobTemplate, error) {
	item, err := s.GetOpsJobTemplate(id)
	if err != nil {
		return nil, err
	}
	item.DefinitionJSON = s.jobDefinitionForView(item.DefinitionJSON)
	return item, nil
}

func (s *Service) CreateOpsJobTemplate(payload OpsJobTemplatePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("作业模板名称不能为空")
	}
	definitionJSON, err := s.normalizeOpsJobDefinitionVariables(payload.DefinitionJSON, "")
	if err != nil {
		return err
	}
	item := model.OpsJobTemplate{
		Name:           Trimmed(payload.Name),
		Description:    Trimmed(payload.Description),
		Status:         normalizeJobStatus(payload.Status),
		GraphJSON:      strings.TrimSpace(payload.GraphJSON),
		DefinitionJSON: definitionJSON,
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateOpsJobTemplate(payload OpsJobTemplatePayload) error {
	if payload.ID == 0 {
		return errors.New("作业模板 ID 不能为空")
	}
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("作业模板名称不能为空")
	}
	var existing model.OpsJobTemplate
	if err := s.db.First(&existing, payload.ID).Error; err != nil {
		return err
	}
	definitionJSON, err := s.normalizeOpsJobDefinitionVariables(payload.DefinitionJSON, existing.DefinitionJSON)
	if err != nil {
		return err
	}
	return s.db.Model(&model.OpsJobTemplate{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"name":            Trimmed(payload.Name),
		"description":     Trimmed(payload.Description),
		"status":          normalizeJobStatus(payload.Status),
		"graph_json":      strings.TrimSpace(payload.GraphJSON),
		"definition_json": definitionJSON,
	}).Error
}

func (s *Service) DeleteOpsJobTemplate(id uint) error {
	return s.db.Delete(&model.OpsJobTemplate{}, id).Error
}

func (s *Service) UpdateOpsJobTemplateStatus(payload OpsJobStatusPayload) error {
	if payload.ID == 0 {
		return errors.New("作业模板 ID 不能为空")
	}
	return s.db.Model(&model.OpsJobTemplate{}).Where("id = ?", payload.ID).Update("status", normalizeJobStatus(payload.Status)).Error
}

func (s *Service) ListOpsJobHistories(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsJobHistory{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("job_name LIKE ? OR summary LIKE ? OR current_step_name LIKE ?", like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsJobHistory
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetOpsJobHistoryDetail(id uint) (map[string]any, error) {
	var history model.OpsJobHistory
	if err := s.db.First(&history, id).Error; err != nil {
		return nil, err
	}
	var steps []model.OpsJobHistoryStep
	if err := s.db.Where("history_id = ?", id).Order("id ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"history": history,
		"steps":   steps,
	}, nil
}

func (s *Service) RunOpsJob(id uint) error {
	job, err := s.GetOpsJob(id)
	if err != nil {
		return err
	}
	if job.Status != 1 {
		return errors.New("当前作业已禁用，无法执行")
	}
	definition, err := parseOpsJobDefinition(job.DefinitionJSON)
	if err != nil {
		return err
	}
	startedAt := time.Now()
	history := model.OpsJobHistory{
		JobID:          job.ID,
		JobName:        job.Name,
		TriggerType:    "manual",
		Status:         "running",
		Summary:        "作业已触发，正在执行",
		DefinitionJSON: job.DefinitionJSON,
		StartedAt:      &startedAt,
	}
	if err := s.db.Create(&history).Error; err != nil {
		return err
	}
	go s.runOpsJobDefinition(history.ID, *definition)
	return nil
}

func (s *Service) ApproveOpsJobHistoryStep(payload OpsJobHistoryApprovalPayload) error {
	var history model.OpsJobHistory
	if err := s.db.First(&history, payload.HistoryID).Error; err != nil {
		return err
	}
	if history.Status != "waiting_approval" {
		return errors.New("当前作业不处于待确认状态")
	}
	var step model.OpsJobHistoryStep
	if err := s.db.Where("history_id = ? AND step_id = ?", payload.HistoryID, payload.StepID).First(&step).Error; err != nil {
		return err
	}
	if step.Status != "waiting_approval" {
		return errors.New("当前步骤不处于待确认状态")
	}
	finishedAt := time.Now()
	duration := int64(0)
	if step.StartedAt != nil {
		duration = finishedAt.Sub(*step.StartedAt).Milliseconds()
	}
	if err := s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(map[string]any{
		"status":            "success",
		"summary":           firstNonEmpty(step.Summary, "人工确认已通过"),
		"approval_decision": "approved",
		"approval_note":     strings.TrimSpace(payload.Note),
		"finished_at":       &finishedAt,
		"duration_ms":       duration,
	}).Error; err != nil {
		return err
	}
	var definition OpsJobDefinition
	if err := json.Unmarshal([]byte(history.DefinitionJSON), &definition); err != nil {
		return errors.New("作业定义已损坏，无法继续执行")
	}
	go s.resumeOpsJobDefinition(payload.HistoryID, definition, payload.StepID)
	return nil
}

func (s *Service) RejectOpsJobHistoryStep(payload OpsJobHistoryApprovalPayload) error {
	var history model.OpsJobHistory
	if err := s.db.First(&history, payload.HistoryID).Error; err != nil {
		return err
	}
	var step model.OpsJobHistoryStep
	if err := s.db.Where("history_id = ? AND step_id = ?", payload.HistoryID, payload.StepID).First(&step).Error; err != nil {
		return err
	}
	finishedAt := time.Now()
	duration := int64(0)
	if step.StartedAt != nil {
		duration = finishedAt.Sub(*step.StartedAt).Milliseconds()
	}
	if err := s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(map[string]any{
		"status":            "rejected",
		"summary":           "人工确认已拒绝",
		"approval_decision": "rejected",
		"approval_note":     strings.TrimSpace(payload.Note),
		"finished_at":       &finishedAt,
		"duration_ms":       duration,
	}).Error; err != nil {
		return err
	}
	return s.db.Model(&model.OpsJobHistory{}).Where("id = ?", payload.HistoryID).Updates(map[string]any{
		"status":            "rejected",
		"summary":           "作业被人工确认步骤拒绝",
		"current_step_id":   step.StepID,
		"current_step_name": step.StepName,
		"finished_at":       &finishedAt,
	}).Error
}

func buildOpsJobNodeOrder(definition OpsJobDefinition) ([]OpsJobNode, error) {
	nodeMap := make(map[string]OpsJobNode, len(definition.Nodes))
	inDegree := make(map[string]int, len(definition.Nodes))
	nextMap := make(map[string][]string)
	for _, node := range definition.Nodes {
		nodeMap[node.ID] = node
		inDegree[node.ID] = 0
	}
	for _, edge := range definition.Edges {
		if edge.Source == "" || edge.Target == "" {
			continue
		}
		if _, ok := nodeMap[edge.Source]; !ok {
			continue
		}
		if _, ok := nodeMap[edge.Target]; !ok {
			continue
		}
		inDegree[edge.Target]++
		nextMap[edge.Source] = append(nextMap[edge.Source], edge.Target)
	}
	queue := make([]string, 0)
	for _, node := range definition.Nodes {
		if inDegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	order := make([]OpsJobNode, 0, len(definition.Nodes))
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		order = append(order, nodeMap[currentID])
		for _, nextID := range nextMap[currentID] {
			inDegree[nextID]--
			if inDegree[nextID] == 0 {
				queue = append(queue, nextID)
			}
		}
	}
	if len(order) != len(definition.Nodes) {
		return nil, errors.New("作业编排中存在循环依赖，请检查连线关系")
	}
	return order, nil
}

func (s *Service) runOpsJobDefinition(historyID uint, definition OpsJobDefinition) {
	order, err := buildOpsJobNodeOrder(definition)
	if err != nil {
		s.finishOpsJobHistory(historyID, "failed", err.Error(), "", "")
		return
	}
	for _, node := range order {
		paused, failed := s.runSingleOpsJobNode(historyID, node)
		if paused || failed {
			return
		}
	}
	s.finishOpsJobHistory(historyID, "success", "作业执行完成", "", "")
}

func (s *Service) resumeOpsJobDefinition(historyID uint, definition OpsJobDefinition, approvedStepID string) {
	order, err := buildOpsJobNodeOrder(definition)
	if err != nil {
		s.finishOpsJobHistory(historyID, "failed", err.Error(), "", "")
		return
	}
	afterApproved := false
	for _, node := range order {
		if !afterApproved {
			if node.ID == approvedStepID {
				afterApproved = true
			}
			continue
		}
		paused, failed := s.runSingleOpsJobNode(historyID, node)
		if paused || failed {
			return
		}
	}
	s.finishOpsJobHistory(historyID, "success", "作业执行完成", "", "")
}

func (s *Service) runSingleOpsJobNode(historyID uint, node OpsJobNode) (paused bool, failed bool) {
	stepName := firstNonEmpty(strings.TrimSpace(node.Label), node.Type)
	now := time.Now()
	step := model.OpsJobHistoryStep{
		HistoryID: historyID,
		StepID:    node.ID,
		StepName:  stepName,
		StepType:  node.Type,
		Status:    "running",
		Summary:   "步骤开始执行",
		StartedAt: &now,
	}
	if err := s.db.Create(&step).Error; err != nil {
		s.finishOpsJobHistory(historyID, "failed", err.Error(), node.ID, stepName)
		return false, true
	}
	_ = s.db.Model(&model.OpsJobHistory{}).Where("id = ?", historyID).Updates(map[string]any{
		"status":            "running",
		"summary":           fmt.Sprintf("正在执行步骤：%s", stepName),
		"current_step_id":   node.ID,
		"current_step_name": stepName,
	}).Error

	switch node.Type {
	case "notify":
		status, summary, output, err := s.executeOpsJobNotifyNode(historyID, stepName, node)
		finishedAt := time.Now()
		duration := finishedAt.Sub(now).Milliseconds()
		_ = s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(map[string]any{
			"status":      status,
			"summary":     summary,
			"output":      output,
			"finished_at": &finishedAt,
			"duration_ms": duration,
		}).Error
		if err != nil {
			s.finishOpsJobHistory(historyID, "failed", firstNonEmpty(summary, err.Error()), node.ID, stepName)
			return false, true
		}
	case "approval":
		_ = s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(map[string]any{
			"status":  "waiting_approval",
			"summary": firstNonEmpty(stringConfig(node.Config, "message"), "等待人工确认"),
			"output":  firstNonEmpty(stringConfig(node.Config, "content"), "请确认后继续执行该作业"),
		}).Error
		_ = s.db.Model(&model.OpsJobHistory{}).Where("id = ?", historyID).Updates(map[string]any{
			"status":            "waiting_approval",
			"summary":           fmt.Sprintf("等待人工确认：%s", stepName),
			"current_step_id":   node.ID,
			"current_step_name": stepName,
		}).Error
		return true, false
	case "file":
		status, summary, output, execTaskID, err := s.executeOpsJobFileNode(stepName, node.Config)
		finishedAt := time.Now()
		duration := finishedAt.Sub(now).Milliseconds()
		updates := map[string]any{
			"status":       status,
			"summary":      summary,
			"output":       output,
			"exec_task_id": execTaskID,
			"finished_at":  &finishedAt,
			"duration_ms":  duration,
		}
		_ = s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(updates).Error
		if err != nil || status == "failed" {
			s.finishOpsJobHistory(historyID, "failed", firstNonEmpty(summary, errString(err)), node.ID, stepName)
			return false, true
		}
	default:
		status, summary, output, execTaskID, err := s.executeOpsJobScriptNode(stepName, node.Config)
		finishedAt := time.Now()
		duration := finishedAt.Sub(now).Milliseconds()
		updates := map[string]any{
			"status":       status,
			"summary":      summary,
			"output":       output,
			"exec_task_id": execTaskID,
			"finished_at":  &finishedAt,
			"duration_ms":  duration,
		}
		_ = s.db.Model(&model.OpsJobHistoryStep{}).Where("id = ?", step.ID).Updates(updates).Error
		if err != nil || status == "failed" {
			s.finishOpsJobHistory(historyID, "failed", firstNonEmpty(summary, errString(err)), node.ID, stepName)
			return false, true
		}
	}
	return false, false
}

func (s *Service) executeOpsJobScriptNode(stepName string, config map[string]any) (string, string, string, uint, error) {
	scriptID := uint(numberConfig(config, "scriptId"))
	if scriptID == 0 {
		return "failed", "缺少脚本配置", "", 0, errors.New("缺少脚本配置")
	}
	script, err := s.GetOpsScript(scriptID)
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	hostIDs, groupIDs := opsJobTargetIDs(config)
	hosts, err := s.resolveOpsTargetHosts(hostIDs, groupIDs)
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	params := strings.TrimSpace(stringConfig(config, "parameters"))
	_, variables, err := resolveScheduleScriptVariables(script, nil, model.OpsScriptVariableValues(stringConfigMap(config["variables"])))
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	task := model.OpsExecTask{
		TaskType:       "script",
		Title:          stepName,
		ScriptID:       script.ID,
		ScriptName:     script.Name,
		Parameters:     params,
		Concurrency:    normalizeOpsConcurrency(numberConfig(config, "concurrency")),
		TimeoutSeconds: normalizeOpsTimeout(script.TimeoutSeconds),
		Status:         "running",
		Summary:        "作业步骤执行中",
		HostCount:      len(hosts),
		Operator:       "job-engine",
		Source:         "job",
		RiskLevel:      opsRiskLevel(script.Content),
		ScriptVersion:  script.CurrentVersion,
		TargetSnapshot: opsTargetSnapshot(hosts),
	}
	result, err := s.runOpsTaskLegacy(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		return s.execScriptOnHost(host, *script, params, variables, task.TimeoutSeconds)
	})
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	taskInfo, _ := result["task"].(model.OpsExecTask)
	rows, _ := result["results"].([]model.OpsExecTargetResult)
	outputs := make([]string, 0, len(rows))
	for _, row := range rows {
		line := fmt.Sprintf("%s [%s] exit=%d", row.HostName, row.Status, row.ExitCode)
		if strings.TrimSpace(row.ErrorText) != "" {
			line += " " + strings.TrimSpace(row.ErrorText)
		}
		outputs = append(outputs, line)
	}
	return taskInfo.Status, taskInfo.Summary, strings.Join(outputs, "\n"), taskInfo.ID, nil
}

func (s *Service) executeOpsJobFileNode(stepName string, config map[string]any) (string, string, string, uint, error) {
	sourceHostID := uint(numberConfig(config, "sourceHostId"))
	sourcePath := strings.TrimSpace(stringConfig(config, "sourcePath"))
	targetPath := strings.TrimSpace(stringConfig(config, "targetPath"))
	if sourceHostID == 0 || sourcePath == "" || targetPath == "" {
		return "failed", "文件分发配置不完整", "", 0, errors.New("文件分发配置不完整")
	}
	hostIDs, groupIDs := opsJobTargetIDs(config)
	hosts, err := s.resolveOpsTargetHosts(hostIDs, groupIDs)
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	sourceHost, stagedPath, err := s.stageRemoteFile(sourceHostID, sourcePath)
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	defer os.Remove(stagedPath)
	task := model.OpsExecTask{
		TaskType:       "file",
		Title:          stepName,
		SourceType:     "server",
		SourceHostID:   sourceHostID,
		SourceHostName: sourceHost.HostName,
		SourcePath:     sourcePath,
		TargetPath:     targetPath,
		FileName:       path.Base(sourcePath),
		Concurrency:    normalizeOpsConcurrency(numberConfig(config, "concurrency")),
		TimeoutSeconds: normalizeOpsTimeout(numberConfig(config, "timeoutSeconds")),
		Status:         "running",
		Summary:        "作业步骤执行中",
		HostCount:      len(hosts),
		Operator:       "job-engine",
		Source:         "job",
		RiskLevel:      "normal",
		TargetSnapshot: opsTargetSnapshot(hosts),
	}
	overwrite := boolConfig(config, "overwrite")
	result, err := s.runOpsTaskLegacy(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		return s.dispatchFileFromPath(host, task.FileName, targetPath, stagedPath, overwrite, task.TimeoutSeconds)
	})
	if err != nil {
		return "failed", err.Error(), "", 0, err
	}
	taskInfo, _ := result["task"].(model.OpsExecTask)
	rows, _ := result["results"].([]model.OpsExecTargetResult)
	outputs := make([]string, 0, len(rows))
	for _, row := range rows {
		line := fmt.Sprintf("%s [%s] exit=%d", row.HostName, row.Status, row.ExitCode)
		if strings.TrimSpace(row.ErrorText) != "" {
			line += " " + strings.TrimSpace(row.ErrorText)
		}
		outputs = append(outputs, line)
	}
	return taskInfo.Status, taskInfo.Summary, strings.Join(outputs, "\n"), taskInfo.ID, nil
}

func (s *Service) executeOpsJobNotifyNode(historyID uint, stepName string, node OpsJobNode) (string, string, string, error) {
	ruleID := uint(numberConfig(node.Config, "notifyRuleId"))
	if ruleID == 0 {
		return "failed", "缺少通知规则", "", errors.New("缺少通知规则")
	}
	var history model.OpsJobHistory
	if err := s.db.First(&history, historyID).Error; err != nil {
		return "failed", err.Error(), "", err
	}
	var job model.OpsJob
	if err := s.db.First(&job, history.JobID).Error; err != nil {
		return "failed", err.Error(), "", err
	}
	now := time.Now()
	summary := firstNonEmpty(stringConfig(node.Config, "message"), stepName)
	detail := firstNonEmpty(stringConfig(node.Config, "content"), history.Summary)
	s.DispatchNotifyRule(ruleID, NotifyEvent{
		Scope:      "job",
		Event:      "notify",
		TargetID:   job.ID,
		TargetName: job.Name,
		Status:     "notice",
		Summary:    summary,
		Detail:     detail,
		StartedAt:  history.StartedAt,
		FinishedAt: &now,
		Extra: map[string]string{
			"jobName":         job.Name,
			"jobHistoryId":    fmt.Sprintf("%d", history.ID),
			"historyId":       fmt.Sprintf("%d", history.ID),
			"triggerType":     history.TriggerType,
			"stepName":        stepName,
			"stepMessage":     summary,
			"notifyAt":        now.Format("2006-01-02 15:04:05"),
			"currentStepId":   node.ID,
			"currentStepName": stepName,
		},
	})
	return "success", "通知已触发", detail, nil
}

func (s *Service) finishOpsJobHistory(historyID uint, status, summary, currentStepID, currentStepName string) {
	finishedAt := time.Now()
	_ = s.db.Model(&model.OpsJobHistory{}).Where("id = ?", historyID).Updates(map[string]any{
		"status":            status,
		"summary":           summary,
		"current_step_id":   currentStepID,
		"current_step_name": currentStepName,
		"finished_at":       &finishedAt,
	}).Error
}

func (s *Service) dispatchOpsJobNotification(historyID uint, event, summary, detail string) {
	var history model.OpsJobHistory
	if err := s.db.First(&history, historyID).Error; err != nil {
		return
	}
	var job model.OpsJob
	if err := s.db.First(&job, history.JobID).Error; err != nil {
		return
	}
	if !job.NotifyEnabled || job.NotifyRuleID == 0 {
		return
	}
	s.DispatchNotifyRule(job.NotifyRuleID, NotifyEvent{
		Scope:      "job",
		Event:      event,
		TargetID:   job.ID,
		TargetName: job.Name,
		Status:     event,
		Summary:    summary,
		Detail:     detail,
		StartedAt:  history.StartedAt,
		FinishedAt: history.FinishedAt,
		Extra: map[string]string{
			"jobName":         job.Name,
			"jobHistoryId":    fmt.Sprintf("%d", history.ID),
			"historyId":       fmt.Sprintf("%d", history.ID),
			"triggerType":     history.TriggerType,
			"stepName":        history.CurrentStepName,
			"currentStepId":   history.CurrentStepID,
			"currentStepName": history.CurrentStepName,
		},
	})
}

func stringConfig(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func numberConfig(config map[string]any, key string) int {
	if config == nil {
		return 0
	}
	value, ok := config[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		var result int
		_, _ = fmt.Sscanf(fmt.Sprintf("%v", value), "%d", &result)
		return result
	}
}

func uintSliceConfig(config map[string]any, key string) []uint {
	if config == nil {
		return nil
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}
	list := make([]uint, 0)
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			number := numberConfig(map[string]any{"value": item}, "value")
			if number > 0 {
				list = append(list, uint(number))
			}
		}
	case []uint:
		return typed
	case []int:
		for _, item := range typed {
			if item > 0 {
				list = append(list, uint(item))
			}
		}
	}
	return list
}

// Older job definitions may retain groupIds after the user switches to explicit hosts.
// Explicit host selection is the more specific scope, so it wins during execution.
func opsJobTargetIDs(config map[string]any) ([]uint, []uint) {
	hostIDs := uintSliceConfig(config, "hostIds")
	groupIDs := uintSliceConfig(config, "groupIds")
	if len(hostIDs) > 0 {
		return hostIDs, nil
	}
	return nil, groupIDs
}

func boolConfig(config map[string]any, key string) bool {
	if config == nil {
		return false
	}
	value, ok := config[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return strings.EqualFold(fmt.Sprintf("%v", value), "true")
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
