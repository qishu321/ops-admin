package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"
	"ops-admin/backend/util"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type OpsScheduleTemplatePayload struct {
	ID             uint              `json:"id"`
	Name           string            `json:"name"`
	TaskType       string            `json:"taskType"`
	ScriptID       uint              `json:"scriptId"`
	Variables      map[string]string `json:"variables"`
	HTTPMethod     string            `json:"httpMethod"`
	URL            string            `json:"url"`
	HeadersJSON    string            `json:"headersJson"`
	Body           string            `json:"body"`
	ExpectedStatus int               `json:"expectedStatus"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	CronExpr       string            `json:"cronExpr"`
	Description    string            `json:"description"`
	Status         int               `json:"status"`
}

type OpsScheduleTaskPayload struct {
	ID                   uint              `json:"id"`
	Name                 string            `json:"name"`
	TaskType             string            `json:"taskType"`
	TemplateID           uint              `json:"templateId"`
	ScriptID             uint              `json:"scriptId"`
	Variables            map[string]string `json:"variables"`
	HostIDs              []uint            `json:"hostIds"`
	GroupIDs             []uint            `json:"groupIds"`
	Concurrency          int               `json:"concurrency"`
	HTTPMethod           string            `json:"httpMethod"`
	URL                  string            `json:"url"`
	HeadersJSON          string            `json:"headersJson"`
	Body                 string            `json:"body"`
	ExpectedStatus       int               `json:"expectedStatus"`
	TimeoutSeconds       int               `json:"timeoutSeconds"`
	RetryEnabled         bool              `json:"retryEnabled"`
	MaxRetries           int               `json:"maxRetries"`
	RetryIntervalSeconds int               `json:"retryIntervalSeconds"`
	RetryBackoff         string            `json:"retryBackoff"`
	AllowUnsafeRetry     bool              `json:"allowUnsafeRetry"`
	CronExpr             string            `json:"cronExpr"`
	Description          string            `json:"description"`
	Status               int               `json:"status"`
	NotifyEnabled        bool              `json:"notifyEnabled"`
	NotifyRuleID         uint              `json:"notifyRuleId"`
	NotifyOnFailureOnly  bool              `json:"notifyOnFailureOnly"`
}

type OpsScheduleNotifyPreviewPayload struct {
	Task          OpsScheduleTaskPayload `json:"task"`
	PreviewStatus string                 `json:"previewStatus"`
}

type OpsScheduleTaskStatusPayload struct {
	IDs    []uint `json:"ids"`
	Status int    `json:"status"`
}

type OpsScheduler struct {
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[uint]cron.EntryID
}

func normalizeScheduleTaskType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http", "probe", "http_probe":
		return "http"
	default:
		return "script"
	}
}

func normalizeScheduleStatus(value int) int {
	if value == 1 {
		return 1
	}
	return 2
}

func normalizeHTTPMethod(value string) string {
	method := strings.ToUpper(strings.TrimSpace(value))
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
		return method
	default:
		return "GET"
	}
}

func normalizeExpectedStatus(value int) int {
	if value <= 0 {
		return 200
	}
	return value
}

func normalizeScheduleMaxRetries(enabled bool, value int) int {
	if !enabled {
		return 0
	}
	if value <= 0 {
		return 2
	}
	if value > 5 {
		return 5
	}
	return value
}

func normalizeScheduleRetryInterval(value int) int {
	if value <= 0 {
		return 5
	}
	if value > 300 {
		return 300
	}
	return value
}

func normalizeScheduleRetryBackoff(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "fixed") {
		return "fixed"
	}
	return "exponential"
}

func scheduleMethodNeedsUnsafeRetryApproval(method string) bool {
	switch normalizeHTTPMethod(method) {
	case "POST", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func normalizeCronExpr(value string) string {
	expr := strings.TrimSpace(value)
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		return "0 " + expr
	}
	return expr
}

func encodeUintList(list []uint) string {
	if len(list) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(list)
	return string(data)
}

func decodeUintList(raw string) []uint {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var list []uint
	_ = json.Unmarshal([]byte(value), &list)
	return list
}

// resolveScheduleScriptVariables validates task/template values against the
// selected script and encrypts values declared as secret before persistence.
// Empty secret inputs preserve an existing configured value on edit.
func resolveScheduleScriptVariables(script *model.OpsScript, supplied map[string]string, existing model.OpsScriptVariableValues) (model.OpsScriptVariableValues, map[string]string, error) {
	if supplied == nil {
		supplied = map[string]string{}
	}
	declared := make(map[string]model.OpsScriptVariable, len(script.Variables))
	for _, variable := range script.Variables {
		declared[variable.Name] = variable
	}
	for name := range supplied {
		if _, ok := declared[name]; !ok {
			return nil, nil, fmt.Errorf("变量 VARIABLE_%s 未在脚本中声明", name)
		}
	}
	stored := model.OpsScriptVariableValues{}
	runtimeValues := map[string]string{}
	for name, variable := range declared {
		rawValue, suppliedValue := supplied[name]
		value := strings.TrimSpace(rawValue)
		if existingValue := strings.TrimSpace(existing[name]); existingValue != "" && (!suppliedValue || (variable.Secret && value == "")) {
			if variable.Secret {
				plain, err := util.DecryptSecret(existingValue)
				if err != nil {
					return nil, nil, fmt.Errorf("读取变量 VARIABLE_%s 失败: %w", name, err)
				}
				value = plain
			} else {
				value = existingValue
			}
		}
		if value == "" {
			value = variable.DefaultValue
		}
		if variable.Required && value == "" {
			return nil, nil, fmt.Errorf("请配置必填变量 VARIABLE_%s", name)
		}
		if value == "" {
			continue
		}
		runtimeValues[name] = value
		if variable.Secret {
			encrypted, err := util.EncryptSecret(value)
			if err != nil {
				return nil, nil, err
			}
			stored[name] = encrypted
		} else {
			stored[name] = value
		}
	}
	return stored, runtimeValues, nil
}

// scheduleVariableResponse never sends encrypted values (or plaintext secrets)
// back to the browser. An empty value means a secret is already configured.
func scheduleVariableResponse(script *model.OpsScript, stored model.OpsScriptVariableValues) map[string]string {
	if script == nil || len(stored) == 0 {
		return map[string]string{}
	}
	result := map[string]string{}
	for _, variable := range script.Variables {
		if value, ok := stored[variable.Name]; ok && !variable.Secret {
			result[variable.Name] = value
		}
	}
	return result
}

func normalizeHeadersJSON(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "{}", nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(value), &headers); err != nil {
		return "", errors.New("请求头必须是 JSON 对象")
	}
	data, _ := json.Marshal(headers)
	return string(data), nil
}

func parseHeaderMap(raw string) map[string]string {
	var headers map[string]string
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return map[string]string{}
	}
	return headers
}

func parseCronExpr(expr string) (cron.Schedule, error) {
	expr = normalizeCronExpr(expr)
	if expr == "" {
		return nil, errors.New("Cron 表达式不能为空")
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return parser.Parse(expr)
}

func (s *Service) initOpsScheduler() {
	s.opsSchedulerOnce.Do(func() {
		s.opsScheduler = &OpsScheduler{
			cron: cron.New(
				cron.WithSeconds(),
				cron.WithLocation(time.Local),
				cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
			),
			entries: map[uint]cron.EntryID{},
		}
		s.opsScheduler.cron.Start()
		s.reloadOpsScheduleTasks()
	})
}

func (s *Service) reloadOpsScheduleTasks() {
	if s.opsScheduler == nil {
		return
	}
	var tasks []model.OpsScheduleTask
	if err := s.db.Where("status = ?", 1).Find(&tasks).Error; err != nil {
		return
	}
	for _, task := range tasks {
		_ = s.registerOpsScheduleTask(task)
	}
}

func (s *Service) registerOpsScheduleTask(task model.OpsScheduleTask) error {
	if s.opsScheduler == nil {
		return nil
	}
	schedule, err := parseCronExpr(task.CronExpr)
	if err != nil {
		return err
	}
	s.removeOpsScheduleTask(task.ID)
	entryID, err := s.opsScheduler.cron.AddFunc(task.CronExpr, func() {
		s.executeScheduledTask(task.ID, "schedule")
	})
	if err != nil {
		return err
	}
	next := schedule.Next(time.Now())
	s.opsScheduler.mu.Lock()
	s.opsScheduler.entries[task.ID] = entryID
	s.opsScheduler.mu.Unlock()
	return s.db.Model(&model.OpsScheduleTask{}).Where("id = ?", task.ID).Update("next_run_at", &next).Error
}

func (s *Service) removeOpsScheduleTask(taskID uint) {
	if s.opsScheduler == nil {
		return
	}
	s.opsScheduler.mu.Lock()
	entryID, ok := s.opsScheduler.entries[taskID]
	if ok {
		delete(s.opsScheduler.entries, taskID)
	}
	s.opsScheduler.mu.Unlock()
	if ok {
		s.opsScheduler.cron.Remove(entryID)
	}
	_ = s.db.Model(&model.OpsScheduleTask{}).Where("id = ?", taskID).Update("next_run_at", nil).Error
}

func mapScheduleTaskItem(item model.OpsScheduleTask) map[string]any {
	return map[string]any{
		"id":                   item.ID,
		"name":                 item.Name,
		"taskType":             item.TaskType,
		"templateId":           item.TemplateID,
		"scriptId":             item.ScriptID,
		"scriptName":           item.ScriptName,
		"parameters":           item.Parameters,
		"hostIds":              decodeUintList(item.HostIDsJSON),
		"groupIds":             decodeUintList(item.GroupIDsJSON),
		"concurrency":          item.Concurrency,
		"httpMethod":           item.HTTPMethod,
		"url":                  item.URL,
		"headersJson":          firstNonEmpty(item.HeadersJSON, "{}"),
		"body":                 item.Body,
		"expectedStatus":       item.ExpectedStatus,
		"timeoutSeconds":       item.TimeoutSeconds,
		"retryEnabled":         item.RetryEnabled,
		"maxRetries":           item.MaxRetries,
		"retryIntervalSeconds": item.RetryIntervalSeconds,
		"retryBackoff":         firstNonEmpty(item.RetryBackoff, "exponential"),
		"allowUnsafeRetry":     item.AllowUnsafeRetry,
		"cronExpr":             item.CronExpr,
		"description":          item.Description,
		"status":               item.Status,
		"notifyEnabled":        item.NotifyEnabled,
		"notifyRuleId":         item.NotifyRuleID,
		"notifyOnFailureOnly":  item.NotifyOnFailureOnly,
		"lastStatus":           item.LastStatus,
		"lastSummary":          item.LastSummary,
		"lastRunAt":            item.LastRunAt,
		"nextRunAt":            item.NextRunAt,
		"createTime":           item.CreatedAt,
		"updateTime":           item.UpdatedAt,
	}
}

func mapScheduleTemplateItem(item model.OpsScheduleTemplate) map[string]any {
	return map[string]any{
		"id":             item.ID,
		"name":           item.Name,
		"taskType":       item.TaskType,
		"scriptId":       item.ScriptID,
		"scriptName":     item.ScriptName,
		"parameters":     item.Parameters,
		"httpMethod":     item.HTTPMethod,
		"url":            item.URL,
		"headersJson":    firstNonEmpty(item.HeadersJSON, "{}"),
		"body":           item.Body,
		"expectedStatus": item.ExpectedStatus,
		"timeoutSeconds": item.TimeoutSeconds,
		"cronExpr":       item.CronExpr,
		"description":    item.Description,
		"status":         item.Status,
		"createTime":     item.CreatedAt,
		"updateTime":     item.UpdatedAt,
	}
}

func (s *Service) ListOpsScheduleTasks(pageNum, pageSize int, keyword, taskType, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsScheduleTask{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR script_name LIKE ? OR url LIKE ?", like, like, like, like)
	}
	if strings.TrimSpace(taskType) != "" {
		query = query.Where("task_type = ?", normalizeScheduleTaskType(taskType))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsScheduleTask
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapScheduleTaskItem(item))
	}
	return map[string]any{
		"list":     rows,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetOpsScheduleTask(id uint) (map[string]any, error) {
	var item model.OpsScheduleTask
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	result := mapScheduleTaskItem(item)
	if item.ScriptID > 0 {
		if script, err := s.GetOpsScript(item.ScriptID); err == nil {
			result["variables"] = scheduleVariableResponse(script, item.Variables)
		}
	}
	return result, nil
}

func (s *Service) buildOpsScheduleTaskUpdates(payload OpsScheduleTaskPayload, existing *model.OpsScheduleTask) (map[string]any, error) {
	taskType := normalizeScheduleTaskType(payload.TaskType)
	name := Trimmed(payload.Name)
	if name == "" {
		return nil, errors.New("任务名称不能为空")
	}
	if _, err := parseCronExpr(payload.CronExpr); err != nil {
		return nil, errors.New("Cron 表达式格式不正确")
	}
	headersJSON, err := normalizeHeadersJSON(payload.HeadersJSON)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{
		"name":                   name,
		"task_type":              taskType,
		"template_id":            payload.TemplateID,
		"parameters":             "",
		"host_ids_json":          encodeUintList(payload.HostIDs),
		"group_ids_json":         encodeUintList(payload.GroupIDs),
		"concurrency":            normalizeOpsConcurrency(payload.Concurrency),
		"http_method":            normalizeHTTPMethod(payload.HTTPMethod),
		"url":                    strings.TrimSpace(payload.URL),
		"headers_json":           headersJSON,
		"body":                   payload.Body,
		"expected_status":        normalizeExpectedStatus(payload.ExpectedStatus),
		"timeout_seconds":        normalizeOpsTimeout(payload.TimeoutSeconds),
		"retry_enabled":          payload.RetryEnabled,
		"max_retries":            normalizeScheduleMaxRetries(payload.RetryEnabled, payload.MaxRetries),
		"retry_interval_seconds": normalizeScheduleRetryInterval(payload.RetryIntervalSeconds),
		"retry_backoff":          normalizeScheduleRetryBackoff(payload.RetryBackoff),
		"allow_unsafe_retry":     payload.RetryEnabled && payload.AllowUnsafeRetry,
		"cron_expr":              normalizeCronExpr(payload.CronExpr),
		"description":            Trimmed(payload.Description),
		"status":                 normalizeScheduleStatus(payload.Status),
		"notify_enabled":         payload.NotifyEnabled,
		"notify_rule_id":         payload.NotifyRuleID,
		"notify_on_failure_only": payload.NotifyOnFailureOnly,
		"next_run_at":            nil,
	}

	switch taskType {
	case "script":
		if payload.ScriptID == 0 {
			return nil, errors.New("请选择脚本")
		}
		script, err := s.GetOpsScript(payload.ScriptID)
		if err != nil {
			return nil, err
		}
		if script.Status != 1 {
			return nil, errors.New("脚本已禁用，不能用于定时任务")
		}
		if len(payload.HostIDs) == 0 && len(payload.GroupIDs) == 0 {
			return nil, errors.New("请选择目标主机或主机组")
		}
		if len(payload.HostIDs) > 0 && len(payload.GroupIDs) > 0 {
			return nil, errors.New("目标主机和主机组只能二选一")
		}
		updates["script_id"] = script.ID
		updates["script_name"] = script.Name
		variables, _, err := resolveScheduleScriptVariables(script, payload.Variables, func() model.OpsScriptVariableValues {
			if existing == nil {
				return model.OpsScriptVariableValues{}
			}
			return existing.Variables
		}())
		if err != nil {
			return nil, err
		}
		updates["variables"] = variables
		updates["timeout_seconds"] = normalizeOpsTimeout(script.TimeoutSeconds)
		updates["http_method"] = ""
		updates["url"] = ""
		updates["headers_json"] = "{}"
		updates["body"] = ""
		updates["expected_status"] = 0
		updates["retry_enabled"] = false
		updates["max_retries"] = 0
		updates["allow_unsafe_retry"] = false
	case "http":
		if strings.TrimSpace(payload.URL) == "" {
			return nil, errors.New("HTTP 地址不能为空")
		}
		if payload.RetryEnabled && scheduleMethodNeedsUnsafeRetryApproval(payload.HTTPMethod) && !payload.AllowUnsafeRetry {
			return nil, errors.New("POST、PATCH、DELETE 请求开启重试前必须确认允许重复提交")
		}
		updates["script_id"] = 0
		updates["script_name"] = ""
		updates["variables"] = model.OpsScriptVariableValues{}
		updates["host_ids_json"] = "[]"
		updates["group_ids_json"] = "[]"
		updates["concurrency"] = 1
	default:
		return nil, errors.New("不支持的任务类型")
	}

	if existing != nil {
		updates["last_status"] = existing.LastStatus
		updates["last_summary"] = existing.LastSummary
		updates["last_run_at"] = existing.LastRunAt
	}
	return updates, nil
}

func (s *Service) CreateOpsScheduleTask(payload OpsScheduleTaskPayload) error {
	updates, err := s.buildOpsScheduleTaskUpdates(payload, nil)
	if err != nil {
		return err
	}
	item := model.OpsScheduleTask{}
	if err := s.db.Model(&item).Create(updates).Error; err != nil {
		return err
	}
	if err := s.db.Last(&item).Error; err != nil {
		return err
	}
	if item.Status == 1 {
		return s.registerOpsScheduleTask(item)
	}
	return nil
}

func (s *Service) UpdateOpsScheduleTask(payload OpsScheduleTaskPayload) error {
	var existing model.OpsScheduleTask
	if err := s.db.First(&existing, payload.ID).Error; err != nil {
		return err
	}
	updates, err := s.buildOpsScheduleTaskUpdates(payload, &existing)
	if err != nil {
		return err
	}
	if err := s.db.Model(&model.OpsScheduleTask{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
		return err
	}
	var current model.OpsScheduleTask
	if err := s.db.First(&current, payload.ID).Error; err != nil {
		return err
	}
	if current.Status == 1 {
		return s.registerOpsScheduleTask(current)
	}
	s.removeOpsScheduleTask(current.ID)
	return nil
}

func (s *Service) DeleteOpsScheduleTask(id uint) error {
	s.removeOpsScheduleTask(id)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.OpsScheduleTask{}, id).Error; err != nil {
			return err
		}
		return tx.Where("task_id = ?", id).Delete(&model.OpsScheduleTaskLog{}).Error
	})
}

func (s *Service) BatchDeleteOpsScheduleTasks(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		s.removeOpsScheduleTask(id)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id IN ?", ids).Delete(&model.OpsScheduleTask{}).Error; err != nil {
			return err
		}
		return tx.Where("task_id IN ?", ids).Delete(&model.OpsScheduleTaskLog{}).Error
	})
}

func (s *Service) UpdateOpsScheduleTaskStatus(payload OpsScheduleTaskStatusPayload) error {
	if len(payload.IDs) == 0 {
		return errors.New("请选择任务")
	}
	status := normalizeScheduleStatus(payload.Status)
	if err := s.db.Model(&model.OpsScheduleTask{}).Where("id IN ?", payload.IDs).Update("status", status).Error; err != nil {
		return err
	}
	var tasks []model.OpsScheduleTask
	if err := s.db.Where("id IN ?", payload.IDs).Find(&tasks).Error; err != nil {
		return err
	}
	for _, task := range tasks {
		if status == 1 {
			if err := s.registerOpsScheduleTask(task); err != nil {
				return err
			}
		} else {
			s.removeOpsScheduleTask(task.ID)
		}
	}
	return nil
}

func (s *Service) RunOpsScheduleTask(id uint) error {
	var task model.OpsScheduleTask
	if err := s.db.First(&task, id).Error; err != nil {
		return err
	}
	go s.executeScheduledTask(task.ID, "manual")
	return nil
}

func (s *Service) ListOpsScheduleTemplates(pageNum, pageSize int, keyword, taskType, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsScheduleTemplate{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR script_name LIKE ? OR url LIKE ?", like, like, like, like)
	}
	if strings.TrimSpace(taskType) != "" {
		query = query.Where("task_type = ?", normalizeScheduleTaskType(taskType))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsScheduleTemplate
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapScheduleTemplateItem(item))
	}
	return map[string]any{
		"list":     rows,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetOpsScheduleTemplate(id uint) (map[string]any, error) {
	var item model.OpsScheduleTemplate
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	result := mapScheduleTemplateItem(item)
	if item.ScriptID > 0 {
		if script, err := s.GetOpsScript(item.ScriptID); err == nil {
			result["variables"] = scheduleVariableResponse(script, item.Variables)
		}
	}
	return result, nil
}

func (s *Service) buildOpsScheduleTemplateUpdates(payload OpsScheduleTemplatePayload, existing *model.OpsScheduleTemplate) (map[string]any, error) {
	taskType := normalizeScheduleTaskType(payload.TaskType)
	name := Trimmed(payload.Name)
	if name == "" {
		return nil, errors.New("模板名称不能为空")
	}
	headersJSON, err := normalizeHeadersJSON(payload.HeadersJSON)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.CronExpr) != "" {
		if _, err := parseCronExpr(payload.CronExpr); err != nil {
			return nil, errors.New("Cron 表达式格式不正确")
		}
	}
	updates := map[string]any{
		"name":            name,
		"task_type":       taskType,
		"parameters":      "",
		"http_method":     normalizeHTTPMethod(payload.HTTPMethod),
		"url":             strings.TrimSpace(payload.URL),
		"headers_json":    headersJSON,
		"body":            payload.Body,
		"expected_status": normalizeExpectedStatus(payload.ExpectedStatus),
		"timeout_seconds": normalizeOpsTimeout(payload.TimeoutSeconds),
		"cron_expr":       normalizeCronExpr(payload.CronExpr),
		"description":     Trimmed(payload.Description),
		"status":          normalizeScheduleStatus(payload.Status),
	}
	switch taskType {
	case "script":
		if payload.ScriptID == 0 {
			return nil, errors.New("请选择脚本")
		}
		script, err := s.GetOpsScript(payload.ScriptID)
		if err != nil {
			return nil, err
		}
		updates["script_id"] = script.ID
		updates["script_name"] = script.Name
		variables, _, err := resolveScheduleScriptVariables(script, payload.Variables, func() model.OpsScriptVariableValues {
			if existing == nil {
				return model.OpsScriptVariableValues{}
			}
			return existing.Variables
		}())
		if err != nil {
			return nil, err
		}
		updates["variables"] = variables
		updates["timeout_seconds"] = normalizeOpsTimeout(script.TimeoutSeconds)
		updates["http_method"] = ""
		updates["url"] = ""
		updates["headers_json"] = "{}"
		updates["body"] = ""
		updates["expected_status"] = 0
	case "http":
		if strings.TrimSpace(payload.URL) == "" {
			return nil, errors.New("HTTP 地址不能为空")
		}
		updates["script_id"] = 0
		updates["script_name"] = ""
		updates["variables"] = model.OpsScriptVariableValues{}
	default:
		return nil, errors.New("不支持的模板类型")
	}
	return updates, nil
}

func (s *Service) CreateOpsScheduleTemplate(payload OpsScheduleTemplatePayload) error {
	updates, err := s.buildOpsScheduleTemplateUpdates(payload, nil)
	if err != nil {
		return err
	}
	return s.db.Model(&model.OpsScheduleTemplate{}).Create(updates).Error
}

func (s *Service) UpdateOpsScheduleTemplate(payload OpsScheduleTemplatePayload) error {
	var existing model.OpsScheduleTemplate
	if err := s.db.First(&existing, payload.ID).Error; err != nil {
		return err
	}
	updates, err := s.buildOpsScheduleTemplateUpdates(payload, &existing)
	if err != nil {
		return err
	}
	return s.db.Model(&model.OpsScheduleTemplate{}).Where("id = ?", payload.ID).Updates(updates).Error
}

func (s *Service) DeleteOpsScheduleTemplate(id uint) error {
	var count int64
	if err := s.db.Model(&model.OpsScheduleTask{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该模板仍被任务引用，不能删除")
	}
	return s.db.Delete(&model.OpsScheduleTemplate{}, id).Error
}

func (s *Service) ListOpsScheduleTaskLogs(pageNum, pageSize int, keyword, taskType, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsScheduleTaskLog{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("task_name LIKE ? OR summary LIKE ?", like, like)
	}
	if strings.TrimSpace(taskType) != "" {
		query = query.Where("task_type = ?", normalizeScheduleTaskType(taskType))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsScheduleTaskLog
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

func (s *Service) GetOpsScheduleTaskLog(id uint) (*model.OpsScheduleTaskLog, error) {
	var item model.OpsScheduleTaskLog
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) executeScheduledTask(taskID uint, triggerType string) {
	var task model.OpsScheduleTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return
	}
	if strings.TrimSpace(triggerType) == "" {
		triggerType = "schedule"
	}
	startedAt := time.Now()
	logItem := model.OpsScheduleTaskLog{
		TaskID:      task.ID,
		TaskName:    task.Name,
		TaskType:    task.TaskType,
		TriggerType: triggerType,
		Status:      "running",
		Summary:     "??????????",
		StartedAt:   &startedAt,
	}
	_ = s.db.Create(&logItem).Error

	var (
		status       string
		summary      string
		detail       string
		execTaskID   uint
		httpCode     int
		responseRaw  string
		attemptCount = 1
	)

	switch task.TaskType {
	case "http":
		result := s.runScheduledHTTPTask(task)
		status, summary, detail = result.Status, result.Summary, result.Detail
		httpCode, responseRaw, attemptCount = result.HTTPCode, result.ResponseBody, result.AttemptCount
	default:
		status, summary, detail, execTaskID = s.runScheduledScriptTask(task)
	}

	finishedAt := time.Now()
	nextRunAt := nextRunTime(task.CronExpr)
	_ = s.db.Model(&model.OpsScheduleTaskLog{}).Where("id = ?", logItem.ID).Updates(map[string]any{
		"status":          status,
		"summary":         summary,
		"detail":          detail,
		"exec_task_id":    execTaskID,
		"expected_status": task.ExpectedStatus,
		"actual_status":   httpCode,
		"response_body":   responseRaw,
		"finished_at":     &finishedAt,
		"duration_ms":     finishedAt.Sub(startedAt).Milliseconds(),
		"attempt_count":   attemptCount,
	}).Error
	_ = s.db.Model(&model.OpsScheduleTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"last_status":  status,
		"last_summary": summary,
		"last_run_at":  &finishedAt,
		"next_run_at":  nextRunAt,
	}).Error
	if task.NotifyEnabled && task.NotifyRuleID > 0 && (!task.NotifyOnFailureOnly || !strings.EqualFold(status, "success")) {
		duration := finishedAt.Sub(startedAt)
		s.DispatchNotifyRule(task.NotifyRuleID, NotifyEvent{
			Scope:      "schedule",
			Event:      status,
			TargetID:   task.ID,
			TargetName: task.Name,
			Status:     status,
			Summary:    summary,
			// The execution log keeps the complete response body. Notifications
			// intentionally use a compact result, otherwise a successful HTTP
			// probe can push an entire HTML page into chat.
			Detail:     compactScheduleNotifyDetail(task, status, detail, httpCode, attemptCount),
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
			Extra: map[string]string{
				"taskName":       task.Name,
				"taskType":       scheduleTaskTypeLabel(task.TaskType),
				"triggerType":    scheduleTriggerTypeLabel(triggerType),
				"cronExpr":       task.CronExpr,
				"duration":       formatScheduleDuration(duration),
				"durationMs":     fmt.Sprintf("%d", duration.Milliseconds()),
				"httpStatus":     formatScheduleHTTPStatus(httpCode),
				"expectedStatus": fmt.Sprintf("%d", normalizeExpectedStatus(task.ExpectedStatus)),
				"attemptCount":   strconv.Itoa(attemptCount),
				"retryCount":     strconv.Itoa(maxInt(attemptCount-1, 0)),
				"alertName":      task.Name,
				"severity":       "定时任务",
			},
		})
	}
}

func compactScheduleNotifyDetail(task model.OpsScheduleTask, status, detail string, httpCode, attemptCount int) string {
	if task.TaskType == "http" {
		return scheduleHTTPNotifyDetail(status, httpCode, normalizeExpectedStatus(task.ExpectedStatus), attemptCount)
	}
	return trimNotifyText(detail, 1200)
}

func scheduleHTTPNotifyDetail(status string, httpCode, expectedStatus, attemptCount int) string {
	attemptText := ""
	if attemptCount > 1 {
		attemptText = fmt.Sprintf("共尝试 %d 次。", attemptCount)
	}
	if strings.EqualFold(status, "success") {
		return fmt.Sprintf("HTTP 探针返回 %d，符合预期状态码 %d。%s", httpCode, expectedStatus, attemptText)
	}
	if httpCode > 0 {
		return fmt.Sprintf("HTTP 探针返回 %d，未达到预期状态码 %d。%s完整响应内容请在任务日志中查看。", httpCode, expectedStatus, attemptText)
	}
	return "HTTP 探针请求失败。" + attemptText + "完整错误信息请在任务日志中查看。"
}

func trimNotifyText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...（执行详情已截断，请在任务日志中查看完整输出）"
}

func scheduleTaskTypeLabel(value string) string {
	if value == "http" {
		return "HTTP 探针"
	}
	return "脚本任务"
}

func scheduleTriggerTypeLabel(value string) string {
	if value == "manual" {
		return "手动执行"
	}
	return "定时触发"
}

func formatScheduleDuration(value time.Duration) string {
	if value < time.Second {
		return fmt.Sprintf("%d 毫秒", value.Milliseconds())
	}
	return value.Round(time.Millisecond).String()
}

func formatScheduleHTTPStatus(value int) string {
	if value <= 0 {
		return "-"
	}
	return strconv.Itoa(value)
}

func nextRunTime(expr string) *time.Time {
	schedule, err := parseCronExpr(expr)
	if err != nil {
		return nil
	}
	next := schedule.Next(time.Now())
	return &next
}

func (s *Service) runScheduledScriptTask(task model.OpsScheduleTask) (string, string, string, uint) {
	hostIDs := decodeUintList(task.HostIDsJSON)
	groupIDs := decodeUintList(task.GroupIDsJSON)
	hosts, err := s.resolveOpsTargetHosts(hostIDs, groupIDs)
	if err != nil {
		return "failed", err.Error(), err.Error(), 0
	}
	script, err := s.GetOpsScript(task.ScriptID)
	if err != nil {
		return "failed", err.Error(), err.Error(), 0
	}
	execTask := model.OpsExecTask{
		TaskType:       "script",
		Title:          fmt.Sprintf("定时任务 - %s", task.Name),
		ScriptID:       script.ID,
		ScriptName:     script.Name,
		Parameters:     "",
		Concurrency:    normalizeOpsConcurrency(task.Concurrency),
		TimeoutSeconds: normalizeOpsTimeout(script.TimeoutSeconds),
		Status:         "running",
		Summary:        "定时任务执行中",
		HostCount:      len(hosts),
		Operator:       "scheduler",
		Source:         "schedule",
		RiskLevel:      opsRiskLevel(script.Content),
		ScriptVersion:  script.CurrentVersion,
		TargetSnapshot: opsTargetSnapshot(hosts),
	}
	data, err := s.runOpsTaskLegacy(execTask, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		_, variables, resolveErr := resolveScheduleScriptVariables(script, nil, task.Variables)
		if resolveErr != nil {
			return model.OpsExecTargetResult{HostID: host.ID, HostName: host.HostName, SSHIP: host.SSHIP, Status: "failed", ErrorText: resolveErr.Error()}
		}
		return s.execScriptOnHost(host, *script, "", variables, execTask.TimeoutSeconds)
	})
	if err != nil {
		return "failed", err.Error(), err.Error(), 0
	}
	taskInfo, _ := data["task"].(model.OpsExecTask)
	results, _ := data["results"].([]model.OpsExecTargetResult)
	lines := make([]string, 0, len(results))
	for _, row := range results {
		lines = append(lines, fmt.Sprintf("%s [%s] exit=%d", row.HostName, row.Status, row.ExitCode))
	}
	status := firstNonEmpty(taskInfo.Status, "success")
	return status, firstNonEmpty(taskInfo.Summary, "执行完成"), strings.Join(lines, "\n"), taskInfo.ID
}

type scheduledHTTPRunResult struct {
	Status       string
	Summary      string
	Detail       string
	HTTPCode     int
	ResponseBody string
	AttemptCount int
}

func (s *Service) runScheduledHTTPTask(task model.OpsScheduleTask) scheduledHTTPRunResult {
	client := &http.Client{Timeout: time.Duration(normalizeOpsTimeout(task.TimeoutSeconds)) * time.Second}
	return executeScheduledHTTPTask(task, client.Do, time.Sleep)
}

func executeScheduledHTTPTask(task model.OpsScheduleTask, do func(*http.Request) (*http.Response, error), sleep func(time.Duration)) scheduledHTTPRunResult {
	maxAttempts := 1 + normalizeScheduleMaxRetries(task.RetryEnabled, task.MaxRetries)
	expectedStatus := normalizeExpectedStatus(task.ExpectedStatus)
	method := normalizeHTTPMethod(task.HTTPMethod)
	detailLines := make([]string, 0, maxAttempts*2)
	lastCode := 0
	lastBody := ""
	lastError := ""

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		startedAt := time.Now()
		request, err := http.NewRequest(method, task.URL, strings.NewReader(task.Body))
		if err != nil {
			message := err.Error()
			return scheduledHTTPRunResult{Status: "failed", Summary: message, Detail: message, AttemptCount: attempt}
		}
		for key, value := range parseHeaderMap(task.HeadersJSON) {
			request.Header.Set(key, value)
		}

		response, requestErr := do(request)
		duration := time.Since(startedAt)
		if requestErr != nil {
			lastError = requestErr.Error()
			lastCode = 0
			lastBody = ""
			detailLines = append(detailLines, fmt.Sprintf("第 %d 次：请求失败（%s），耗时 %s", attempt, lastError, formatScheduleDuration(duration)))
		} else {
			bodyBytes, readErr := io.ReadAll(io.LimitReader(response.Body, 32768))
			_ = response.Body.Close()
			lastCode = response.StatusCode
			lastBody = string(bodyBytes)
			if readErr != nil {
				lastError = readErr.Error()
				detailLines = append(detailLines, fmt.Sprintf("第 %d 次：读取响应失败（%s），耗时 %s", attempt, lastError, formatScheduleDuration(duration)))
			} else {
				lastError = ""
				detailLines = append(detailLines, fmt.Sprintf("第 %d 次：HTTP %d，耗时 %s", attempt, lastCode, formatScheduleDuration(duration)))
				if lastCode == expectedStatus {
					return scheduledHTTPRunResult{
						Status: "success", Summary: fmt.Sprintf("%s %s -> %d（第 %d 次尝试成功）", method, task.URL, lastCode, attempt),
						Detail: strings.Join(detailLines, "\n"), HTTPCode: lastCode, ResponseBody: lastBody, AttemptCount: attempt,
					}
				}
			}
		}

		if attempt < maxAttempts {
			delay := scheduleHTTPRetryDelay(task, attempt)
			detailLines = append(detailLines, fmt.Sprintf("等待 %s 后重试", formatScheduleDuration(delay)))
			sleep(delay)
		}
	}

	summary := lastError
	if lastCode > 0 {
		summary = fmt.Sprintf("%s %s -> %d，期望状态码 %d，已尝试 %d 次", method, task.URL, lastCode, expectedStatus, maxAttempts)
	} else if summary == "" {
		summary = fmt.Sprintf("%s %s 请求失败，已尝试 %d 次", method, task.URL, maxAttempts)
	} else {
		summary = fmt.Sprintf("%s（已尝试 %d 次）", summary, maxAttempts)
	}
	return scheduledHTTPRunResult{
		Status: "failed", Summary: summary, Detail: strings.Join(detailLines, "\n"),
		HTTPCode: lastCode, ResponseBody: lastBody, AttemptCount: maxAttempts,
	}
}

func scheduleHTTPRetryDelay(task model.OpsScheduleTask, completedAttempt int) time.Duration {
	seconds := normalizeScheduleRetryInterval(task.RetryIntervalSeconds)
	if normalizeScheduleRetryBackoff(task.RetryBackoff) == "exponential" {
		for step := 1; step < completedAttempt && seconds < 300; step++ {
			seconds *= 2
			if seconds > 300 {
				seconds = 300
			}
		}
	}
	return time.Duration(seconds) * time.Second
}

func buildOpsScheduleNotifyPreviewEvent(payload OpsScheduleTaskPayload, previewStatus string) (NotifyEvent, error) {
	status := strings.ToLower(strings.TrimSpace(previewStatus))
	if status != "success" && status != "failed" {
		return NotifyEvent{}, errors.New("预览状态只能是成功或失败")
	}
	if payload.NotifyOnFailureOnly && status != "failed" {
		return NotifyEvent{}, errors.New("当前通知策略为仅失败时通知，只能预览失败通知")
	}

	now := time.Now()
	taskType := normalizeScheduleTaskType(payload.TaskType)
	taskName := firstNonEmpty(strings.TrimSpace(payload.Name), "未命名定时任务")
	attemptCount := 1
	if taskType == "http" && payload.RetryEnabled {
		attemptCount += normalizeScheduleMaxRetries(true, payload.MaxRetries)
	}
	expectedStatus := normalizeExpectedStatus(payload.ExpectedStatus)
	actualStatus := expectedStatus
	summary := "任务执行成功"
	detail := "预览数据：任务首次执行即成功。"
	if status == "failed" {
		actualStatus = http.StatusInternalServerError
		if actualStatus == expectedStatus {
			actualStatus = http.StatusBadRequest
		}
		summary = "任务执行失败"
		detail = "预览数据：任务执行失败，已达到最大尝试次数。"
	}
	if taskType == "http" {
		method := normalizeHTTPMethod(payload.HTTPMethod)
		url := firstNonEmpty(strings.TrimSpace(payload.URL), "https://example.com/healthz")
		if status == "success" {
			summary = fmt.Sprintf("%s %s -> %d（第 1 次尝试成功）", method, url, actualStatus)
			detail = fmt.Sprintf("第 1 次：HTTP %d，耗时 120ms", actualStatus)
		} else {
			summary = fmt.Sprintf("%s %s -> %d，期望状态码 %d，已尝试 %d 次", method, url, actualStatus, expectedStatus, attemptCount)
			lines := make([]string, 0, attemptCount)
			for attempt := 1; attempt <= attemptCount; attempt++ {
				lines = append(lines, fmt.Sprintf("第 %d 次：HTTP %d，未达到期望状态码 %d", attempt, actualStatus, expectedStatus))
			}
			detail = strings.Join(lines, "\n")
		}
	}

	return NotifyEvent{
		Scope: "schedule", Event: status, TargetID: payload.ID, TargetName: taskName,
		Status: status, Summary: summary, Detail: detail, StartedAt: &now, FinishedAt: &now,
		Extra: map[string]string{
			"taskName": taskName, "taskType": map[string]string{"http": "HTTP 探针", "script": "脚本任务"}[taskType],
			"triggerType": "发送预览", "cronExpr": firstNonEmpty(strings.TrimSpace(payload.CronExpr), "-"),
			"duration": "120ms", "durationMs": "120", "httpStatus": strconv.Itoa(actualStatus),
			"expectedStatus": strconv.Itoa(expectedStatus), "attemptCount": strconv.Itoa(attemptCount),
			"retryCount": strconv.Itoa(attemptCount - 1), "alertName": taskName, "severity": "warning",
		},
	}, nil
}

func (s *Service) PreviewOpsScheduleTaskNotification(payload OpsScheduleNotifyPreviewPayload) (map[string]any, error) {
	if !payload.Task.NotifyEnabled {
		return nil, errors.New("请先开启消息通知")
	}
	if payload.Task.NotifyRuleID == 0 {
		return nil, errors.New("请选择通知规则")
	}
	event, err := buildOpsScheduleNotifyPreviewEvent(payload.Task, payload.PreviewStatus)
	if err != nil {
		return nil, err
	}

	var rule model.NotifyRule
	if err := s.db.First(&rule, payload.Task.NotifyRuleID).Error; err != nil {
		return nil, fmt.Errorf("读取通知规则失败: %w", err)
	}
	if rule.Status != 1 {
		return nil, errors.New("通知规则已禁用")
	}
	if normalizeNotifyScope(rule.Scope) != "all" && normalizeNotifyScope(rule.Scope) != "schedule" {
		return nil, errors.New("所选通知规则不适用于定时任务")
	}
	if !notifyEventMatch(decodeStringList(rule.EventsJSON), event.Event, event.Status) {
		return nil, fmt.Errorf("所选通知规则未订阅%s事件", scheduleNotifyStatusLabel(event.Status))
	}

	var tmpl model.NotifyTemplate
	if err := s.db.First(&tmpl, rule.TemplateID).Error; err != nil {
		return nil, fmt.Errorf("读取消息模板失败: %w", err)
	}
	if tmpl.Status != 1 {
		return nil, errors.New("消息模板已禁用")
	}
	if !notifyTemplateScopeCompatible(tmpl.Scope, event.Scope) {
		return nil, fmt.Errorf("消息模板适用于%s，不能处理定时任务事件", notifyScopeLabel(tmpl.Scope))
	}

	channelIDs := decodeUintList(rule.ChannelIDsJSON)
	if len(channelIDs) == 0 {
		return nil, errors.New("通知规则未配置通知媒介")
	}
	var channels []model.NotifyChannel
	if err := s.db.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, errors.New("通知规则关联的媒介不存在")
	}

	titleTemplate, contentTemplate := normalizeNotifyTemplateForEvent(firstNonEmpty(tmpl.Title, event.TargetName), tmpl.Content, event)
	title := renderNotifyTemplate(titleTemplate, event)
	content := renderNotifyTemplate(contentTemplate, event)
	previewChannels := make([]map[string]any, 0, len(channels))
	for _, channel := range channels {
		if channel.Status != 1 {
			return nil, fmt.Errorf("通知媒介「%s」已禁用", channel.Name)
		}
		if normalizeNotifyChannelType(channel.ChannelType) != normalizeNotifyChannelType(tmpl.ChannelType) {
			return nil, fmt.Errorf("消息模板与通知媒介「%s」类型不兼容", channel.Name)
		}
		if _, err := buildNotifyBody(channel.ChannelType, title, content, event); err != nil {
			return nil, fmt.Errorf("生成通知媒介「%s」消息失败: %w", channel.Name, err)
		}
		previewChannels = append(previewChannels, map[string]any{
			"id": channel.ID, "name": channel.Name, "channelType": normalizeNotifyChannelType(channel.ChannelType),
		})
	}

	return map[string]any{
		"previewStatus": event.Status, "statusLabel": scheduleNotifyStatusLabel(event.Status),
		"ruleName": rule.Name, "templateName": tmpl.Name, "title": title, "content": content,
		"channels": previewChannels, "attemptCount": event.Extra["attemptCount"],
	}, nil
}
