package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type OpsScriptPayload struct {
	ID             uint                      `json:"id"`
	Name           string                    `json:"name"`
	ScriptType     string                    `json:"scriptType"`
	Interpreter    string                    `json:"interpreter"`
	Content        string                    `json:"content"`
	Variables      []model.OpsScriptVariable `json:"variables"`
	TimeoutSeconds int                       `json:"timeoutSeconds"`
	Status         int                       `json:"status"`
	Description    string                    `json:"description"`
	ChangeSummary  string                    `json:"changeSummary"`
	Operator       string                    `json:"-"`
}

type OpsScriptStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type OpsExecCommandPayload struct {
	Title          string `json:"title"`
	CommandText    string `json:"commandText"`
	Parameters     string `json:"parameters"`
	HostIDs        []uint `json:"hostIds"`
	GroupIDs       []uint `json:"groupIds"`
	Concurrency    int    `json:"concurrency"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	RiskConfirmed  bool   `json:"riskConfirmed"`
	Operator       string `json:"-"`
	SourceIP       string `json:"-"`
	Source         string `json:"-"`
	RetryOfTaskID  uint   `json:"-"`
}

type OpsExecScriptPayload struct {
	Title          string            `json:"title"`
	ScriptID       uint              `json:"scriptId"`
	Parameters     string            `json:"parameters"`
	Variables      map[string]string `json:"variables"`
	HostIDs        []uint            `json:"hostIds"`
	GroupIDs       []uint            `json:"groupIds"`
	Concurrency    int               `json:"concurrency"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	RiskConfirmed  bool              `json:"riskConfirmed"`
	Operator       string            `json:"-"`
	SourceIP       string            `json:"-"`
	Source         string            `json:"-"`
	RetryOfTaskID  uint              `json:"-"`
}

var opsScriptVariableName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type OpsFileDispatchPayload struct {
	Title          string `json:"title"`
	SourceType     string `json:"sourceType"`
	SourceHostID   uint   `json:"sourceHostId"`
	SourcePath     string `json:"sourcePath"`
	TargetDir      string `json:"targetDir"`
	TargetFileName string `json:"targetFileName"`
	// TargetPath is retained for existing API callers and job definitions that
	// already provide a complete destination file path.
	TargetPath     string `json:"targetPath"`
	HostIDs        []uint `json:"hostIds"`
	GroupIDs       []uint `json:"groupIds"`
	Concurrency    int    `json:"concurrency"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Overwrite      bool   `json:"overwrite"`
	RiskConfirmed  bool   `json:"riskConfirmed"`
	Operator       string `json:"-"`
	SourceIP       string `json:"-"`
	Source         string `json:"-"`
}

const OpsFileDispatchMaxUploadBytes int64 = 500 * 1024 * 1024

// OpsFileDispatchUpload is a short-lived staged upload. It must remain on
// disk until every asynchronous target transfer completes.
type OpsFileDispatchUpload struct {
	FileName string
	TempPath string
	Size     int64
}

func (upload *OpsFileDispatchUpload) Cleanup() {
	if upload != nil && strings.TrimSpace(upload.TempPath) != "" {
		_ = os.Remove(upload.TempPath)
	}
}

type OpsExecRetryPayload struct {
	TaskID   uint   `json:"taskId"`
	Operator string `json:"-"`
	SourceIP string `json:"-"`
}

func normalizeOpsScriptType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "python", "python3":
		return "python"
	default:
		return "shell"
	}
}

func normalizeOpsInterpreter(v string, scriptType string) string {
	interpreter := strings.ToLower(strings.TrimSpace(v))
	if normalizeOpsScriptType(scriptType) == "python" {
		if slices.Contains([]string{"python", "python3"}, interpreter) {
			return interpreter
		}
		return "python"
	}
	if slices.Contains([]string{"bash", "sh"}, interpreter) {
		return interpreter
	}
	return "bash"
}

func normalizeOpsScriptTimeout(v int) int {
	if v <= 0 {
		return 300
	}
	if v < 30 {
		return 30
	}
	if v > 3600 {
		return 3600
	}
	return v
}

func normalizeOpsScriptVariables(values []model.OpsScriptVariable) ([]model.OpsScriptVariable, error) {
	if len(values) > 30 {
		return nil, errors.New("最多定义 30 个执行变量")
	}
	result := make([]model.OpsScriptVariable, 0, len(values))
	seen := map[string]bool{}
	for _, item := range values {
		item.Name = strings.ToUpper(strings.TrimSpace(item.Name))
		item.Description = Trimmed(item.Description)
		if !opsScriptVariableName.MatchString(item.Name) {
			return nil, fmt.Errorf("变量名 %q 无效；仅支持大写字母、数字和下划线，且必须以字母开头", item.Name)
		}
		if seen[item.Name] {
			return nil, fmt.Errorf("变量 %s 重复", item.Name)
		}
		if len(item.DefaultValue) > 4096 || len(item.Description) > 255 {
			return nil, fmt.Errorf("变量 %s 的默认值或说明过长", item.Name)
		}
		if item.Secret {
			item.DefaultValue = ""
		}
		seen[item.Name] = true
		result = append(result, item)
	}
	return result, nil
}

func resolveOpsScriptVariables(definitions []model.OpsScriptVariable, supplied map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(definitions))
	allowed := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		allowed[definition.Name] = true
		value, exists := supplied[definition.Name]
		if !exists {
			value = definition.DefaultValue
		}
		if definition.Required && strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("请填写执行变量 %s", definition.Name)
		}
		if len(value) > 4096 {
			return nil, fmt.Errorf("执行变量 %s 的值过长", definition.Name)
		}
		resolved[definition.Name] = value
	}
	for name := range supplied {
		if !allowed[name] {
			return nil, fmt.Errorf("变量 %s 未在脚本中定义", name)
		}
	}
	return resolved, nil
}

func normalizeOpsConcurrency(v int) int {
	if v <= 0 {
		return 5
	}
	if v > 10 {
		return 10
	}
	return v
}

func normalizeOpsTimeout(v int) int {
	if v <= 0 {
		return 10
	}
	if v < 10 {
		return 10
	}
	if v > 3600 {
		return 3600
	}
	return v
}

func opsRiskLevel(content string) string {
	value := strings.ToLower(strings.TrimSpace(content))
	highRisk := []string{"rm -rf", "mkfs", "shutdown", "reboot", "init 0", "init 6", "dd if=", "iptables -f", "systemctl stop", "userdel", "drop database", "truncate table"}
	for _, keyword := range highRisk {
		if strings.Contains(value, keyword) {
			return "high"
		}
	}
	return "normal"
}

func requireOpsRiskConfirmation(content string, confirmed bool) (string, error) {
	level := opsRiskLevel(content)
	if level == "high" && !confirmed {
		return level, errors.New("检测到高风险操作，请确认目标范围和命令内容后再次执行")
	}
	return level, nil
}

func opsTaskSource(value string) string {
	if strings.TrimSpace(value) == "" {
		return "quick_exec"
	}
	return strings.TrimSpace(value)
}

func opsTargetSnapshot(hosts []model.AssetHost) string {
	items := make([]map[string]any, 0, len(hosts))
	for _, host := range hosts {
		items = append(items, map[string]any{
			"id": host.ID, "hostName": host.HostName, "sshIp": host.SSHIP,
			"environment": host.Environment,
		})
	}
	data, _ := json.Marshal(items)
	return string(data)
}

func opsHasProductionTarget(hosts []model.AssetHost) bool {
	for _, host := range hosts {
		environment := strings.ToLower(strings.TrimSpace(host.Environment))
		if environment == "prod" || environment == "production" || strings.Contains(environment, "生产") {
			return true
		}
	}
	return false
}

func requireOpsProductionConfirmation(hosts []model.AssetHost, confirmed bool) error {
	if opsHasProductionTarget(hosts) && !confirmed {
		return errors.New("目标包含生产环境主机，请确认目标范围后再次执行")
	}
	return nil
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func (s *Service) ListOpsScripts(pageNum, pageSize int, keyword string, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsScript{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsScript
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

func (s *Service) ListOpsScriptOptions() ([]model.OpsScript, error) {
	var list []model.OpsScript
	if err := s.db.Where("status = ?", 1).Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetOpsScript(id uint) (*model.OpsScript, error) {
	var item model.OpsScript
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) CreateOpsScript(payload OpsScriptPayload) error {
	variables, err := normalizeOpsScriptVariables(payload.Variables)
	if err != nil {
		return err
	}
	item := model.OpsScript{
		Name:           Trimmed(payload.Name),
		ScriptType:     normalizeOpsScriptType(payload.ScriptType),
		Interpreter:    normalizeOpsInterpreter(payload.Interpreter, payload.ScriptType),
		Content:        strings.TrimSpace(payload.Content),
		Variables:      model.OpsScriptVariables(variables),
		TimeoutSeconds: normalizeOpsScriptTimeout(payload.TimeoutSeconds),
		Status:         payload.Status,
		Description:    Trimmed(payload.Description),
		CurrentVersion: 1,
	}
	if item.Name == "" {
		return errors.New("脚本名称不能为空")
	}
	if item.Content == "" {
		return errors.New("脚本内容不能为空")
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return tx.Create(&model.OpsScriptVersion{
			ScriptID: item.ID, Version: 1, Content: item.Content, Variables: item.Variables,
			Interpreter: item.Interpreter, TimeoutSeconds: item.TimeoutSeconds,
			ChangeSummary: "创建脚本", Operator: payload.Operator,
		}).Error
	})
}

func (s *Service) UpdateOpsScript(payload OpsScriptPayload) error {
	existing, err := s.GetOpsScript(payload.ID)
	if err != nil {
		return err
	}
	variables, err := normalizeOpsScriptVariables(payload.Variables)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"name":            Trimmed(payload.Name),
		"script_type":     normalizeOpsScriptType(payload.ScriptType),
		"interpreter":     normalizeOpsInterpreter(payload.Interpreter, payload.ScriptType),
		"content":         strings.TrimSpace(payload.Content),
		"variables":       model.OpsScriptVariables(variables),
		"timeout_seconds": normalizeOpsScriptTimeout(payload.TimeoutSeconds),
		"status":          payload.Status,
		"description":     Trimmed(payload.Description),
	}
	if updates["name"] == "" {
		return errors.New("脚本名称不能为空")
	}
	if updates["content"] == "" {
		return errors.New("脚本内容不能为空")
	}
	if payload.Status == 0 {
		updates["status"] = existing.Status
	}
	nextVersion := existing.CurrentVersion + 1
	if nextVersion < 2 {
		nextVersion = 2
	}
	updates["current_version"] = nextVersion
	return s.db.Transaction(func(tx *gorm.DB) error {
		if existing.CurrentVersion <= 0 {
			if err := tx.Create(&model.OpsScriptVersion{ScriptID: existing.ID, Version: 1, Content: existing.Content, Variables: existing.Variables, Interpreter: existing.Interpreter, TimeoutSeconds: existing.TimeoutSeconds, ChangeSummary: "历史版本归档", Operator: "system"}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.OpsScript{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.OpsScriptVersion{
			ScriptID: payload.ID, Version: nextVersion, Content: updates["content"].(string),
			Variables: model.OpsScriptVariables(variables), Interpreter: updates["interpreter"].(string),
			TimeoutSeconds: updates["timeout_seconds"].(int), ChangeSummary: Trimmed(payload.ChangeSummary), Operator: payload.Operator,
		}).Error
	})
}

func (s *Service) ListOpsScriptVersions(scriptID uint) ([]model.OpsScriptVersion, error) {
	var list []model.OpsScriptVersion
	err := s.db.Where("script_id = ?", scriptID).Order("version DESC").Find(&list).Error
	if err == nil && len(list) == 0 {
		script, loadErr := s.GetOpsScript(scriptID)
		if loadErr != nil {
			return nil, loadErr
		}
		version := script.CurrentVersion
		if version <= 0 {
			version = 1
			_ = s.db.Model(&model.OpsScript{}).Where("id = ?", scriptID).Update("current_version", version).Error
		}
		row := model.OpsScriptVersion{ScriptID: scriptID, Version: version, Content: script.Content, Variables: script.Variables, Interpreter: script.Interpreter, TimeoutSeconds: script.TimeoutSeconds, ChangeSummary: "现有版本归档", Operator: "system"}
		if createErr := s.db.Create(&row).Error; createErr != nil {
			return nil, createErr
		}
		list = []model.OpsScriptVersion{row}
	}
	return list, err
}

func (s *Service) RollbackOpsScript(scriptID uint, version int, operator string) error {
	var target model.OpsScriptVersion
	if err := s.db.Where("script_id = ? AND version = ?", scriptID, version).First(&target).Error; err != nil {
		return err
	}
	existing, err := s.GetOpsScript(scriptID)
	if err != nil {
		return err
	}
	nextVersion := existing.CurrentVersion + 1
	return s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"content": target.Content, "variables": target.Variables, "interpreter": target.Interpreter, "timeout_seconds": target.TimeoutSeconds, "current_version": nextVersion}
		if err := tx.Model(&model.OpsScript{}).Where("id = ?", scriptID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.OpsScriptVersion{ScriptID: scriptID, Version: nextVersion, Content: target.Content, Variables: target.Variables, Interpreter: target.Interpreter, TimeoutSeconds: target.TimeoutSeconds, ChangeSummary: fmt.Sprintf("回滚至 v%d", version), Operator: operator}).Error
	})
}

func (s *Service) DeleteOpsScript(id uint) error {
	return s.db.Delete(&model.OpsScript{}, id).Error
}

func (s *Service) UpdateOpsScriptStatus(payload OpsScriptStatusPayload) error {
	if payload.Status == 0 {
		payload.Status = 2
	}
	return s.db.Model(&model.OpsScript{}).Where("id = ?", payload.ID).Update("status", payload.Status).Error
}

func (s *Service) ExecuteOpsCommand(payload OpsExecCommandPayload) (map[string]any, error) {
	command := strings.TrimSpace(payload.CommandText)
	if command == "" {
		return nil, errors.New("执行命令不能为空")
	}
	riskLevel, err := requireOpsRiskConfirmation(command, payload.RiskConfirmed)
	if err != nil {
		return nil, err
	}
	hosts, err := s.resolveOpsTargetHosts(payload.HostIDs, payload.GroupIDs)
	if err != nil {
		return nil, err
	}
	if err := requireOpsProductionConfirmation(hosts, payload.RiskConfirmed); err != nil {
		return nil, err
	}
	task := model.OpsExecTask{
		TaskType:       "command",
		Title:          opsTaskTitle(payload.Title, "命令执行"),
		CommandText:    command,
		Parameters:     strings.TrimSpace(payload.Parameters),
		Concurrency:    normalizeOpsConcurrency(payload.Concurrency),
		TimeoutSeconds: normalizeOpsTimeout(payload.TimeoutSeconds),
		Status:         "running",
		Summary:        "任务创建成功，正在执行中",
		HostCount:      len(hosts),
		Operator:       payload.Operator,
		SourceIP:       payload.SourceIP,
		Source:         opsTaskSource(payload.Source),
		RiskLevel:      riskLevel,
		TargetSnapshot: opsTargetSnapshot(hosts),
		RetryOfTaskID:  payload.RetryOfTaskID,
	}
	return s.runOpsTaskAsync(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		finalCommand := command
		if strings.TrimSpace(payload.Parameters) != "" {
			finalCommand += " " + strings.TrimSpace(payload.Parameters)
		}
		return s.execCommandOnHost(host, finalCommand, task.TimeoutSeconds)
	})
}

func (s *Service) ExecuteOpsScript(payload OpsExecScriptPayload) (map[string]any, error) {
	if payload.ScriptID == 0 {
		return nil, errors.New("请选择脚本")
	}
	script, err := s.GetOpsScript(payload.ScriptID)
	if err != nil {
		return nil, err
	}
	if script.Status != 1 {
		return nil, errors.New("当前脚本已禁用，无法执行")
	}
	riskLevel, err := requireOpsRiskConfirmation(script.Content, payload.RiskConfirmed)
	if err != nil {
		return nil, err
	}
	hosts, err := s.resolveOpsTargetHosts(payload.HostIDs, payload.GroupIDs)
	if err != nil {
		return nil, err
	}
	if err := requireOpsProductionConfirmation(hosts, payload.RiskConfirmed); err != nil {
		return nil, err
	}
	variables, err := resolveOpsScriptVariables([]model.OpsScriptVariable(script.Variables), payload.Variables)
	if err != nil {
		return nil, err
	}
	timeoutSeconds := payload.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = script.TimeoutSeconds
	}
	task := model.OpsExecTask{
		TaskType:       "script",
		Title:          opsTaskTitle(payload.Title, "脚本执行"),
		ScriptID:       script.ID,
		ScriptName:     script.Name,
		Parameters:     strings.TrimSpace(payload.Parameters),
		Concurrency:    normalizeOpsConcurrency(payload.Concurrency),
		TimeoutSeconds: normalizeOpsTimeout(timeoutSeconds),
		Status:         "running",
		Summary:        "任务创建成功，正在执行中",
		HostCount:      len(hosts),
		Operator:       payload.Operator,
		SourceIP:       payload.SourceIP,
		Source:         opsTaskSource(payload.Source),
		RiskLevel:      riskLevel,
		ScriptVersion:  script.CurrentVersion,
		TargetSnapshot: opsTargetSnapshot(hosts),
		RetryOfTaskID:  payload.RetryOfTaskID,
	}
	return s.runOpsTaskAsync(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		params := strings.TrimSpace(payload.Parameters)
		return s.execScriptOnHost(host, *script, params, variables, task.TimeoutSeconds)
	})
}

func (s *Service) ExecuteOpsFileDispatch(payload OpsFileDispatchPayload, upload *OpsFileDispatchUpload) (map[string]any, error) {
	sourceType := strings.ToLower(strings.TrimSpace(payload.SourceType))
	var fileName string
	switch sourceType {
	case "upload":
		if upload == nil || strings.TrimSpace(upload.TempPath) == "" || strings.TrimSpace(upload.FileName) == "" {
			return nil, errors.New("请上传待分发文件")
		}
		if upload.Size > OpsFileDispatchMaxUploadBytes {
			upload.Cleanup()
			return nil, errors.New("本地上传文件大小不能超过 500 MB")
		}
		fileName = filepath.Base(strings.TrimSpace(upload.FileName))
	case "server":
		if payload.SourceHostID == 0 {
			return nil, errors.New("请选择源服务器")
		}
		if strings.TrimSpace(payload.SourcePath) == "" {
			return nil, errors.New("源文件路径不能为空")
		}
		fileName = path.Base(strings.TrimSpace(payload.SourcePath))
	default:
		return nil, errors.New("不支持的文件来源类型")
	}
	targetPath, err := resolveOpsDispatchTargetPath(payload, fileName)
	if err != nil {
		return nil, err
	}
	riskLevel := "normal"
	if strings.HasPrefix(targetPath, "/etc/") || strings.HasPrefix(targetPath, "/boot/") || strings.HasPrefix(targetPath, "/usr/") {
		riskLevel = "high"
		if !payload.RiskConfirmed {
			return nil, errors.New("目标目录属于系统目录，请确认目标范围后再次执行")
		}
	}
	hosts, err := s.resolveOpsTargetHosts(payload.HostIDs, payload.GroupIDs)
	if err != nil {
		return nil, err
	}
	if err := requireOpsProductionConfirmation(hosts, payload.RiskConfirmed); err != nil {
		return nil, err
	}

	var stagedPath string
	var sourceHostName string
	cleanup := func() {}
	if sourceType == "upload" {
		stagedPath = upload.TempPath
		cleanup = upload.Cleanup
	} else {
		sourceHost, tempPath, stageErr := s.stageRemoteFile(payload.SourceHostID, payload.SourcePath)
		if stageErr != nil {
			return nil, stageErr
		}
		sourceHostName = sourceHost.HostName
		stagedPath = tempPath
		cleanup = func() { _ = os.Remove(tempPath) }
	}

	task := model.OpsExecTask{
		TaskType:       "file",
		Title:          opsTaskTitle(payload.Title, "文件分发"),
		SourceType:     sourceType,
		SourceHostID:   payload.SourceHostID,
		SourceHostName: sourceHostName,
		SourcePath:     strings.TrimSpace(payload.SourcePath),
		TargetPath:     targetPath,
		FileName:       fileName,
		Concurrency:    normalizeOpsConcurrency(payload.Concurrency),
		TimeoutSeconds: normalizeOpsTimeout(payload.TimeoutSeconds),
		Status:         "running",
		Summary:        "任务创建成功，正在执行中",
		HostCount:      len(hosts),
		Operator:       payload.Operator,
		SourceIP:       payload.SourceIP,
		Source:         opsTaskSource(payload.Source),
		RiskLevel:      riskLevel,
		TargetSnapshot: opsTargetSnapshot(hosts),
	}
	result, err := s.runOpsTaskAsyncWithFinalizer(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
		return s.dispatchFileFromPath(host, fileName, targetPath, stagedPath, payload.Overwrite, task.TimeoutSeconds)
	}, cleanup)
	if err != nil {
		cleanup()
	}
	return result, err
}

func resolveOpsDispatchTargetPath(payload OpsFileDispatchPayload, sourceFileName string) (string, error) {
	targetDir := strings.TrimSpace(payload.TargetDir)
	if targetDir == "" {
		legacyPath := strings.TrimSpace(payload.TargetPath)
		if legacyPath == "" {
			return "", errors.New("目标目录不能为空")
		}
		return legacyPath, nil
	}
	fileName := strings.TrimSpace(payload.TargetFileName)
	if fileName == "" {
		fileName = sourceFileName
	}
	if fileName == "" || fileName == "." || fileName == ".." || strings.ContainsAny(fileName, "/\\") {
		return "", errors.New("目标文件名不合法")
	}
	return path.Join(targetDir, fileName), nil
}

func (s *Service) ListOpsExecTasks(pageNum, pageSize int, keyword, taskType, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsExecTask{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("title LIKE ? OR script_name LIKE ? OR file_name LIKE ? OR command_text LIKE ?", like, like, like, like)
	}
	if taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsExecTask
	if err := query.Select("id", "task_type", "title", "script_name", "file_name", "host_count", "success_count", "failed_count", "status", "summary", "operator", "risk_level", "started_at", "created_at").Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetOpsExecTaskDetail(id uint) (map[string]any, error) {
	var task model.OpsExecTask
	if err := s.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	var results []model.OpsExecTargetResult
	if err := s.db.Where("task_id = ?", id).Order("id ASC").Find(&results).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"task":    task,
		"results": results,
	}, nil
}

func (s *Service) RetryFailedOpsExecTask(payload OpsExecRetryPayload) (map[string]any, error) {
	var task model.OpsExecTask
	if err := s.db.First(&task, payload.TaskID).Error; err != nil {
		return nil, err
	}
	if task.Status == "running" {
		return nil, errors.New("任务仍在执行中，暂不能重试")
	}
	if task.TaskType == "file" {
		return nil, errors.New("文件内容不会长期保存在平台，请复制原文件分发任务后重新上传")
	}
	var failedResults []model.OpsExecTargetResult
	if err := s.db.Where("task_id = ? AND status <> ?", task.ID, "success").Find(&failedResults).Error; err != nil {
		return nil, err
	}
	if len(failedResults) == 0 {
		return nil, errors.New("当前任务没有失败或超时主机")
	}
	hostIDs := make([]uint, 0, len(failedResults))
	for _, result := range failedResults {
		hostIDs = append(hostIDs, result.HostID)
	}
	if task.TaskType == "command" {
		return s.ExecuteOpsCommand(OpsExecCommandPayload{
			Title: task.Title + "（失败重试）", CommandText: task.CommandText, Parameters: task.Parameters,
			HostIDs: hostIDs, Concurrency: task.Concurrency, TimeoutSeconds: task.TimeoutSeconds,
			RiskConfirmed: true, Operator: payload.Operator, SourceIP: payload.SourceIP, Source: "retry", RetryOfTaskID: task.ID,
		})
	}
	return s.ExecuteOpsScript(OpsExecScriptPayload{
		Title: task.Title + "（失败重试）", ScriptID: task.ScriptID, Parameters: task.Parameters,
		HostIDs: hostIDs, Concurrency: task.Concurrency, TimeoutSeconds: task.TimeoutSeconds,
		RiskConfirmed: true, Operator: payload.Operator, SourceIP: payload.SourceIP, Source: "retry", RetryOfTaskID: task.ID,
	})
}

func opsTaskTitle(title string, fallback string) string {
	value := strings.TrimSpace(title)
	if value != "" {
		return value
	}
	return fmt.Sprintf("%s %s", fallback, time.Now().Format("2006-01-02 15:04:05"))
}

func (s *Service) resolveOpsTargetHosts(hostIDs []uint, groupIDs []uint) ([]model.AssetHost, error) {
	if len(hostIDs) > 0 && len(groupIDs) > 0 {
		return nil, errors.New("目标主机和主机组只能二选一")
	}
	hostMap := map[uint]model.AssetHost{}

	if len(hostIDs) > 0 {
		var hosts []model.AssetHost
		if err := s.db.Preload("Group").Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").Where("id IN ?", hostIDs).Find(&hosts).Error; err != nil {
			return nil, err
		}
		for _, host := range hosts {
			hostMap[host.ID] = host
		}
	}

	if len(groupIDs) > 0 {
		var relationIDs []uint
		if err := s.db.Model(&model.AssetHostGroupRelation{}).Where("group_id IN ?", groupIDs).Distinct().Pluck("host_id", &relationIDs).Error; err != nil {
			return nil, err
		}
		if len(relationIDs) > 0 {
			var hosts []model.AssetHost
			if err := s.db.Preload("Group").Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").Where("id IN ?", relationIDs).Find(&hosts).Error; err != nil {
				return nil, err
			}
			for _, host := range hosts {
				hostMap[host.ID] = host
			}
		}
	}

	if len(hostMap) == 0 {
		return nil, errors.New("请至少选择一台主机或一个主机组")
	}

	hosts := make([]model.AssetHost, 0, len(hostMap))
	for _, host := range hostMap {
		hosts = append(hosts, host)
	}
	slices.SortFunc(hosts, func(a, b model.AssetHost) int {
		return int(a.ID) - int(b.ID)
	})
	return hosts, nil
}

func (s *Service) runOpsTaskLegacy(task model.OpsExecTask, hosts []model.AssetHost, runner func(host model.AssetHost) model.OpsExecTargetResult) (map[string]any, error) {
	now := time.Now()
	task.StartedAt = &now
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}

	semaphore := make(chan struct{}, task.Concurrency)
	results := make([]model.OpsExecTargetResult, len(hosts))
	var wg sync.WaitGroup

	for index, host := range hosts {
		wg.Add(1)
		go func(i int, item model.AssetHost) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			result := runner(item)
			result.TaskID = task.ID
			results[i] = result
			_ = s.db.Create(&result).Error
		}(index, host)
	}
	wg.Wait()

	successCount := 0
	failedCount := 0
	for _, result := range results {
		if result.Status == "success" {
			successCount++
		} else {
			failedCount++
		}
	}
	finishedAt := time.Now()
	status := "success"
	switch {
	case successCount == 0:
		status = "failed"
	case failedCount > 0:
		status = "partial"
	}
	summary := fmt.Sprintf("成功 %d 台，失败 %d 台", successCount, failedCount)
	_ = s.db.Model(&model.OpsExecTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"success_count": successCount,
		"failed_count":  failedCount,
		"status":        status,
		"summary":       summary,
		"finished_at":   &finishedAt,
	}).Error

	return s.GetOpsExecTaskDetail(task.ID)
}

func (s *Service) runOpsTaskAsync(task model.OpsExecTask, hosts []model.AssetHost, runner func(host model.AssetHost) model.OpsExecTargetResult) (map[string]any, error) {
	return s.runOpsTaskAsyncWithFinalizer(task, hosts, runner, nil)
}

func (s *Service) runOpsTaskAsyncWithFinalizer(task model.OpsExecTask, hosts []model.AssetHost, runner func(host model.AssetHost) model.OpsExecTargetResult, finalizer func()) (map[string]any, error) {
	now := time.Now()
	task.StartedAt = &now
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	pendingRows := make([]model.OpsExecTargetResult, 0, len(hosts))
	for _, host := range hosts {
		pendingRows = append(pendingRows, model.OpsExecTargetResult{
			TaskID:    task.ID,
			HostID:    host.ID,
			HostName:  host.HostName,
			GroupName: host.Group.Name,
			SSHIP:     host.SSHIP,
			Status:    "running",
		})
	}
	if len(pendingRows) > 0 {
		_ = s.db.Create(&pendingRows).Error
	}
	go func() {
		s.processOpsTask(task, hosts, runner)
		if finalizer != nil {
			finalizer()
		}
	}()
	return s.GetOpsExecTaskDetail(task.ID)
}

func (s *Service) processOpsTask(task model.OpsExecTask, hosts []model.AssetHost, runner func(host model.AssetHost) model.OpsExecTargetResult) {
	semaphore := make(chan struct{}, task.Concurrency)
	var wg sync.WaitGroup

	for _, host := range hosts {
		wg.Add(1)
		go func(item model.AssetHost) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			result := runner(item)
			_ = s.db.Model(&model.OpsExecTargetResult{}).
				Where("task_id = ? AND host_id = ?", task.ID, item.ID).
				Updates(map[string]any{
					"status":      result.Status,
					"exit_code":   result.ExitCode,
					"duration_ms": result.DurationMs,
					"stdout":      result.Stdout,
					"stderr":      result.Stderr,
					"error_text":  result.ErrorText,
				}).Error
			s.refreshOpsTaskProgress(task.ID)
		}(host)
	}
	wg.Wait()
	s.finishOpsTask(task.ID)
}

func (s *Service) refreshOpsTaskProgress(taskID uint) {
	var successCount int64
	var failedCount int64
	var runningCount int64
	_ = s.db.Model(&model.OpsExecTargetResult{}).Where("task_id = ? AND status = ?", taskID, "success").Count(&successCount).Error
	_ = s.db.Model(&model.OpsExecTargetResult{}).Where("task_id = ? AND status = ?", taskID, "running").Count(&runningCount).Error
	_ = s.db.Model(&model.OpsExecTargetResult{}).Where("task_id = ? AND status NOT IN ?", taskID, []string{"success", "running"}).Count(&failedCount).Error
	completedCount := successCount + failedCount
	summary := fmt.Sprintf("已完成 %d 台，成功 %d 台，失败 %d 台，执行中 %d 台", completedCount, successCount, failedCount, runningCount)
	_ = s.db.Model(&model.OpsExecTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"success_count": int(successCount),
		"failed_count":  int(failedCount),
		"summary":       summary,
	}).Error
}

func (s *Service) finishOpsTask(taskID uint) {
	var successCount int64
	var failedCount int64
	_ = s.db.Model(&model.OpsExecTargetResult{}).Where("task_id = ? AND status = ?", taskID, "success").Count(&successCount).Error
	_ = s.db.Model(&model.OpsExecTargetResult{}).Where("task_id = ? AND status NOT IN ?", taskID, []string{"success", "running"}).Count(&failedCount).Error
	finishedAt := time.Now()
	status := "success"
	switch {
	case successCount == 0:
		status = "failed"
	case failedCount > 0:
		status = "partial"
	}
	summary := fmt.Sprintf("执行完成，成功 %d 台，失败 %d 台", successCount, failedCount)
	_ = s.db.Model(&model.OpsExecTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"success_count": int(successCount),
		"failed_count":  int(failedCount),
		"status":        status,
		"summary":       summary,
		"finished_at":   &finishedAt,
	}).Error
}

func (s *Service) execCommandOnHost(host model.AssetHost, command string, timeoutSeconds int) model.OpsExecTargetResult {
	return s.execCommandOnHostStreaming(host, command, timeoutSeconds, nil)
}

// execCommandOnHostStreaming reports remote stdout/stderr as soon as SSH receives it.
// Callers that do not need incremental output should keep using execCommandOnHost.
func (s *Service) execCommandOnHostStreaming(host model.AssetHost, command string, timeoutSeconds int, onOutput func(string)) model.OpsExecTargetResult {
	started := time.Now()
	result := model.OpsExecTargetResult{
		HostID:    host.ID,
		HostName:  host.HostName,
		GroupName: host.Group.Name,
		SSHIP:     host.SSHIP,
		Status:    "failed",
	}
	client, err := s.newSSHClient(host)
	if err != nil {
		result.ErrorText = err.Error()
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	defer client.Close()

	stdout, stderr, exitCode, runErr := runSSHCommandDetailedStreaming(client, "bash -lc "+shellQuote(command), timeoutSeconds, onOutput)
	result.Stdout = stdout
	result.Stderr = stderr
	result.ExitCode = exitCode
	result.DurationMs = time.Since(started).Milliseconds()
	if runErr == nil {
		result.Status = "success"
	} else {
		result.ErrorText = runErr.Error()
	}
	return result
}

func (s *Service) execScriptOnHost(host model.AssetHost, script model.OpsScript, params string, variables map[string]string, timeoutSeconds int) model.OpsExecTargetResult {
	encoded := base64.StdEncoding.EncodeToString([]byte(script.Content))
	interpreter := normalizeOpsInterpreter(script.Interpreter, script.ScriptType)
	command := fmt.Sprintf(
		"tmp=$(mktemp /tmp/ops-script-XXXXXX) && base64 -d <<'__OPS_SCRIPT__' > \"$tmp\"\n%s\n__OPS_SCRIPT__\nchmod +x \"$tmp\"\n%s\n%s \"$tmp\" %s\nstatus=$?\nrm -f \"$tmp\"\nexit $status",
		encoded,
		opsScriptVariableExports(variables),
		interpreter,
		strings.TrimSpace(params),
	)
	return s.execCommandOnHost(host, command, timeoutSeconds)
}

func opsScriptVariableExports(variables map[string]string) string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		encoded := base64.StdEncoding.EncodeToString([]byte(variables[name]))
		lines = append(lines, fmt.Sprintf("export VARIABLE_%s=$(printf %%s %s | base64 -d)", name, shellQuote(encoded)))
	}
	return strings.Join(lines, "\n")
}

func (s *Service) dispatchFileFromPath(host model.AssetHost, fileName, targetPath, sourcePath string, overwrite bool, timeoutSeconds int) model.OpsExecTargetResult {
	reader, err := os.Open(sourcePath)
	if err != nil {
		return failedOpsFileResult(host, fmt.Errorf("打开待分发文件失败: %w", err))
	}
	defer reader.Close()
	return s.dispatchFileReaderToHost(host, fileName, targetPath, reader, overwrite, timeoutSeconds)
}

func failedOpsFileResult(host model.AssetHost, err error) model.OpsExecTargetResult {
	return model.OpsExecTargetResult{
		HostID: host.ID, HostName: host.HostName, GroupName: host.Group.Name, SSHIP: host.SSHIP,
		Status: "failed", ExitCode: -1, ErrorText: err.Error(),
	}
}

// dispatchFileReaderToHost streams raw file bytes through the SSH channel's
// standard input. Large files must never be encoded into an SSH exec command.
func (s *Service) dispatchFileReaderToHost(host model.AssetHost, fileName, targetPath string, reader io.Reader, overwrite bool, timeoutSeconds int) model.OpsExecTargetResult {
	started := time.Now()
	result := model.OpsExecTargetResult{
		HostID:    host.ID,
		HostName:  host.HostName,
		GroupName: host.Group.Name,
		SSHIP:     host.SSHIP,
		Status:    "failed",
	}
	client, err := s.newSSHClient(host)
	if err != nil {
		result.ErrorText = fmt.Sprintf("SSH 连接失败: %v", err)
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	defer client.Close()

	target := strings.TrimSpace(targetPath)
	dir := path.Dir(target)
	existsCheck := ""
	if !overwrite {
		existsCheck = fmt.Sprintf("if [ -e %s ]; then echo 'target file already exists'; exit 17; fi\n", shellQuote(target))
	}
	command := fmt.Sprintf(
		"set -e\nmkdir -p %s\n%stmp=$(mktemp %s)\ncleanup() { rm -f \"$tmp\"; }\ntrap cleanup EXIT\ncat > \"$tmp\"\nmv -f \"$tmp\" %s\ntrap - EXIT\n",
		shellQuote(dir),
		existsCheck,
		shellQuote(path.Join(dir, fmt.Sprintf(".%s.ops-XXXXXX", fileName))),
		shellQuote(target),
	)
	session, err := client.NewSession()
	if err != nil {
		result.ErrorText = fmt.Sprintf("SSH 会话创建失败: %v", err)
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	defer session.Close()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	stdin, err := session.StdinPipe()
	if err != nil {
		result.ErrorText = fmt.Sprintf("文件传输通道创建失败: %v", err)
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	if err := session.Start("bash -lc " + shellQuote(command)); err != nil {
		result.ErrorText = fmt.Sprintf("远端文件写入命令启动失败: %v", err)
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	type transferOutcome struct{ copyErr, closeErr, waitErr error }
	done := make(chan transferOutcome, 1)
	go func() {
		_, copyErr := io.Copy(stdin, reader)
		closeErr := stdin.Close()
		waitErr := session.Wait()
		done <- transferOutcome{copyErr: copyErr, closeErr: closeErr, waitErr: waitErr}
	}()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	var outcome transferOutcome
	select {
	case outcome = <-done:
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		result.ExitCode = 124
		result.ErrorText = fmt.Sprintf("文件传输超时，已在 %d 秒后终止", timeoutSeconds)
		result.DurationMs = time.Since(started).Milliseconds()
		return result
	}
	stdoutText, stderrText := stdout.String(), stderr.String()
	result.Stdout = stdoutText
	result.Stderr = stderrText
	result.DurationMs = time.Since(started).Milliseconds()
	// The target can reject the transfer before it consumes stdin (for example,
	// when overwrite is disabled and the destination already exists). In that
	// case the local writer sees EOF, but the target-side validation is the
	// actionable error and must not be hidden by that secondary pipe error.
	if strings.Contains(stdoutText, "target file already exists") {
		result.ExitCode = 17
		result.ErrorText = "目标文件已存在，未开启“覆盖已有”"
		return result
	}
	runErr := outcome.waitErr
	if runErr == nil && outcome.copyErr != nil {
		runErr = fmt.Errorf("向目标主机写入文件失败: %w", outcome.copyErr)
	}
	if runErr == nil && outcome.closeErr != nil {
		runErr = fmt.Errorf("关闭文件传输通道失败: %w", outcome.closeErr)
	}
	if runErr == nil {
		result.ExitCode = 0
		result.Status = "success"
		result.Stdout = strings.TrimSpace(strings.Join([]string{stdoutText, fmt.Sprintf("distributed to %s", target), fileName}, "\n"))
	} else {
		var exitErr *ssh.ExitError
		if errors.As(outcome.waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.ExitCode = -1
		}
		result.ErrorText = strings.TrimSpace(strings.Join([]string{runErr.Error(), stderrText}, "\n"))
	}
	return result
}

func (s *Service) readRemoteFile(hostID uint, sourcePath string) (*model.AssetHost, []byte, error) {
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, hostID).Error; err != nil {
		return nil, nil, err
	}
	client, err := s.newSSHClient(host)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close()

	command := fmt.Sprintf("if [ ! -f %s ]; then echo '__OPS_FILE_NOT_FOUND__'; exit 22; fi\nbase64 < %s", shellQuote(sourcePath), shellQuote(sourcePath))
	stdout, stderr, _, runErr := runSSHCommandDetailed(client, "bash -lc "+shellQuote(command), 30)
	if runErr != nil {
		return nil, nil, errors.New(strings.TrimSpace(strings.Join([]string{stderr, runErr.Error()}, "\n")))
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout))
	if err != nil {
		return nil, nil, err
	}
	return &host, decoded, nil
}

// stageRemoteFile streams a server-side source file to the platform's temporary
// storage once, so a one-to-many dispatch never retains the whole payload in
// memory or re-reads it once per target.
func (s *Service) stageRemoteFile(hostID uint, sourcePath string) (*model.AssetHost, string, error) {
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, hostID).Error; err != nil {
		return nil, "", err
	}
	client, err := s.newSSHClient(host)
	if err != nil {
		return nil, "", fmt.Errorf("源服务器 SSH 连接失败: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return nil, "", fmt.Errorf("源服务器 SSH 会话创建失败: %w", err)
	}
	defer session.Close()
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("源文件读取通道创建失败: %w", err)
	}
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err := session.Start("cat -- " + shellQuote(strings.TrimSpace(sourcePath))); err != nil {
		return nil, "", fmt.Errorf("源文件读取命令启动失败: %w", err)
	}
	file, tempPath, err := createOpsFileDispatchTempFile()
	if err != nil {
		return nil, "", err
	}
	_, copyErr := io.Copy(file, stdout)
	closeErr := file.Close()
	waitErr := session.Wait()
	if copyErr != nil || closeErr != nil || waitErr != nil {
		_ = os.Remove(tempPath)
		message := strings.TrimSpace(strings.Join([]string{stderr.String(), errorText(copyErr), errorText(closeErr), errorText(waitErr)}, "\n"))
		if message == "" {
			message = "读取源服务器文件失败"
		}
		return nil, "", errors.New(message)
	}
	return &host, tempPath, nil
}

func createOpsFileDispatchTempFile() (*os.File, string, error) {
	directory := filepath.Join("uploads", "temp", "file-dispatch")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, "", fmt.Errorf("创建文件分发临时目录失败: %w", err)
	}
	file, err := os.CreateTemp(directory, "dispatch-*")
	if err != nil {
		return nil, "", fmt.Errorf("创建文件分发临时文件失败: %w", err)
	}
	return file, file.Name(), nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runSSHCommandDetailed(client *ssh.Client, command string, timeoutSeconds int) (string, string, int, error) {
	return runSSHCommandDetailedStreaming(client, command, timeoutSeconds, nil)
}

func runSSHCommandDetailedStreaming(client *ssh.Client, command string, timeoutSeconds int, onOutput func(string)) (string, string, int, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", -1, err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var outputMu sync.Mutex
	newWriter := func(buffer *bytes.Buffer) io.Writer {
		return writerFunc(func(chunk []byte) (int, error) {
			outputMu.Lock()
			_, _ = buffer.Write(chunk)
			outputMu.Unlock()
			if onOutput != nil && len(chunk) > 0 {
				onOutput(string(chunk))
			}
			return len(chunk), nil
		})
	}
	session.Stdout = newWriter(&stdout)
	session.Stderr = newWriter(&stderr)

	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	if err := session.Start(command); err != nil {
		return "", "", -1, err
	}
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err = <-done:
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		err = fmt.Errorf("执行超时，已在 %d 秒后终止", timeoutSeconds)
	}
	exitCode := 0
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitStatus()
		} else if strings.Contains(err.Error(), "执行超时") {
			exitCode = 124
		} else {
			exitCode = -1
		}
	}
	outputMu.Lock()
	stdoutText, stderrText := stdout.String(), stderr.String()
	outputMu.Unlock()
	return stdoutText, stderrText, exitCode, err
}

type writerFunc func([]byte) (int, error)

func (fn writerFunc) Write(chunk []byte) (int, error) { return fn(chunk) }
