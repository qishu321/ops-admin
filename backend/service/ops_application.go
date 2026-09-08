package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"ops-admin/backend/model"
)

type OpsApplicationPayload struct {
	ID               uint                                      `json:"id"`
	Name             string                                    `json:"name"`
	Code             string                                    `json:"code"`
	ServiceType      string                                    `json:"serviceType"`
	RepoType         string                                    `json:"repoType"`
	RepoURL          string                                    `json:"repoUrl"`
	RepoCredentialID uint                                      `json:"repoCredentialId"`
	Branch           string                                    `json:"branch"`
	Workspace        string                                    `json:"workspace"`
	BuildScript      string                                    `json:"buildScript"`
	DeployScript     string                                    `json:"deployScript"`
	Env              string                                    `json:"env"`
	Status           int                                       `json:"status"`
	Description      string                                    `json:"description"`
	Bindings         []OpsApplicationEnvironmentBindingPayload `json:"bindings"`
}

type OpsApplicationEnvironmentBindingPayload struct {
	Env                 string `json:"env"`
	HostGroupID         uint   `json:"hostGroupId"`
	K8sClusterID        uint   `json:"k8sClusterId"`
	Namespace           string `json:"namespace"`
	WorkloadType        string `json:"workloadType"`
	WorkloadName        string `json:"workloadName"`
	DatabaseID          uint   `json:"databaseId"`
	MonitorDatasourceID uint   `json:"monitorDatasourceId"`
	GatewayID           uint   `json:"gatewayId"`
}

type OpsAppBuildTaskPayload struct {
	ID             uint                             `json:"id"`
	AppID          uint                             `json:"appId"`
	Name           string                           `json:"name"`
	Env            string                           `json:"env"`
	Branch         string                           `json:"branch"`
	BuildScript    string                           `json:"buildScript"`
	DeployScript   string                           `json:"deployScript"`
	BuildParams    []OpsAppBuildParameterDefinition `json:"buildParams"`
	RunnerType     string                           `json:"runnerType"`
	RunnerHostID   uint                             `json:"runnerHostId"`
	ExecutionPath  string                           `json:"executionPath"`
	ArtifactType   string                           `json:"artifactType"`
	ArtifactPath   string                           `json:"artifactPath"`
	TimeoutSeconds int                              `json:"timeoutSeconds"`
	Status         int                              `json:"status"`
	Description    string                           `json:"description"`
}

type OpsAppBuildTaskStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type OpsAppBuildRunPayload struct {
	TaskID  uint           `json:"taskId"`
	Version string         `json:"version"`
	Branch  string         `json:"branch"`
	Params  map[string]any `json:"params"`
}

type OpsAppBuildParameterDefinition struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Default     any      `json:"default"`
	Options     []string `json:"options"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
}

type OpsAppReleasePayload struct {
	AppID   uint   `json:"appId"`
	Version string `json:"version"`
	Branch  string `json:"branch"`
}

type OpsAppPipelinePayload struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	AppID          uint   `json:"appId"`
	DefaultBranch  string `json:"defaultBranch"`
	Env            string `json:"env"`
	TechStack      string `json:"techStack"`
	TemplateID     uint   `json:"templateId"`
	BuildTaskID    uint   `json:"buildTaskId"`
	ExecutorHostID uint   `json:"executorHostId"`
	Status         int    `json:"status"`
	Description    string `json:"description"`
	DefinitionJSON string `json:"definitionJson"`
}

type OpsAppPipelineTemplatePayload struct {
	Name           string `json:"name"`
	Category       string `json:"category"`
	TechStack      string `json:"techStack"`
	Description    string `json:"description"`
	DefinitionJSON string `json:"definitionJson"`
}

type OpsAppPipelineStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type OpsAppPipelineRunPayload struct {
	PipelineID uint              `json:"pipelineId"`
	Branch     string            `json:"branch"`
	Env        string            `json:"env"`
	ImageTag   string            `json:"imageTag"`
	ArtifactID uint              `json:"artifactId"`
	Params     map[string]string `json:"params"`
}

type OpsImageRegistryPayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Namespace   string `json:"namespace"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type OpsAppPipelineApprovalPayload struct {
	RunID    uint   `json:"runId"`
	Decision string `json:"decision"`
	Note     string `json:"note"`
	Operator string `json:"operator"`
}

type OpsAppPipelineStageDefinition struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	FailurePolicy  string            `json:"failurePolicy"`
	Config         map[string]any    `json:"config"`
	Env            map[string]string `json:"env"`
}

type opsBuildExecution struct {
	ReleaseID      uint
	App            model.OpsApplication
	TaskID         uint
	TaskName       string
	Env            string
	BuildScript    string
	DeployScript   string
	TimeoutSeconds int
	Branch         string
	Workspace      string
	RunnerType     string
	RunnerHostID   uint
	ArtifactType   string
	ArtifactPath   string
	Version        string
	Params         map[string]string
}

// opsBuildLogWriter keeps the persisted log usable while a build is still
// running. The database write is throttled so noisy build tools do not cause a
// write per output chunk.
type opsBuildLogWriter struct {
	service     *Service
	releaseID   uint
	column      string
	mu          sync.Mutex
	content     strings.Builder
	lastPersist time.Time
}

func newOpsBuildLogWriter(service *Service, releaseID uint, column string) *opsBuildLogWriter {
	return &opsBuildLogWriter{service: service, releaseID: releaseID, column: column}
}

func (writer *opsBuildLogWriter) Append(text string) {
	if text == "" {
		return
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.content.WriteString(text)
	if time.Since(writer.lastPersist) >= 500*time.Millisecond {
		writer.persistLocked()
	}
}

func (writer *opsBuildLogWriter) Flush() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.persistLocked()
}

func (writer *opsBuildLogWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.content.String()
}

func (writer *opsBuildLogWriter) persistLocked() {
	_ = writer.service.db.Model(&model.OpsAppRelease{}).Where("id = ?", writer.releaseID).Update(writer.column, writer.content.String()).Error
	writer.lastPersist = time.Now()
}

type opsPipelineExecution struct {
	RunID        uint
	PipelineID   uint
	App          model.OpsApplication
	Branch       string
	Env          string
	ImageTag     string
	Workspace    string
	ExecutorHost model.AssetHost
	Params       map[string]string
	Stages       []OpsAppPipelineStageDefinition
	StartedAt    time.Time
}

func normalizeOpsRepoType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "svn":
		return "svn"
	default:
		return "git"
	}
}

func normalizeOpsSVNRevision(value string) (string, error) {
	revision := strings.ToUpper(strings.TrimSpace(value))
	if revision == "" || revision == "HEAD" {
		return "HEAD", nil
	}
	for _, char := range revision {
		if char < '0' || char > '9' {
			return "", errors.New("SVN 版本号仅支持 HEAD 或数字修订号")
		}
	}
	return revision, nil
}

func normalizeOpsAppStatus(value int) int {
	if value == 2 {
		return 2
	}
	return 1
}

func normalizeOpsBuildTimeout(value int) int {
	if value < 60 {
		return 60
	}
	if value > 7200 {
		return 7200
	}
	return value
}

var opsAppBuildParamNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func normalizeOpsAppBuildParameters(input []OpsAppBuildParameterDefinition) ([]OpsAppBuildParameterDefinition, string, error) {
	result := make([]OpsAppBuildParameterDefinition, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item.Name = strings.ToUpper(strings.TrimSpace(item.Name))
		item.Label = strings.TrimSpace(item.Label)
		item.Description = strings.TrimSpace(item.Description)
		if item.Name == "" {
			continue
		}
		if !opsAppBuildParamNamePattern.MatchString(item.Name) {
			return nil, "", fmt.Errorf("构建参数 %s 格式不正确，只能使用大写字母、数字和下划线", item.Name)
		}
		if _, exists := seen[item.Name]; exists {
			return nil, "", fmt.Errorf("构建参数 %s 重复", item.Name)
		}
		seen[item.Name] = struct{}{}
		switch item.Type {
		case "select", "multiSelect", "boolean":
		default:
			item.Type = "text"
		}
		if item.Label == "" {
			item.Label = item.Name
		}
		options := make([]string, 0, len(item.Options))
		for _, option := range item.Options {
			if value := strings.TrimSpace(option); value != "" {
				options = append(options, value)
			}
		}
		item.Options = options
		result = append(result, item)
	}
	encoded, err := json.Marshal(result)
	return result, string(encoded), err
}

func normalizeOpsAppBuildParametersJSON(raw string) ([]OpsAppBuildParameterDefinition, string, error) {
	if strings.TrimSpace(raw) == "" {
		return []OpsAppBuildParameterDefinition{}, "[]", nil
	}
	var input []OpsAppBuildParameterDefinition
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, "", errors.New("构建参数配置格式不正确")
	}
	return normalizeOpsAppBuildParameters(input)
}

func resolveOpsAppBuildParams(definitions []OpsAppBuildParameterDefinition, values map[string]any) (map[string]string, error) {
	result := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		value, exists := values[definition.Name]
		if !exists || value == nil {
			value = definition.Default
		}
		text := opsAppBuildParamValue(value)
		if definition.Required && strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("请填写构建参数 %s", definition.Label)
		}
		if definition.Type == "select" && text != "" && len(definition.Options) > 0 && !containsString(definition.Options, text) {
			return nil, fmt.Errorf("构建参数 %s 的值不在可选范围内", definition.Label)
		}
		result[definition.Name] = text
	}
	return result, nil
}

func opsAppBuildParamValue(value any) string {
	switch typed := value.(type) {
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(typed, ",")
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Service) ListOpsApplications(pageNum, pageSize int, keyword, repoType, status, serviceType, env string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsApplication{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR code LIKE ? OR repo_url LIKE ? OR description LIKE ?", like, like, like, like)
	}
	if repoType = strings.ToLower(strings.TrimSpace(repoType)); repoType != "" {
		query = query.Where("repo_type = ?", normalizeOpsRepoType(repoType))
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if serviceType = strings.TrimSpace(serviceType); serviceType != "" {
		query = query.Where("service_type = ?", serviceType)
	}
	if env = normalizeEnvCode(env); env != "" {
		bindingAppIDs := s.db.Model(&model.OpsApplicationEnvironmentBinding{}).Select("app_id").Where("env = ? AND status = ?", env, 1)
		query = query.Where("env = ? OR id IN (?)", env, bindingAppIDs)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsApplication
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListOpsApplicationOptions() ([]map[string]any, error) {
	var list []model.OpsApplication
	if err := s.db.Where("status = ?", 1).Order("name ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	options := make([]map[string]any, 0, len(list))
	for _, item := range list {
		options = append(options, map[string]any{
			"id": item.ID, "name": item.Name, "code": item.Code, "repoType": item.RepoType,
			"repoUrl": item.RepoURL, "branch": item.Branch, "workspace": item.Workspace, "env": item.Env, "serviceType": item.ServiceType,
			"repoCredentialId": item.RepoCredentialID,
		})
	}
	return options, nil
}

func (s *Service) GetOpsApplication(id uint) (*model.OpsApplication, error) {
	var item model.OpsApplication
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) SaveOpsApplication(payload OpsApplicationPayload) error {
	item := model.OpsApplication{
		Name:             Trimmed(payload.Name),
		Code:             Trimmed(payload.Code),
		ServiceType:      Trimmed(payload.ServiceType),
		RepoType:         normalizeOpsRepoType(payload.RepoType),
		RepoURL:          Trimmed(payload.RepoURL),
		RepoCredentialID: payload.RepoCredentialID,
		Branch:           Trimmed(payload.Branch),
		BuildScript:      strings.TrimSpace(payload.BuildScript),
		DeployScript:     strings.TrimSpace(payload.DeployScript),
		Env:              Trimmed(payload.Env),
		Status:           normalizeOpsAppStatus(payload.Status),
		Description:      Trimmed(payload.Description),
	}
	if item.Name == "" {
		return errors.New("应用名称不能为空")
	}
	if item.Code == "" {
		return errors.New("应用编码不能为空")
	}
	if item.RepoURL == "" {
		return errors.New("仓库地址不能为空")
	}
	if item.ServiceType == "" {
		item.ServiceType = "后端服务"
	}
	if item.RepoType == "svn" {
		var err error
		item.Branch, err = normalizeOpsSVNRevision(item.Branch)
		if err != nil {
			return err
		}
	} else if item.Branch == "" {
		item.Branch = "master"
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		appID := payload.ID
		if appID == 0 {
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			appID = item.ID
		} else if err := tx.Model(&model.OpsApplication{}).Where("id = ?", appID).Updates(map[string]any{
			"name": item.Name, "code": item.Code, "service_type": item.ServiceType, "repo_type": item.RepoType,
			"repo_url": item.RepoURL, "repo_credential_id": item.RepoCredentialID, "branch": item.Branch,
			"build_script": item.BuildScript, "deploy_script": item.DeployScript, "env": item.Env,
			"status": item.Status, "description": item.Description,
		}).Error; err != nil {
			return err
		}
		if payload.Bindings == nil {
			return nil
		}
		if err := tx.Where("app_id = ?", appID).Delete(&model.OpsApplicationEnvironmentBinding{}).Error; err != nil {
			return err
		}
		seen := map[string]struct{}{}
		for _, binding := range payload.Bindings {
			env := normalizeEnvCode(binding.Env)
			if env == "" {
				continue
			}
			if _, ok := seen[env]; ok {
				return fmt.Errorf("环境 %s 只能绑定一次", env)
			}
			seen[env] = struct{}{}
			row := model.OpsApplicationEnvironmentBinding{
				AppID: appID, Env: env, HostGroupID: binding.HostGroupID, K8sClusterID: binding.K8sClusterID,
				Namespace: Trimmed(binding.Namespace), WorkloadType: strings.ToLower(Trimmed(binding.WorkloadType)),
				WorkloadName: Trimmed(binding.WorkloadName), DatabaseID: binding.DatabaseID,
				MonitorDatasourceID: binding.MonitorDatasourceID, GatewayID: binding.GatewayID, Status: 1,
			}
			if row.WorkloadType == "" {
				row.WorkloadType = "deployment"
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ListOpsApplicationEnvironmentBindings(appID uint) ([]model.OpsApplicationEnvironmentBinding, error) {
	var list []model.OpsApplicationEnvironmentBinding
	err := s.db.Where("app_id = ?", appID).Order("env ASC").Find(&list).Error
	return list, err
}

func (s *Service) DeleteOpsApplication(id uint) error {
	var count int64
	if err := s.db.Model(&model.OpsAppBuildTask{}).Where("app_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该应用下存在构建任务，请先删除构建任务")
	}
	if err := s.db.Model(&model.OpsAppPipeline{}).Where("app_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该应用下存在 CI/CD 流水线，请先删除流水线")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("app_id = ?", id).Delete(&model.OpsApplicationEnvironmentBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.OpsApplication{}, id).Error
	})
}

func (s *Service) ListOpsAppBuildTasks(pageNum, pageSize int, appID uint, keyword, env, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsAppBuildTask{})
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR app_name LIKE ? OR app_code LIKE ? OR description LIKE ?", like, like, like, like)
	}
	if env = strings.TrimSpace(env); env != "" {
		query = query.Where("env = ?", env)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsAppBuildTask
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	s.hydrateOpsAppBuildTaskExecutionPaths(list)
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetOpsAppBuildTask(id uint) (*model.OpsAppBuildTask, error) {
	var item model.OpsAppBuildTask
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.ExecutionPath) == "" {
		if app, err := s.GetOpsApplication(item.AppID); err == nil {
			item.ExecutionPath = s.resolveOpsAppWorkspace(*app)
		}
	}
	return &item, nil
}

func (s *Service) hydrateOpsAppBuildTaskExecutionPaths(list []model.OpsAppBuildTask) {
	appIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for i := range list {
		if strings.TrimSpace(list[i].ExecutionPath) != "" || list[i].AppID == 0 {
			continue
		}
		if _, exists := seen[list[i].AppID]; !exists {
			seen[list[i].AppID] = struct{}{}
			appIDs = append(appIDs, list[i].AppID)
		}
	}
	if len(appIDs) == 0 {
		return
	}
	var apps []model.OpsApplication
	if err := s.db.Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
		return
	}
	byID := make(map[uint]model.OpsApplication, len(apps))
	for _, app := range apps {
		byID[app.ID] = app
	}
	for i := range list {
		if strings.TrimSpace(list[i].ExecutionPath) == "" {
			if app, exists := byID[list[i].AppID]; exists {
				list[i].ExecutionPath = s.resolveOpsAppWorkspace(app)
			}
		}
	}
}

func (s *Service) SaveOpsAppBuildTask(payload OpsAppBuildTaskPayload) error {
	if payload.AppID == 0 {
		return errors.New("请选择所属应用")
	}
	app, err := s.GetOpsApplication(payload.AppID)
	if err != nil {
		return err
	}
	_, buildParamsJSON, err := normalizeOpsAppBuildParameters(payload.BuildParams)
	if err != nil {
		return err
	}
	task := model.OpsAppBuildTask{
		Name:            Trimmed(payload.Name),
		AppID:           app.ID,
		AppName:         app.Name,
		AppCode:         app.Code,
		Env:             Trimmed(payload.Env),
		Branch:          Trimmed(payload.Branch),
		BuildScript:     strings.TrimSpace(payload.BuildScript),
		DeployScript:    strings.TrimSpace(payload.DeployScript),
		BuildParamsJSON: buildParamsJSON,
		RunnerType:      strings.ToLower(Trimmed(payload.RunnerType)),
		RunnerHostID:    payload.RunnerHostID,
		ExecutionPath:   Trimmed(payload.ExecutionPath),
		ArtifactType:    strings.ToLower(Trimmed(payload.ArtifactType)),
		ArtifactPath:    Trimmed(payload.ArtifactPath),
		TimeoutSeconds:  normalizeOpsBuildTimeout(payload.TimeoutSeconds),
		Status:          normalizeOpsAppStatus(payload.Status),
		Description:     Trimmed(payload.Description),
	}
	if task.Name == "" {
		return errors.New("构建任务名称不能为空")
	}
	if task.BuildScript == "" {
		return errors.New("构建脚本不能为空")
	}
	if task.Env == "" {
		task.Env = app.Env
	}
	if task.Env == "" {
		task.Env = "test"
	}
	if task.Branch == "" {
		task.Branch = app.Branch
	}
	if task.Branch == "" && app.RepoType == "git" {
		task.Branch = "master"
	}
	if task.RunnerType == "" {
		task.RunnerType = "local"
	}
	if task.RunnerType != "local" && task.RunnerType != "host" {
		return errors.New("构建执行节点类型不支持")
	}
	if task.RunnerType == "host" && task.RunnerHostID == 0 {
		return errors.New("请选择构建主机")
	}
	if task.ExecutionPath == "" {
		task.ExecutionPath = s.resolveOpsAppWorkspace(*app)
	}
	if task.ArtifactType == "" {
		task.ArtifactType = "file"
	}
	if payload.ID == 0 {
		return s.db.Create(&task).Error
	}
	return s.db.Model(&model.OpsAppBuildTask{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"name": task.Name, "app_id": task.AppID, "app_name": task.AppName, "app_code": task.AppCode,
		"env": task.Env, "branch": task.Branch, "build_script": task.BuildScript, "deploy_script": task.DeployScript,
		"build_params_json": task.BuildParamsJSON,
		"runner_type":       task.RunnerType, "runner_host_id": task.RunnerHostID, "execution_path": task.ExecutionPath,
		"artifact_type": task.ArtifactType, "artifact_path": task.ArtifactPath,
		"timeout_seconds": task.TimeoutSeconds, "status": task.Status, "description": task.Description,
	}).Error
}

func (s *Service) UpdateOpsAppBuildTaskStatus(payload OpsAppBuildTaskStatusPayload) error {
	if payload.ID == 0 {
		return errors.New("构建任务ID不能为空")
	}
	return s.db.Model(&model.OpsAppBuildTask{}).Where("id = ?", payload.ID).Update("status", normalizeOpsAppStatus(payload.Status)).Error
}

func (s *Service) DeleteOpsAppBuildTask(id uint) error {
	return s.db.Delete(&model.OpsAppBuildTask{}, id).Error
}

func (s *Service) RunOpsAppBuildTask(payload OpsAppBuildRunPayload) (map[string]any, error) {
	task, err := s.GetOpsAppBuildTask(payload.TaskID)
	if err != nil {
		return nil, err
	}
	if task.Status != 1 {
		return nil, errors.New("当前构建任务已禁用")
	}
	app, err := s.GetOpsApplication(task.AppID)
	if err != nil {
		return nil, err
	}
	if app.Status != 1 {
		return nil, errors.New("当前应用已禁用，无法构建")
	}
	branch := Trimmed(payload.Branch)
	if branch == "" {
		branch = task.Branch
	}
	if branch == "" {
		branch = app.Branch
	}
	version := Trimmed(payload.Version)
	if version == "" {
		version = time.Now().Format("20060102150405")
	}
	definitions, _, err := normalizeOpsAppBuildParametersJSON(task.BuildParamsJSON)
	if err != nil {
		return nil, err
	}
	params, err := resolveOpsAppBuildParams(definitions, payload.Params)
	if err != nil {
		return nil, err
	}
	paramsJSON, _ := json.Marshal(params)
	now := time.Now()
	workspace := strings.TrimSpace(task.ExecutionPath)
	if workspace == "" {
		workspace = s.resolveOpsAppWorkspace(*app)
	}
	release := model.OpsAppRelease{
		AppID: app.ID, AppName: app.Name, AppCode: app.Code, BuildTaskID: task.ID, BuildTaskName: task.Name,
		Env: task.Env, Version: version, RepoType: app.RepoType, RepoURL: app.RepoURL, Branch: branch,
		Workspace: workspace, Status: "running", Stage: "checkout", Summary: "构建任务已创建，正在拉取代码", ParamsJSON: string(paramsJSON), StartedAt: &now,
	}
	if err := s.db.Create(&release).Error; err != nil {
		return nil, err
	}
	_ = s.db.Model(&model.OpsAppBuildTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"last_release_id": release.ID, "last_status": "running", "last_run_at": &now,
	})
	_ = s.db.Model(&model.OpsApplication{}).Where("id = ?", app.ID).Updates(map[string]any{
		"last_release_id": release.ID, "last_status": "running",
	})
	go s.runOpsAppBuild(opsBuildExecution{
		ReleaseID: release.ID, App: *app, TaskID: task.ID, TaskName: task.Name, Env: task.Env,
		BuildScript: task.BuildScript, DeployScript: task.DeployScript, TimeoutSeconds: task.TimeoutSeconds,
		Branch: branch, Workspace: workspace, RunnerType: task.RunnerType, RunnerHostID: task.RunnerHostID,
		ArtifactType: task.ArtifactType, ArtifactPath: task.ArtifactPath, Version: version, Params: params,
	})
	return map[string]any{"releaseId": release.ID, "status": release.Status, "summary": release.Summary}, nil
}

func (s *Service) ListOpsAppReleases(pageNum, pageSize int, appID uint, keyword, status, env, startTime, endTime string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsAppRelease{})
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("app_name LIKE ? OR app_code LIKE ? OR build_task_name LIKE ? OR version LIKE ? OR summary LIKE ?", like, like, like, like, like)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if env = strings.TrimSpace(env); env != "" {
		query = query.Where("env = ?", env)
	}
	if value, ok := parseOpsReleaseQueryTime(startTime); ok {
		query = query.Where("created_at >= ?", value)
	}
	if value, ok := parseOpsReleaseQueryTime(endTime); ok {
		query = query.Where("created_at <= ?", value)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsAppRelease
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func parseOpsReleaseQueryTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (s *Service) RetryOpsAppRelease(id uint) (map[string]any, error) {
	if id == 0 {
		return nil, errors.New("构建记录 ID 不能为空")
	}
	release, err := s.GetOpsAppRelease(id)
	if err != nil {
		return nil, err
	}
	if release.BuildTaskID == 0 {
		return nil, errors.New("该历史记录不关联构建任务，无法按原配置重试")
	}
	params := map[string]any{}
	if strings.TrimSpace(release.ParamsJSON) != "" {
		if err := json.Unmarshal([]byte(release.ParamsJSON), &params); err != nil {
			return nil, errors.New("原构建参数无法读取，无法重试")
		}
	}
	return s.RunOpsAppBuildTask(OpsAppBuildRunPayload{TaskID: release.BuildTaskID, Branch: release.Branch, Params: params})
}

func (s *Service) GetOpsAppRelease(id uint) (*model.OpsAppRelease, error) {
	var item model.OpsAppRelease
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) ListOpsAppArtifacts(appID uint, env, status string) ([]model.OpsAppArtifact, error) {
	query := s.db.Model(&model.OpsAppArtifact{})
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	if env = normalizeEnvCode(env); env != "" {
		query = query.Where("env = ?", env)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var list []model.OpsAppArtifact
	err := query.Order("id DESC").Limit(500).Find(&list).Error
	return list, err
}

func normalizeOpsImageRegistryAddress(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")), "/")
}

func mapOpsImageRegistry(item model.OpsImageRegistry) map[string]any {
	return map[string]any{"id": item.ID, "name": item.Name, "address": item.Address, "namespace": item.Namespace, "username": item.Username, "hasPassword": strings.TrimSpace(item.Password) != "", "status": item.Status, "description": item.Description, "createTime": item.CreatedAt, "updateTime": item.UpdatedAt}
}

func (s *Service) ListOpsImageRegistries(enabledOnly bool) ([]map[string]any, error) {
	query := s.db.Model(&model.OpsImageRegistry{})
	if enabledOnly {
		query = query.Where("status = ?", 1)
	}
	var list []model.OpsImageRegistry
	if err := query.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		result = append(result, mapOpsImageRegistry(item))
	}
	return result, nil
}

func (s *Service) SaveOpsImageRegistry(payload OpsImageRegistryPayload) error {
	name, address := Trimmed(payload.Name), normalizeOpsImageRegistryAddress(payload.Address)
	if name == "" || address == "" {
		return errors.New("镜像仓库名称和地址不能为空")
	}
	status := 1
	if payload.Status == 2 {
		status = 2
	}
	updates := map[string]any{"name": name, "address": address, "namespace": strings.Trim(strings.TrimSpace(payload.Namespace), "/"), "username": Trimmed(payload.Username), "status": status, "description": Trimmed(payload.Description)}
	if strings.TrimSpace(payload.Password) != "" {
		updates["password"] = payload.Password
	}
	if payload.ID > 0 {
		return s.db.Model(&model.OpsImageRegistry{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	if _, ok := updates["password"]; !ok {
		updates["password"] = ""
	}
	return s.db.Create(&model.OpsImageRegistry{Name: name, Address: address, Namespace: updates["namespace"].(string), Username: updates["username"].(string), Password: updates["password"].(string), Status: updates["status"].(int), Description: updates["description"].(string)}).Error
}

func (s *Service) DeleteOpsImageRegistry(id uint) error {
	if id == 0 {
		return errors.New("镜像仓库 ID 不能为空")
	}
	return s.db.Delete(&model.OpsImageRegistry{}, id).Error
}

func builtinOpsAppPipelineTemplates() []map[string]any {
	templates := []struct {
		ID          uint
		Name        string
		Category    string
		TechStack   string
		Description string
		Stages      []OpsAppPipelineStageDefinition
	}{
		{
			ID: 1000001, Name: "Go 后端通用模板", Category: "Go", TechStack: "go",
			Description: "Go 编译、镜像构建、上传镜像仓库、工作负载更新",
			Stages: []OpsAppPipelineStageDefinition{
				{ID: "checkout", Name: "代码拉取", Type: "checkout", TimeoutSeconds: 600, FailurePolicy: "stop"},
				{ID: "deps", Name: "Go 依赖安装", Type: "command", TimeoutSeconds: 600, FailurePolicy: "stop", Config: map[string]any{"script": "go mod download"}},
				{ID: "test", Name: "单元测试", Type: "test", TimeoutSeconds: 900, FailurePolicy: "stop", Config: map[string]any{"script": "go test ./..."}},
				{ID: "build", Name: "Go 编译", Type: "build", TimeoutSeconds: 900, FailurePolicy: "stop", Config: map[string]any{"script": "go build ./..."}},
				{ID: "docker-build", Name: "Docker 镜像构建", Type: "dockerBuild", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "docker-push", Name: "上传镜像仓库", Type: "dockerPush", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "k8s-deploy", Name: "K8s 工作负载更新", Type: "k8sDeploy", TimeoutSeconds: 900, FailurePolicy: "stop"},
			},
		},
		{
			ID: 1000002, Name: "Maven Java 通用模板", Category: "Java", TechStack: "maven",
			Description: "Maven 打包、Jar 镜像、K8s 发布",
			Stages: []OpsAppPipelineStageDefinition{
				{ID: "checkout", Name: "代码拉取", Type: "checkout", TimeoutSeconds: 600, FailurePolicy: "stop"},
				{ID: "deps", Name: "Maven 依赖安装", Type: "command", TimeoutSeconds: 900, FailurePolicy: "stop", Config: map[string]any{"script": "mvn dependency:go-offline"}},
				{ID: "test", Name: "单元测试", Type: "test", TimeoutSeconds: 1200, FailurePolicy: "stop", Config: map[string]any{"script": "mvn test"}},
				{ID: "package", Name: "Maven 打包", Type: "build", TimeoutSeconds: 1200, FailurePolicy: "stop", Config: map[string]any{"script": "mvn clean package -DskipTests"}},
				{ID: "docker-build", Name: "Docker 镜像构建", Type: "dockerBuild", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "docker-push", Name: "上传镜像仓库", Type: "dockerPush", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "k8s-deploy", Name: "K8s 发布", Type: "k8sDeploy", TimeoutSeconds: 900, FailurePolicy: "stop"},
			},
		},
		{
			ID: 1000003, Name: "Vue 前端通用模板", Category: "Node.js", TechStack: "vue",
			Description: "npm 构建、镜像打包、K8s 滚动发布",
			Stages: []OpsAppPipelineStageDefinition{
				{ID: "checkout", Name: "代码拉取", Type: "checkout", TimeoutSeconds: 600, FailurePolicy: "stop"},
				{ID: "install", Name: "npm install", Type: "command", TimeoutSeconds: 900, FailurePolicy: "stop", Config: map[string]any{"script": "npm install"}},
				{ID: "build", Name: "npm run build", Type: "build", TimeoutSeconds: 900, FailurePolicy: "stop", Config: map[string]any{"script": "npm run build"}},
				{ID: "docker-build", Name: "Docker 镜像构建", Type: "dockerBuild", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "docker-push", Name: "上传镜像仓库", Type: "dockerPush", TimeoutSeconds: 1200, FailurePolicy: "stop"},
				{ID: "k8s-deploy", Name: "K8s 滚动发布", Type: "k8sDeploy", TimeoutSeconds: 900, FailurePolicy: "stop"},
				{ID: "notify", Name: "发布通知", Type: "notify", TimeoutSeconds: 60, FailurePolicy: "ignore"},
			},
		},
	}
	result := make([]map[string]any, 0, len(templates))
	for _, item := range templates {
		definition, _ := json.Marshal(map[string]any{"stages": item.Stages})
		result = append(result, map[string]any{
			"id": item.ID, "name": item.Name, "category": item.Category, "techStack": item.TechStack,
			"description": item.Description, "stageCount": len(item.Stages), "definitionJson": string(definition), "builtin": true, "status": 1,
		})
	}
	return result
}

func (s *Service) ListOpsAppPipelineTemplates(category string) ([]map[string]any, error) {
	category = strings.TrimSpace(category)
	all := builtinOpsAppPipelineTemplates()
	var custom []model.OpsAppPipelineTemplate
	if err := s.db.Where("status = ?", 1).Order("updated_at DESC").Find(&custom).Error; err != nil {
		return nil, err
	}
	for _, item := range custom {
		all = append(all, map[string]any{
			"id": item.ID, "name": item.Name, "category": item.Category, "techStack": item.TechStack,
			"description": item.Description, "stageCount": item.StageCount, "definitionJson": item.DefinitionJSON,
			"builtin": false, "status": item.Status, "createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
		})
	}
	if category == "" || category == "全部模板" {
		return all, nil
	}
	filtered := make([]map[string]any, 0)
	for _, item := range all {
		if strings.EqualFold(fmt.Sprint(item["category"]), category) || strings.EqualFold(fmt.Sprint(item["techStack"]), category) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *Service) SaveOpsAppPipelineTemplate(payload OpsAppPipelineTemplatePayload) error {
	name := Trimmed(payload.Name)
	if name == "" {
		return errors.New("模板名称不能为空")
	}
	stages, definitionJSON, err := normalizeOpsPipelineStages(payload.DefinitionJSON)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		return errors.New("模板至少需要配置一个阶段")
	}
	for index, stage := range stages {
		if strings.TrimSpace(stage.Name) == "" || strings.TrimSpace(stage.Type) == "" {
			return fmt.Errorf("请完善第 %d 个阶段的名称与类型", index+1)
		}
	}
	item := model.OpsAppPipelineTemplate{
		Name:           name,
		Category:       firstNonEmpty(Trimmed(payload.Category), "自定义"),
		TechStack:      firstNonEmpty(Trimmed(payload.TechStack), "custom"),
		Description:    Trimmed(payload.Description),
		StageCount:     len(stages),
		DefinitionJSON: definitionJSON,
		Status:         1,
	}
	return s.db.Create(&item).Error
}

func (s *Service) NormalizeOpsAppPipelineTemplateDefinition(definition string) (map[string]any, error) {
	stages, normalized, err := normalizeOpsPipelineStages(definition)
	if err != nil {
		return nil, err
	}
	return map[string]any{"stages": stages, "definitionJson": normalized}, nil
}

func normalizeOpsPipelineStages(definitionJSON string) ([]OpsAppPipelineStageDefinition, string, error) {
	definitionJSON = strings.TrimSpace(definitionJSON)
	if definitionJSON == "" {
		return []OpsAppPipelineStageDefinition{}, `{"stages":[]}`, nil
	}
	var wrapper struct {
		Stages []OpsAppPipelineStageDefinition `json:"stages"`
	}
	if err := json.Unmarshal([]byte(definitionJSON), &wrapper); err != nil {
		yamlErr := yaml.Unmarshal([]byte(definitionJSON), &wrapper)
		if yamlErr != nil || strings.Contains(definitionJSON, "steps:") {
			stages, shorthandErr := parseOpsPipelineShorthandYAML([]byte(definitionJSON))
			if shorthandErr != nil {
				return nil, "", errors.New("流水线阶段配置不是有效 JSON 或 YAML")
			}
			wrapper.Stages = stages
		}
	}
	for index := range wrapper.Stages {
		if wrapper.Stages[index].ID == "" {
			wrapper.Stages[index].ID = fmt.Sprintf("stage-%d", index+1)
		}
		if wrapper.Stages[index].Name == "" {
			wrapper.Stages[index].Name = fmt.Sprintf("阶段 %d", index+1)
		}
		if wrapper.Stages[index].Type == "" {
			wrapper.Stages[index].Type = "command"
		}
		if wrapper.Stages[index].TimeoutSeconds <= 0 {
			wrapper.Stages[index].TimeoutSeconds = 1800
		}
		if wrapper.Stages[index].FailurePolicy == "" {
			wrapper.Stages[index].FailurePolicy = "stop"
		}
	}
	normalized, _ := json.Marshal(map[string]any{"stages": wrapper.Stages})
	return wrapper.Stages, string(normalized), nil
}

// parseOpsPipelineShorthandYAML accepts the compact form used by the template
// editor. It deliberately hides storage details such as IDs, timeoutSeconds and
// config envelopes while retaining an escape hatch for stage-specific options.
//
// steps:
//   - name: 运行测试
//     type: test
//     run: go test ./...
//   - name: 发布
//     uses: kubernetes-deploy
//     with:
//     namespace: test
//     workload: api
func parseOpsPipelineShorthandYAML(definition []byte) ([]OpsAppPipelineStageDefinition, error) {
	var document map[string]any
	if err := yaml.Unmarshal(definition, &document); err != nil {
		return nil, err
	}
	rawStages, ok := document["steps"].([]any)
	if !ok {
		rawStages, ok = document["stages"].([]any)
	}
	if !ok {
		return nil, errors.New("steps must be a list")
	}
	stages := make([]OpsAppPipelineStageDefinition, 0, len(rawStages))
	for index, raw := range rawStages {
		stage := OpsAppPipelineStageDefinition{TimeoutSeconds: 1800, FailurePolicy: "stop", Config: map[string]any{}, Env: map[string]string{}}
		switch value := raw.(type) {
		case string:
			stage.Type = opsPipelineStageTypeAlias(value)
		case map[string]any:
			// `name/type/run/uses/with` mirrors the vocabulary operators use in
			// Jenkins-like pipeline files. `run` is a script stage; `uses` references
			// a built-in operation such as checkout or kubernetes-deploy.
			if value["type"] != nil || value["uses"] != nil || value["run"] != nil {
				if name, exists := value["name"]; exists && name != nil {
					stage.Name = strings.TrimSpace(fmt.Sprint(name))
				}
				stageType := ""
				if uses, exists := value["uses"]; exists && uses != nil {
					stageType = fmt.Sprint(uses)
				}
				if stageType == "" {
					if stageKind, exists := value["type"]; exists && stageKind != nil {
						stageType = fmt.Sprint(stageKind)
					}
				}
				if stageType == "" {
					stageType = "command"
				}
				stage.Type = opsPipelineStageTypeAlias(stageType)
				if run, exists := value["run"]; exists && run != nil {
					stage.Config["script"] = fmt.Sprint(run)
				}
				if timeout, ok := value["timeout"].(int); ok && timeout > 0 {
					stage.TimeoutSeconds = timeout
				}
				if policy, exists := value["onFailure"]; exists && policy != nil {
					stage.FailurePolicy = fmt.Sprint(policy)
				}
				if options, ok := value["with"].(map[string]any); ok {
					for key, option := range options {
						stage.Config[key] = option
					}
				}
			} else if len(value) != 1 {
				return nil, errors.New("step must include type, uses, or run")
			} else {
				// Compatibility with the initial compact `- test: go test ./...` form.
				for stageType, settings := range value {
					stage.Type = opsPipelineStageTypeAlias(stageType)
					switch option := settings.(type) {
					case string:
						if isOpsPipelineScriptStage(stage.Type) {
							stage.Config["script"] = option
						}
					case map[string]any:
						for key, setting := range option {
							switch key {
							case "name":
								stage.Name = fmt.Sprint(setting)
							case "timeout":
								if timeout, ok := setting.(int); ok && timeout > 0 {
									stage.TimeoutSeconds = timeout
								}
							case "onFailure":
								stage.FailurePolicy = fmt.Sprint(setting)
							case "script":
								stage.Config["script"] = fmt.Sprint(setting)
							default:
								stage.Config[key] = setting
							}
						}
					case nil:
						// A bare map entry, e.g. `- checkout:`, only needs defaults.
					default:
						return nil, errors.New("stage options must be text or a map")
					}
				}
			}
		default:
			return nil, errors.New("stage must be text or a map")
		}
		if stage.Type == "" {
			return nil, errors.New("stage type cannot be empty")
		}
		stage.ID = fmt.Sprintf("stage-%d", index+1)
		stage.Name = firstNonEmpty(stage.Name, opsPipelineStageDisplayName(stage.Type), fmt.Sprintf("阶段 %d", index+1))
		stages = append(stages, stage)
	}
	return stages, nil
}

func opsPipelineStageTypeAlias(stageType string) string {
	switch strings.TrimSpace(stageType) {
	case "image", "docker-build":
		return "dockerBuild"
	case "push", "docker-push":
		return "dockerPush"
	case "deploy", "kubernetes-deploy", "k8s-deploy":
		return "k8sDeploy"
	default:
		return strings.TrimSpace(stageType)
	}
}

func isOpsPipelineScriptStage(stageType string) bool {
	return stageType == "command" || stageType == "test" || stageType == "build"
}

func opsPipelineStageDisplayName(stageType string) string {
	return map[string]string{
		"checkout": "代码拉取", "command": "执行命令", "test": "自动化测试", "build": "编译构建",
		"dockerBuild": "构建镜像", "dockerPush": "推送镜像", "k8sDeploy": "K8s 发布",
		"manual": "人工确认", "notify": "消息通知",
	}[stageType]
}

func validateOpsPipelineStages(stages []OpsAppPipelineStageDefinition, validateDeployTarget bool) error {
	if len(stages) == 0 {
		return errors.New("流水线至少需要配置一个执行阶段")
	}
	allowedTypes := map[string]bool{
		"checkout": true, "command": true, "test": true, "build": true,
		"dockerBuild": true, "dockerPush": true, "k8sDeploy": true,
		"manual": true, "notify": true,
	}
	stageIDs := map[string]bool{}
	buildRegistryByStageID := map[string]uint{}
	buildStageByRegistryID := map[uint]string{}
	for index, stage := range stages {
		if !allowedTypes[stage.Type] {
			return fmt.Errorf("第 %d 个阶段的类型不受支持：%s", index+1, stage.Type)
		}
		if strings.TrimSpace(stage.ID) == "" || stageIDs[stage.ID] {
			return fmt.Errorf("第 %d 个阶段标识为空或重复", index+1)
		}
		stageIDs[stage.ID] = true
		if strings.TrimSpace(stage.Name) == "" {
			return fmt.Errorf("请填写第 %d 个阶段名称", index+1)
		}
		if (stage.Type == "command" || stage.Type == "test" || stage.Type == "build") && strings.TrimSpace(opsPipelineConfigString(stage.Config, "script")) == "" {
			return fmt.Errorf("阶段「%s」缺少执行命令", stage.Name)
		}
		if stage.Type == "notify" && opsPipelineConfigUint(stage.Config, "notifyRuleId") == 0 {
			return fmt.Errorf("消息通知阶段「%s」需要选择通知规则", stage.Name)
		}
		if stage.Type == "dockerBuild" {
			registryID := opsPipelineConfigUint(stage.Config, "registryId")
			if registryID == 0 {
				return fmt.Errorf("镜像构建阶段「%s」需要选择镜像仓库", stage.Name)
			}
			buildRegistryByStageID[stage.ID] = registryID
			if _, exists := buildStageByRegistryID[registryID]; !exists {
				buildStageByRegistryID[registryID] = stage.ID
			}
		}
		if validateDeployTarget && stage.Type == "k8sDeploy" {
			if opsPipelineConfigUint(stage.Config, "clusterId") == 0 || opsPipelineConfigString(stage.Config, "namespace") == "" || opsPipelineConfigString(stage.Config, "workload") == "" || opsPipelineConfigString(stage.Config, "container") == "" {
				return fmt.Errorf("K8s 发布阶段「%s」缺少集群、命名空间、工作负载或容器配置", stage.Name)
			}
		}
	}
	for index := range stages {
		stage := &stages[index]
		if stage.Type != "dockerPush" {
			continue
		}
		if stage.Config == nil {
			stage.Config = map[string]any{}
		}
		sourceStageID := opsPipelineConfigString(stage.Config, "sourceStageId")
		if sourceStageID == "" {
			// Compatibility for existing pipelines that selected the same registry in
			// both stages before source-stage binding was introduced.
			sourceStageID = buildStageByRegistryID[opsPipelineConfigUint(stage.Config, "registryId")]
			if sourceStageID != "" {
				stage.Config["sourceStageId"] = sourceStageID
			}
		}
		registryID := buildRegistryByStageID[sourceStageID]
		if sourceStageID == "" || registryID == 0 {
			return fmt.Errorf("上传镜像阶段「%s」需要引用前面的镜像构建阶段", stage.Name)
		}
		// Persist the resolved registry ID in the execution definition. This makes the
		// pushed tag exactly the image produced by the selected build stage.
		stage.Config["registryId"] = registryID
	}
	return nil
}

func (s *Service) ListOpsAppPipelines(pageNum, pageSize int, appID uint, keyword, env, status, techStack string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsAppPipeline{})
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR app_name LIKE ? OR app_code LIKE ? OR repo_url LIKE ? OR description LIKE ?", like, like, like, like, like)
	}
	if env = strings.TrimSpace(env); env != "" {
		query = query.Where("env = ?", env)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if techStack = strings.TrimSpace(techStack); techStack != "" {
		query = query.Where("tech_stack = ?", techStack)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsAppPipeline
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	var enabled, failed int64
	_ = s.db.Model(&model.OpsAppPipeline{}).Where("status = ?", 1).Count(&enabled).Error
	_ = s.db.Model(&model.OpsAppPipeline{}).Where("last_status = ?", "failed").Count(&failed).Error
	return map[string]any{
		"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize,
		"stats": map[string]any{"total": total, "enabled": enabled, "failed": failed},
	}, nil
}

func (s *Service) GetOpsAppPipeline(id uint) (map[string]any, error) {
	var item model.OpsAppPipeline
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	stages, _, err := normalizeOpsPipelineStages(item.DefinitionJSON)
	if err != nil {
		stages = []OpsAppPipelineStageDefinition{}
	}
	return map[string]any{"pipeline": item, "stages": stages}, nil
}

func (s *Service) SaveOpsAppPipeline(payload OpsAppPipelinePayload) error {
	if payload.AppID == 0 {
		return errors.New("请选择所属应用")
	}
	app, err := s.GetOpsApplication(payload.AppID)
	if err != nil {
		return err
	}
	stages, normalizedDefinition, err := normalizeOpsPipelineStages(payload.DefinitionJSON)
	if err != nil {
		return err
	}
	if err := validateOpsPipelineStages(stages, false); err != nil {
		return err
	}
	definition, _ := json.Marshal(map[string]any{"stages": stages})
	normalizedDefinition = string(definition)
	if payload.ExecutorHostID == 0 {
		return errors.New("请选择流水线执行节点，流水线命令不会在 Ops Admin 容器内执行")
	}
	if _, err := s.getOpsPipelineExecutorHost(payload.ExecutorHostID); err != nil {
		return err
	}
	item := model.OpsAppPipeline{
		Name:           Trimmed(payload.Name),
		AppID:          app.ID,
		AppName:        app.Name,
		AppCode:        app.Code,
		RepoType:       app.RepoType,
		RepoURL:        app.RepoURL,
		DefaultBranch:  Trimmed(payload.DefaultBranch),
		Env:            Trimmed(payload.Env),
		TechStack:      Trimmed(payload.TechStack),
		TemplateID:     payload.TemplateID,
		BuildTaskID:    payload.BuildTaskID,
		ExecutorHostID: payload.ExecutorHostID,
		StageCount:     len(stages),
		Status:         normalizeOpsAppStatus(payload.Status),
		Description:    Trimmed(payload.Description),
		DefinitionJSON: normalizedDefinition,
	}
	if item.Name == "" {
		return errors.New("流水线名称不能为空")
	}
	if item.DefaultBranch == "" {
		item.DefaultBranch = app.Branch
	}
	if item.DefaultBranch == "" && app.RepoType == "git" {
		item.DefaultBranch = "master"
	}
	if item.Env == "" {
		item.Env = app.Env
	}
	if item.Env == "" {
		item.Env = "test"
	}
	if item.TechStack == "" {
		item.TechStack = "custom"
	}
	if payload.ID == 0 {
		return s.db.Create(&item).Error
	}
	return s.db.Model(&model.OpsAppPipeline{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"name": item.Name, "app_id": item.AppID, "app_name": item.AppName, "app_code": item.AppCode,
		"repo_type": item.RepoType, "repo_url": item.RepoURL, "default_branch": item.DefaultBranch,
		"env": item.Env, "tech_stack": item.TechStack, "template_id": item.TemplateID, "build_task_id": item.BuildTaskID, "executor_host_id": item.ExecutorHostID,
		"stage_count": item.StageCount, "status": item.Status, "description": item.Description,
		"definition_json": item.DefinitionJSON,
	}).Error
}

func (s *Service) UpdateOpsAppPipelineStatus(payload OpsAppPipelineStatusPayload) error {
	if payload.ID == 0 {
		return errors.New("流水线 ID 不能为空")
	}
	return s.db.Model(&model.OpsAppPipeline{}).Where("id = ?", payload.ID).Update("status", normalizeOpsAppStatus(payload.Status)).Error
}

func (s *Service) DeleteOpsAppPipeline(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var runIDs []uint
		if err := tx.Model(&model.OpsAppPipelineRun{}).Where("pipeline_id = ?", id).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			if err := tx.Where("run_id IN ?", runIDs).Delete(&model.OpsAppPipelineRunStage{}).Error; err != nil {
				return err
			}
			if err := tx.Where("pipeline_id = ?", id).Delete(&model.OpsAppPipelineRun{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.OpsAppPipeline{}, id).Error
	})
}

func (s *Service) CopyOpsAppPipeline(id uint) (map[string]any, error) {
	var item model.OpsAppPipeline
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	item.ID = 0
	item.Name = item.Name + "-copy"
	item.LastRunID = 0
	item.LastStatus = ""
	item.LastRunAt = nil
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return map[string]any{"id": item.ID}, nil
}

func (s *Service) getOpsPipelineExecutorHost(id uint) (model.AssetHost, error) {
	if id == 0 {
		return model.AssetHost{}, errors.New("请选择流水线执行节点")
	}
	var host model.AssetHost
	if err := s.db.Preload("Group").Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, id).Error; err != nil {
		return model.AssetHost{}, errors.New("流水线执行节点不存在")
	}
	if host.Status != 1 {
		return model.AssetHost{}, errors.New("所选流水线执行节点已停用")
	}
	if host.CredentialID == nil || *host.CredentialID == 0 {
		return model.AssetHost{}, errors.New("所选流水线执行节点未配置 SSH 认证凭据")
	}
	return host, nil
}

func (s *Service) RunOpsAppPipeline(payload OpsAppPipelineRunPayload) (map[string]any, error) {
	var pipeline model.OpsAppPipeline
	if err := s.db.First(&pipeline, payload.PipelineID).Error; err != nil {
		return nil, err
	}
	if pipeline.Status != 1 {
		return nil, errors.New("当前流水线已禁用，无法执行")
	}
	executorHost, err := s.getOpsPipelineExecutorHost(pipeline.ExecutorHostID)
	if err != nil {
		return nil, err
	}
	app, err := s.GetOpsApplication(pipeline.AppID)
	if err != nil {
		return nil, err
	}
	stages, normalizedDefinition, err := normalizeOpsPipelineStages(pipeline.DefinitionJSON)
	if err != nil {
		return nil, err
	}
	branch := Trimmed(payload.Branch)
	if branch == "" {
		branch = pipeline.DefaultBranch
	}
	env := Trimmed(payload.Env)
	if env == "" {
		env = pipeline.Env
	}
	var bindingCount int64
	_ = s.db.Model(&model.OpsApplicationEnvironmentBinding{}).Where("app_id = ?", pipeline.AppID).Count(&bindingCount).Error
	if bindingCount > 0 {
		var binding model.OpsApplicationEnvironmentBinding
		if err := s.db.Where("app_id = ? AND env = ? AND status = ?", pipeline.AppID, normalizeEnvCode(env), 1).First(&binding).Error; err != nil {
			return nil, errors.New("当前应用未配置所选环境的资源绑定")
		}
		for index := range stages {
			if stages[index].Type != "k8sDeploy" {
				continue
			}
			if stages[index].Config == nil {
				stages[index].Config = map[string]any{}
			}
			if opsPipelineConfigUint(stages[index].Config, "clusterId") == 0 && binding.K8sClusterID > 0 {
				stages[index].Config["clusterId"] = binding.K8sClusterID
			}
			if opsPipelineConfigString(stages[index].Config, "namespace") == "" {
				stages[index].Config["namespace"] = binding.Namespace
			}
			if opsPipelineConfigString(stages[index].Config, "workloadType") == "" {
				stages[index].Config["workloadType"] = binding.WorkloadType
			}
			if opsPipelineConfigString(stages[index].Config, "workload") == "" {
				stages[index].Config["workload"] = binding.WorkloadName
			}
		}
		definition, _ := json.Marshal(map[string]any{"stages": stages})
		normalizedDefinition = string(definition)
	}
	if err := validateOpsPipelineStages(stages, true); err != nil {
		return nil, err
	}
	definition, _ := json.Marshal(map[string]any{"stages": stages})
	normalizedDefinition = string(definition)
	if strings.EqualFold(env, "prod") || strings.Contains(env, "生产") {
		hasApproval := false
		for _, stage := range stages {
			if stage.Type == "manual" {
				hasApproval = true
				break
			}
		}
		if !hasApproval {
			return nil, errors.New("生产环境流水线必须包含人工确认阶段")
		}
	}
	if pipeline.BuildTaskID > 0 && payload.ArtifactID == 0 {
		return nil, errors.New("该流水线已绑定构建任务，请选择已成功生成的制品")
	}
	imageTag := Trimmed(payload.ImageTag)
	var artifact model.OpsAppArtifact
	if payload.ArtifactID > 0 {
		artifactQuery := s.db.Where("id = ? AND app_id = ? AND status = ?", payload.ArtifactID, pipeline.AppID, "ready")
		if pipeline.BuildTaskID > 0 {
			artifactQuery = artifactQuery.Where("build_task_id = ?", pipeline.BuildTaskID)
		}
		if normalizedEnv := normalizeEnvCode(env); normalizedEnv != "" {
			artifactQuery = artifactQuery.Where("env = ?", normalizedEnv)
		}
		if err := artifactQuery.First(&artifact).Error; err != nil {
			return nil, errors.New("所选制品不存在、不可用或不属于当前应用")
		}
		if imageTag == "" {
			imageTag = artifact.Version
		}
	}
	if imageTag == "" {
		imageTag = opsPipelineImageTag(branch, time.Now())
	}
	workspace := s.resolveOpsAppWorkspace(*app)
	paramsJSON, _ := json.Marshal(payload.Params)
	now := time.Now()
	run := model.OpsAppPipelineRun{
		PipelineID: pipeline.ID, PipelineName: pipeline.Name, AppID: pipeline.AppID, AppName: pipeline.AppName,
		AppCode: pipeline.AppCode, Env: env, Branch: branch, ImageTag: imageTag, ArtifactID: payload.ArtifactID, TriggerType: "manual",
		ExecutorHostID: pipeline.ExecutorHostID, TriggerUser: "系统管理员", Status: "running", Summary: "流水线已创建，等待阶段执行",
		ParamsJSON: string(paramsJSON), DefinitionJSON: normalizedDefinition, StartedAt: &now,
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, err
	}
	for _, stage := range stages {
		_ = s.db.Create(&model.OpsAppPipelineRunStage{
			RunID: run.ID, StageID: stage.ID, StageName: stage.Name, StageType: stage.Type,
			Status: "waiting", Summary: "等待执行",
		}).Error
	}
	_ = s.db.Model(&model.OpsAppPipeline{}).Where("id = ?", pipeline.ID).Updates(map[string]any{
		"last_run_id": run.ID, "last_status": "running", "last_run_at": &now,
	})
	go s.runOpsAppPipeline(opsPipelineExecution{
		RunID: run.ID, PipelineID: pipeline.ID, App: *app, Branch: branch, Env: env,
		ImageTag: imageTag, Workspace: workspace, ExecutorHost: executorHost, Params: payload.Params, Stages: stages, StartedAt: now,
	})
	return map[string]any{"runId": run.ID, "status": run.Status, "summary": run.Summary}, nil
}

func (s *Service) ListOpsAppPipelineRuns(pageNum, pageSize int, pipelineID, appID uint, keyword, status, env string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.OpsAppPipelineRun{})
	if pipelineID > 0 {
		query = query.Where("pipeline_id = ?", pipelineID)
	}
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("pipeline_name LIKE ? OR app_name LIKE ? OR app_code LIKE ? OR image_tag LIKE ? OR summary LIKE ?", like, like, like, like, like)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if env = strings.TrimSpace(env); env != "" {
		query = query.Where("env = ?", env)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.OpsAppPipelineRun
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetOpsAppPipelineRun(id uint) (map[string]any, error) {
	var run model.OpsAppPipelineRun
	if err := s.db.First(&run, id).Error; err != nil {
		return nil, err
	}
	var stages []model.OpsAppPipelineRunStage
	if err := s.db.Where("run_id = ?", id).Order("id ASC").Find(&stages).Error; err != nil {
		return nil, err
	}
	return map[string]any{"run": run, "stages": stages}, nil
}

func (s *Service) runOpsAppPipeline(execInfo opsPipelineExecution) {
	status, summary := "success", "流水线执行完成"
	for _, stage := range execInfo.Stages {
		var persisted model.OpsAppPipelineRunStage
		_ = s.db.Where("run_id = ? AND stage_id = ?", execInfo.RunID, stage.ID).First(&persisted).Error
		if persisted.Status == "success" {
			continue
		}
		if stage.Type == "manual" {
			now := time.Now()
			_ = s.db.Model(&model.OpsAppPipelineRunStage{}).Where("run_id = ? AND stage_id = ?", execInfo.RunID, stage.ID).Updates(map[string]any{
				"status": "waiting_approval", "summary": "等待人工审批", "started_at": &now,
			}).Error
			_ = s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", execInfo.RunID).Updates(map[string]any{
				"status": "waiting_approval", "summary": "等待人工审批：" + stage.Name, "approval_status": "pending",
			}).Error
			return
		}
		started := time.Now()
		_ = s.db.Model(&model.OpsAppPipelineRunStage{}).
			Where("run_id = ? AND stage_id = ?", execInfo.RunID, stage.ID).
			Updates(map[string]any{"status": "running", "summary": "正在执行", "started_at": &started}).Error
		_ = s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", execInfo.RunID).Update("summary", "正在执行："+stage.Name).Error

		stageStatus, stageSummary := "success", "执行成功"
		logText, err := s.executeOpsAppPipelineStage(execInfo, stage)
		if err != nil {
			stageStatus, stageSummary = "failed", err.Error()
			if strings.EqualFold(stage.FailurePolicy, "ignore") {
				stageStatus, stageSummary = "success", "执行失败，已按策略忽略："+err.Error()
				logText += "\n[WARN] " + stageSummary + "\n"
			} else {
				status, summary = "failed", stage.Name+" 执行失败："+err.Error()
			}
		}
		finished := time.Now()
		if strings.TrimSpace(logText) == "" {
			logText = fmt.Sprintf("[%s] 阶段执行完成：%s\n", finished.Format("2006-01-02 15:04:05"), stage.Name)
		}
		_ = s.db.Model(&model.OpsAppPipelineRunStage{}).
			Where("run_id = ? AND stage_id = ?", execInfo.RunID, stage.ID).
			Updates(map[string]any{
				"status": stageStatus, "summary": stageSummary, "log": logText, "finished_at": &finished,
				"duration_ms": finished.Sub(started).Milliseconds(),
			}).Error
		if status == "failed" {
			break
		}
	}
	finished := time.Now()
	_ = s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", execInfo.RunID).Updates(map[string]any{
		"status": status, "summary": summary, "finished_at": &finished, "duration_ms": finished.Sub(execInfo.StartedAt).Milliseconds(),
	}).Error
	_ = s.db.Model(&model.OpsAppPipeline{}).Where("id = ?", execInfo.PipelineID).Update("last_status", status).Error
}

func (s *Service) ApproveOpsAppPipelineRun(payload OpsAppPipelineApprovalPayload) error {
	if payload.RunID == 0 {
		return errors.New("流水线执行 ID 不能为空")
	}
	var run model.OpsAppPipelineRun
	if err := s.db.First(&run, payload.RunID).Error; err != nil {
		return err
	}
	if run.Status != "waiting_approval" {
		return errors.New("当前流水线不处于待审批状态")
	}
	var stage model.OpsAppPipelineRunStage
	if err := s.db.Where("run_id = ? AND status = ?", run.ID, "waiting_approval").Order("id ASC").First(&stage).Error; err != nil {
		return errors.New("未找到待审批阶段")
	}
	decision := strings.ToLower(strings.TrimSpace(payload.Decision))
	operator := firstNonEmpty(payload.Operator, "系统管理员")
	now := time.Now()
	if decision != "approve" {
		_ = s.db.Model(&model.OpsAppPipelineRunStage{}).Where("id = ?", stage.ID).Updates(map[string]any{"status": "failed", "summary": "审批拒绝：" + payload.Note, "finished_at": &now}).Error
		return s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": "failed", "summary": "人工审批未通过", "approval_status": "rejected", "approver": operator,
			"approval_note": payload.Note, "finished_at": &now,
		}).Error
	}
	if err := s.db.Model(&model.OpsAppPipelineRunStage{}).Where("id = ?", stage.ID).Updates(map[string]any{
		"status": "success", "summary": "审批通过：" + payload.Note, "finished_at": &now,
	}).Error; err != nil {
		return err
	}
	if err := s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status": "running", "summary": "审批通过，继续执行", "approval_status": "approved", "approver": operator, "approval_note": payload.Note,
	}).Error; err != nil {
		return err
	}
	var app model.OpsApplication
	if err := s.db.First(&app, run.AppID).Error; err != nil {
		return err
	}
	executorHost, err := s.getOpsPipelineExecutorHost(run.ExecutorHostID)
	if err != nil {
		return err
	}
	stages, _, err := normalizeOpsPipelineStages(run.DefinitionJSON)
	if err != nil {
		return err
	}
	params := map[string]string{}
	_ = json.Unmarshal([]byte(run.ParamsJSON), &params)
	started := time.Now()
	if run.StartedAt != nil {
		started = *run.StartedAt
	}
	go s.runOpsAppPipeline(opsPipelineExecution{
		RunID: run.ID, PipelineID: run.PipelineID, App: app, Branch: run.Branch, Env: run.Env,
		ImageTag: run.ImageTag, Workspace: s.resolveOpsAppWorkspace(app), ExecutorHost: executorHost, Params: params, Stages: stages, StartedAt: started,
	})
	return nil
}

func (s *Service) RollbackOpsAppPipelineRun(runID uint, operator string) (map[string]any, error) {
	var current model.OpsAppPipelineRun
	if err := s.db.First(&current, runID).Error; err != nil {
		return nil, err
	}
	executorHost, err := s.getOpsPipelineExecutorHost(current.ExecutorHostID)
	if err != nil {
		return nil, err
	}
	var previous model.OpsAppPipelineRun
	if err := s.db.Where("pipeline_id = ? AND status = ? AND id < ? AND image_tag <> ''", current.PipelineID, "success", current.ID).Order("id DESC").First(&previous).Error; err != nil {
		return nil, errors.New("没有找到可回滚的历史成功版本")
	}
	definitions, _, err := normalizeOpsPipelineStages(current.DefinitionJSON)
	if err != nil {
		return nil, err
	}
	stages := make([]OpsAppPipelineStageDefinition, 0)
	for _, stage := range definitions {
		if stage.Type == "k8sDeploy" || stage.Type == "notify" {
			stages = append(stages, stage)
		}
	}
	if len(stages) == 0 {
		return nil, errors.New("当前流水线没有 K8s 发布阶段，无法自动回滚")
	}
	normalized, _ := json.Marshal(map[string]any{"stages": stages})
	now := time.Now()
	run := model.OpsAppPipelineRun{
		PipelineID: current.PipelineID, PipelineName: current.PipelineName, AppID: current.AppID, AppName: current.AppName,
		AppCode: current.AppCode, Env: current.Env, Branch: previous.Branch, ImageTag: previous.ImageTag,
		ArtifactID: previous.ArtifactID, ExecutorHostID: current.ExecutorHostID, TriggerType: "rollback", TriggerUser: firstNonEmpty(operator, "系统管理员"),
		Status: "running", Summary: fmt.Sprintf("正在回滚到执行 #%d / %s", previous.ID, previous.ImageTag),
		DefinitionJSON: string(normalized), StartedAt: &now,
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, err
	}
	for _, stage := range stages {
		_ = s.db.Create(&model.OpsAppPipelineRunStage{RunID: run.ID, StageID: stage.ID, StageName: stage.Name, StageType: stage.Type, Status: "waiting", Summary: "等待执行"}).Error
	}
	var app model.OpsApplication
	if err := s.db.First(&app, run.AppID).Error; err != nil {
		return nil, err
	}
	go s.runOpsAppPipeline(opsPipelineExecution{RunID: run.ID, PipelineID: run.PipelineID, App: app, Branch: run.Branch, Env: run.Env, ImageTag: run.ImageTag, Workspace: s.resolveOpsAppWorkspace(app), ExecutorHost: executorHost, Stages: stages, StartedAt: now})
	return map[string]any{"runId": run.ID, "rollbackFromRunId": current.ID, "rollbackToRunId": previous.ID, "imageTag": previous.ImageTag}, nil
}

func (s *Service) executeOpsAppPipelineStage(execInfo opsPipelineExecution, stage OpsAppPipelineStageDefinition) (string, error) {
	header := fmt.Sprintf("[%s] 阶段开始：%s\n阶段类型：%s\n执行策略：%s\n执行节点：%s (%s)\n工作目录：%s\n\n",
		time.Now().Format("2006-01-02 15:04:05"), stage.Name, stage.Type, stage.FailurePolicy, execInfo.ExecutorHost.HostName, execInfo.ExecutorHost.SSHIP, execInfo.Workspace)
	timeout := time.Duration(normalizeOpsBuildTimeout(stage.TimeoutSeconds)) * time.Second
	// Commands, checkout, Docker and Kubernetes deployment intentionally execute through
	// the selected SSH host. This prevents an Ops Admin container from becoming an
	// accidental CI runner.
	if stage.Type != "manual" && stage.Type != "notify" {
		return s.executeOpsAppPipelineRemoteStage(execInfo, stage, header, timeout)
	}
	switch stage.Type {
	case "checkout":
		if err := os.MkdirAll(filepath.Dir(execInfo.Workspace), 0o755); err != nil {
			return header, err
		}
		output, err := s.checkoutOpsAppCode(execInfo.App, execInfo.Workspace, execInfo.Branch)
		return header + sectionLog("代码拉取", output), err
	case "command", "test", "build":
		script := opsPipelineConfigString(stage.Config, "script")
		if script == "" {
			return header, errors.New("阶段脚本不能为空")
		}
		if err := ensureOpsAppPipelineWorkspace(execInfo.Workspace); err != nil {
			return header, err
		}
		output, err := runOpsAppShell(script, execInfo.Workspace, timeout)
		return header + sectionLog("执行脚本", script) + sectionLog(stage.Name+" 输出", appendOpsAppCommandError(output, err)), err
	case "dockerBuild":
		if err := ensureOpsAppPipelineWorkspace(execInfo.Workspace); err != nil {
			return header, err
		}
		image, imageErr := s.opsPipelineImageName(stage.Config, execInfo)
		if imageErr != nil {
			return header, imageErr
		}
		loginOutput, loginErr := s.loginOpsPipelineRegistry(stage.Config, execInfo.Workspace, timeout)
		if loginErr != nil {
			return header + sectionLog("镜像仓库登录", loginOutput), loginErr
		}
		dockerfile := opsPipelineConfigStringDefault(stage.Config, "dockerfile", "Dockerfile")
		contextDir := opsPipelineConfigStringDefault(stage.Config, "context", ".")
		args := []string{"build", "-t", image, "-f", dockerfile, contextDir}
		output, err := runOpsAppCommand(execInfo.Workspace, timeout, "docker", args...)
		return header + sectionLog("镜像仓库登录", loginOutput) + sectionLog("Docker Build: "+image, appendOpsAppCommandError(output, err)), err
	case "dockerPush":
		if err := ensureOpsAppPipelineWorkspace(execInfo.Workspace); err != nil {
			return header, err
		}
		image, imageErr := s.opsPipelineImageName(stage.Config, execInfo)
		if imageErr != nil {
			return header, imageErr
		}
		loginOutput, loginErr := s.loginOpsPipelineRegistry(stage.Config, execInfo.Workspace, timeout)
		if loginErr != nil {
			return header + sectionLog("镜像仓库登录", loginOutput), loginErr
		}
		output, err := runOpsAppCommand(execInfo.Workspace, timeout, "docker", "push", image)
		return header + sectionLog("镜像仓库登录", loginOutput) + sectionLog("上传镜像仓库: "+image, appendOpsAppCommandError(output, err)), err
	case "k8sDeploy":
		clusterID := opsPipelineConfigUint(stage.Config, "clusterId")
		if clusterID == 0 {
			return header, errors.New("K8s 发布需要选择目标集群")
		}
		kubeconfigPath, cleanup, err := s.opsPipelineKubeconfigFile(clusterID)
		if err != nil {
			return header, err
		}
		defer cleanup()
		namespace := opsPipelineConfigString(stage.Config, "namespace")
		workloadType := strings.ToLower(opsPipelineConfigStringDefault(stage.Config, "workloadType", "deployment"))
		workload := opsPipelineConfigString(stage.Config, "workload")
		container := opsPipelineConfigString(stage.Config, "container")
		switch workloadType {
		case "deployment", "statefulset", "daemonset":
		default:
			return header, fmt.Errorf("不支持的 K8s 工作负载类型：%s", workloadType)
		}
		if namespace == "" {
			namespace = execInfo.Env
		}
		if namespace == "" {
			namespace = "default"
		}
		if workload == "" {
			workload = execInfo.App.Code
		}
		if container == "" {
			container = execInfo.App.Code
		}
		image, imageErr := s.opsPipelineImageName(stage.Config, execInfo)
		if imageErr != nil {
			return header, imageErr
		}
		target := workloadType + "/" + workload
		output, err := runOpsAppCommand(".", timeout, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "set", "image", target, container+"="+image)
		if err != nil {
			return header + sectionLog("kubectl set image", appendOpsAppCommandError(output, err)), err
		}
		rollout, rolloutErr := runOpsAppCommand(".", timeout, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "rollout", "status", target, "--timeout="+fmt.Sprintf("%ds", stage.TimeoutSeconds))
		logText := header + sectionLog("kubectl set image", output) + sectionLog("kubectl rollout status", appendOpsAppCommandError(rollout, rolloutErr))
		if rolloutErr != nil {
			return logText, rolloutErr
		}
		healthURL := opsPipelineConfigString(stage.Config, "healthUrl")
		if healthURL == "" {
			return logText, nil
		}
		healthOutput, healthErr := runOpsAppHealthCheck(healthURL, timeout)
		return logText + sectionLog("发布后健康检查: "+healthURL, appendOpsAppCommandError(healthOutput, healthErr)), healthErr
	case "manual":
		return header + "人工确认阶段由流水线调度器处理：执行会暂停，待审批通过后继续。\n", nil
	case "notify":
		ruleID := opsPipelineConfigUint(stage.Config, "notifyRuleId")
		now := time.Now()
		queued, err := s.enqueueNotifyRule(ruleID, NotifyEvent{
			Scope: "pipeline", Event: "notify", TargetID: execInfo.RunID,
			TargetName: execInfo.App.Name + " / " + stage.Name,
			Status:     "notify",
			Summary:    fmt.Sprintf("流水线 #%d 已执行到通知阶段「%s」", execInfo.RunID, stage.Name),
			Detail:     fmt.Sprintf("应用：%s\n环境：%s\n分支：%s\n镜像版本：%s", execInfo.App.Name, execInfo.Env, execInfo.Branch, execInfo.ImageTag),
			StartedAt:  &now, FinishedAt: &now,
			Extra: map[string]string{
				"pipelineName": fmt.Sprintf("流水线 #%d", execInfo.PipelineID), "pipelineRunId": fmt.Sprintf("%d", execInfo.RunID),
				"appName": execInfo.App.Name, "env": execInfo.Env, "branch": execInfo.Branch, "imageTag": execInfo.ImageTag,
				"stageName": stage.Name, "notifyAt": now.Format("2006-01-02 15:04:05"),
			},
		}, false)
		if err != nil {
			return header, fmt.Errorf("发送消息通知失败: %w", err)
		}
		if queued == 0 {
			return header, errors.New("所选通知规则未产生有效投递，请检查规则、模板和通知媒介状态")
		}
		return header + fmt.Sprintf("已通过通知规则 #%d 创建 %d 条投递任务，实际发送结果请在消息通知 / 发送日志查看。\n", ruleID, queued), nil
	default:
		return header, fmt.Errorf("不支持的阶段类型：%s", stage.Type)
	}
}

func (s *Service) executeOpsAppPipelineRemoteStage(execInfo opsPipelineExecution, stage OpsAppPipelineStageDefinition, header string, timeout time.Duration) (string, error) {
	if execInfo.ExecutorHost.ID == 0 {
		return header, errors.New("流水线未配置执行节点")
	}
	seconds := normalizeOpsBuildTimeout(stage.TimeoutSeconds)
	run := func(name, command string) (string, error) {
		result := s.execCommandOnHost(execInfo.ExecutorHost, command, seconds)
		output := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
		return sectionLog(name, output), func() error {
			if result.Status == "success" {
				return nil
			}
			return errors.New(firstNonEmpty(result.ErrorText, "远程执行失败"))
		}()
	}
	workspace := filepath.ToSlash(execInfo.Workspace)
	switch stage.Type {
	case "checkout":
		output, err := run("远程代码拉取", s.remoteOpsAppCheckoutCommand(execInfo.App, workspace, execInfo.Branch))
		return header + output, err
	case "command", "test", "build":
		script := opsPipelineConfigString(stage.Config, "script")
		if script == "" {
			return header, errors.New("阶段脚本不能为空")
		}
		output, err := run("远程执行脚本", remoteOpsPipelineScriptCommand(workspace, script, execInfo))
		return header + sectionLog("执行脚本", script) + output, err
	case "dockerBuild":
		image, err := s.opsPipelineImageName(stage.Config, execInfo)
		if err != nil {
			return header, err
		}
		login, err := s.opsPipelineRegistryLoginCommand(stage.Config)
		if err != nil {
			return header, err
		}
		dockerfile := opsPipelineConfigStringDefault(stage.Config, "dockerfile", "Dockerfile")
		contextDir := opsPipelineConfigStringDefault(stage.Config, "context", ".")
		command := "cd " + shellQuote(workspace) + " && " + login + " && docker build -t " + shellQuote(image) + " -f " + shellQuote(dockerfile) + " " + shellQuote(contextDir)
		output, runErr := run("远程 Docker Build: "+image, command)
		return header + output, runErr
	case "dockerPush":
		image, err := s.opsPipelineImageName(stage.Config, execInfo)
		if err != nil {
			return header, err
		}
		login, err := s.opsPipelineRegistryLoginCommand(stage.Config)
		if err != nil {
			return header, err
		}
		output, runErr := run("远程上传镜像仓库: "+image, "cd "+shellQuote(workspace)+" && "+login+" && docker push "+shellQuote(image))
		return header + output, runErr
	case "k8sDeploy":
		clusterID := opsPipelineConfigUint(stage.Config, "clusterId")
		if clusterID == 0 {
			return header, errors.New("K8s 发布需要选择目标集群")
		}
		kubeconfigPath, cleanup, err := s.opsPipelineKubeconfigFile(clusterID)
		if err != nil {
			return header, err
		}
		defer cleanup()
		kubeconfig, err := os.ReadFile(kubeconfigPath)
		if err != nil {
			return header, err
		}
		namespace := firstNonEmpty(opsPipelineConfigString(stage.Config, "namespace"), execInfo.Env, "default")
		workloadType := strings.ToLower(opsPipelineConfigStringDefault(stage.Config, "workloadType", "deployment"))
		if workloadType != "deployment" && workloadType != "statefulset" && workloadType != "daemonset" {
			return header, fmt.Errorf("不支持的 K8s 工作负载类型：%s", workloadType)
		}
		workload := firstNonEmpty(opsPipelineConfigString(stage.Config, "workload"), execInfo.App.Code)
		container := firstNonEmpty(opsPipelineConfigString(stage.Config, "container"), execInfo.App.Code)
		image, err := s.opsPipelineImageName(stage.Config, execInfo)
		if err != nil {
			return header, err
		}
		target := workloadType + "/" + workload
		encoded := base64.StdEncoding.EncodeToString(kubeconfig)
		command := "kcfg=$(mktemp) && trap 'rm -f \"$kcfg\"' EXIT && base64 -d <<'__OPS_KUBECONFIG__' > \"$kcfg\"\n" + encoded + "\n__OPS_KUBECONFIG__\nkubectl --kubeconfig \"$kcfg\" -n " + shellQuote(namespace) + " set image " + shellQuote(target) + " " + shellQuote(container+"="+image) + " && kubectl --kubeconfig \"$kcfg\" -n " + shellQuote(namespace) + " rollout status " + shellQuote(target) + " --timeout=" + fmt.Sprintf("%ds", seconds)
		output, runErr := run("远程 kubectl 发布: "+target, command)
		if runErr != nil {
			return header + output, runErr
		}
		healthURL := opsPipelineConfigString(stage.Config, "healthUrl")
		if healthURL == "" {
			return header + output, nil
		}
		healthOutput, healthErr := run("远程发布后健康检查: "+healthURL, "curl -fsS --max-time "+strconv.Itoa(seconds)+" "+shellQuote(healthURL))
		return header + output + healthOutput, healthErr
	default:
		return header, fmt.Errorf("不支持的阶段类型：%s", stage.Type)
	}
}

func remoteOpsPipelineScriptCommand(workspace, script string, execInfo opsPipelineExecution) string {
	variables := map[string]string{"BRANCH": execInfo.Branch, "ENVIRONMENT": execInfo.Env, "IMAGE_TAG": execInfo.ImageTag, "PROJECT_NAME": execInfo.App.Name, "PROJECT_CODE": execInfo.App.Code, "PIPELINE_RUN_ID": strconv.FormatUint(uint64(execInfo.RunID), 10)}
	for key, value := range execInfo.Params {
		variables[key] = value
	}
	return remoteOpsAppScriptCommand(workspace, script, variables)
}

func (s *Service) opsPipelineRegistryLoginCommand(config map[string]any) (string, error) {
	registryID := opsPipelineConfigUint(config, "registryId")
	if registryID == 0 {
		return "", errors.New("请选择镜像仓库")
	}
	var registry model.OpsImageRegistry
	if err := s.db.First(&registry, registryID).Error; err != nil {
		return "", errors.New("所选镜像仓库不存在")
	}
	if registry.Status != 1 {
		return "", errors.New("所选镜像仓库已停用")
	}
	if strings.EqualFold(opsPipelineConfigString(config, "loginMode"), "executor") {
		return "echo '使用执行节点已有的 Docker 登录会话'", nil
	}
	if strings.TrimSpace(registry.Username) == "" || strings.TrimSpace(registry.Password) == "" {
		return "", errors.New("镜像仓库未配置登录账号或密码；请改用“执行节点已有登录会话”，或在镜像仓库中补充凭据")
	}
	return "printf %s " + shellQuote(registry.Password) + " | docker login " + shellQuote(registry.Address) + " --username " + shellQuote(registry.Username) + " --password-stdin", nil
}

func (s *Service) opsPipelineKubeconfigFile(clusterID uint) (string, func(), error) {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return "", func() {}, err
	}
	if strings.TrimSpace(cluster.KubeConfig) == "" {
		return "", func() {}, errors.New("目标集群 kubeconfig 为空")
	}
	content := cluster.KubeConfig
	tunnelCleanup := func() {}
	if normalizeConnectionMode(cluster.ConnectionMode) == "gateway" && cluster.GatewayID != nil && *cluster.GatewayID > 0 {
		rewritten, cleanup, err := s.gatewayKubeconfigForKubectl(cluster)
		if err != nil {
			return "", func() {}, err
		}
		content = rewritten
		tunnelCleanup = cleanup
	}
	file, err := os.CreateTemp("", fmt.Sprintf("ops-admin-kubeconfig-%d-*.yaml", clusterID))
	if err != nil {
		tunnelCleanup()
		return "", func() {}, err
	}
	path := file.Name()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		tunnelCleanup()
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		tunnelCleanup()
		return "", func() {}, err
	}
	return path, func() {
		_ = os.Remove(path)
		tunnelCleanup()
	}, nil
}

func (s *Service) gatewayKubeconfigForKubectl(cluster model.K8sCluster) (string, func(), error) {
	var cfg kubeConfig
	if err := yaml.Unmarshal([]byte(cluster.KubeConfig), &cfg); err != nil {
		return "", func() {}, err
	}
	runtime, err := parseKubeConfig(cluster.KubeConfig)
	if err != nil {
		return "", func() {}, err
	}
	parsed, err := url.Parse(runtime.Server)
	if err != nil {
		return "", func() {}, err
	}
	targetAddress := parsed.Host
	if !strings.Contains(targetAddress, ":") {
		if parsed.Scheme == "http" {
			targetAddress += ":80"
		} else {
			targetAddress += ":443"
		}
	}
	localAddress, cleanup, err := s.startGatewayTunnel(*cluster.GatewayID, targetAddress)
	if err != nil {
		return "", func() {}, err
	}
	localScheme := parsed.Scheme
	if localScheme == "" {
		localScheme = "https"
	}
	for i := range cfg.Clusters {
		cfg.Clusters[i].Cluster.Server = localScheme + "://" + localAddress
		cfg.Clusters[i].Cluster.InsecureSkipTLSVerify = true
		cfg.Clusters[i].Cluster.CertificateAuthorityData = ""
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return string(data), cleanup, nil
}

func ensureOpsAppPipelineWorkspace(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || workspace == "." {
		return nil
	}
	return os.MkdirAll(workspace, 0o755)
}

func appendOpsAppCommandError(output string, err error) string {
	if err == nil {
		return output
	}
	if strings.TrimSpace(output) == "" {
		return "[ERROR] " + err.Error() + "\n"
	}
	return output + "\n[ERROR] " + err.Error() + "\n"
}

func opsPipelineConfigString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func opsPipelineConfigStringDefault(config map[string]any, key string, fallback string) string {
	if value := opsPipelineConfigString(config, key); value != "" {
		return value
	}
	return fallback
}

func opsPipelineConfigUint(config map[string]any, key string) uint {
	value := opsPipelineConfigString(config, key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return uint(parsed)
}

func opsPipelineImageTag(branch string, now time.Time) string {
	branch = strings.ToLower(strings.TrimSpace(branch))
	branch = strings.NewReplacer("/", "-", "_", "-", " ", "-", ":", "-").Replace(branch)
	branch = strings.Trim(branch, "-.")
	if branch == "" {
		branch = "main"
	}
	return branch + "-" + now.Format("20060102150405")
}

func (s *Service) opsPipelineImageName(config map[string]any, execInfo opsPipelineExecution) (string, error) {
	image := opsPipelineConfigString(config, "image")
	if image != "" {
		return opsPipelineReplaceVars(image, execInfo), nil
	}
	registryID := opsPipelineConfigUint(config, "registryId")
	if registryID > 0 {
		var registry model.OpsImageRegistry
		if err := s.db.Where("id = ? AND status = ?", registryID, 1).First(&registry).Error; err != nil {
			return "", errors.New("所选镜像仓库不存在或已禁用")
		}
		parts := []string{strings.Trim(registry.Address, "/")}
		if namespace := strings.Trim(registry.Namespace, "/"); namespace != "" {
			parts = append(parts, namespace)
		}
		parts = append(parts, execInfo.App.Code)
		return strings.Join(parts, "/") + ":" + execInfo.ImageTag, nil
	}
	repository := opsPipelineConfigString(config, "repository")
	if repository == "" {
		repository = execInfo.App.Code
	}
	repository = opsPipelineReplaceVars(repository, execInfo)
	if !strings.Contains(repository, ":") {
		repository += ":" + execInfo.ImageTag
	}
	return repository, nil
}

func (s *Service) loginOpsPipelineRegistry(config map[string]any, workspace string, timeout time.Duration) (string, error) {
	registryID := opsPipelineConfigUint(config, "registryId")
	if registryID == 0 {
		return "未配置镜像仓库登录凭据，跳过 docker login。\n", nil
	}
	var registry model.OpsImageRegistry
	if err := s.db.Where("id = ? AND status = ?", registryID, 1).First(&registry).Error; err != nil {
		return "", errors.New("所选镜像仓库不存在或已禁用")
	}
	if strings.TrimSpace(registry.Username) == "" || strings.TrimSpace(registry.Password) == "" {
		return "未配置仓库登录凭据，使用当前 Docker 客户端会话。\n", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "login", registry.Address, "--username", registry.Username, "--password-stdin")
	cmd.Dir = workspace
	cmd.Stdin = strings.NewReader(registry.Password)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("docker login 超时（%s）", timeout)
	}
	return string(output), err
}

func opsPipelineReplaceVars(value string, execInfo opsPipelineExecution) string {
	replacer := strings.NewReplacer(
		"{{appCode}}", execInfo.App.Code,
		"{{appName}}", execInfo.App.Name,
		"{{env}}", execInfo.Env,
		"{{branch}}", execInfo.Branch,
		"{{imageTag}}", execInfo.ImageTag,
	)
	return replacer.Replace(value)
}

func (s *Service) RunOpsAppRelease(payload OpsAppReleasePayload) (map[string]any, error) {
	app, err := s.GetOpsApplication(payload.AppID)
	if err != nil {
		return nil, err
	}
	if app.Status != 1 {
		return nil, errors.New("当前应用已禁用，无法发布")
	}
	buildScript := strings.TrimSpace(app.BuildScript)
	if buildScript == "" {
		var task model.OpsAppBuildTask
		err := s.db.Where("app_id = ? AND status = ?", app.ID, 1).Order("id DESC").First(&task).Error
		if err == nil {
			return s.RunOpsAppBuildTask(OpsAppBuildRunPayload{TaskID: task.ID, Version: payload.Version, Branch: payload.Branch})
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, errors.New("请先为应用创建构建任务")
	}
	branch := Trimmed(payload.Branch)
	if branch == "" {
		branch = app.Branch
	}
	version := Trimmed(payload.Version)
	if version == "" {
		version = time.Now().Format("20060102150405")
	}
	now := time.Now()
	workspace := s.resolveOpsAppWorkspace(*app)
	release := model.OpsAppRelease{
		AppID: app.ID, AppName: app.Name, AppCode: app.Code, Env: app.Env, Version: version, RepoType: app.RepoType,
		RepoURL: app.RepoURL, Branch: branch, Workspace: workspace, Status: "running", Stage: "checkout",
		Summary: "构建任务已创建，正在拉取代码", StartedAt: &now,
	}
	if err := s.db.Create(&release).Error; err != nil {
		return nil, err
	}
	_ = s.db.Model(&model.OpsApplication{}).Where("id = ?", app.ID).Updates(map[string]any{
		"last_release_id": release.ID, "last_status": "running",
	})
	go s.runOpsAppBuild(opsBuildExecution{
		ReleaseID: release.ID, App: *app, Env: app.Env, BuildScript: buildScript, DeployScript: app.DeployScript,
		TimeoutSeconds: 1800, Branch: branch, Workspace: workspace,
	})
	return map[string]any{"releaseId": release.ID, "status": release.Status, "summary": release.Summary}, nil
}

func (s *Service) resolveOpsAppWorkspace(app model.OpsApplication) string {
	if strings.TrimSpace(app.Workspace) != "" {
		return strings.TrimSpace(app.Workspace)
	}
	return filepath.Join("uploads", "apps", app.Code)
}

func (s *Service) runOpsAppBuild(execInfo opsBuildExecution) {
	started := time.Now()
	workspace := execInfo.Workspace
	buildLogs := newOpsBuildLogWriter(s, execInfo.ReleaseID, "build_log")
	postBuildLogs := newOpsBuildLogWriter(s, execInfo.ReleaseID, "deploy_log")
	status, stage, summary := "success", "done", "构建及构建后操作已完成"
	commitID := ""
	timeout := time.Duration(normalizeOpsBuildTimeout(execInfo.TimeoutSeconds)) * time.Second

	if strings.EqualFold(execInfo.RunnerType, "host") {
		remoteWorkspace := filepath.ToSlash(workspace)
		var host model.AssetHost
		if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, execInfo.RunnerHostID).Error; err != nil {
			status, stage, summary = "failed", "prepare", "构建主机不存在或不可用"
		} else {
			checkoutCommand := s.remoteOpsAppCheckoutCommand(execInfo.App, remoteWorkspace, execInfo.Branch)
			buildLogs.Append(sectionLog("Remote Checkout", ""))
			_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{"stage": "checkout", "summary": "正在拉取代码"}).Error
			checkoutResult := s.execCommandOnHostStreaming(host, checkoutCommand, normalizeOpsBuildTimeout(execInfo.TimeoutSeconds), func(chunk string) {
				buildLogs.Append(s.sanitizeOpsAppLog(execInfo.App, chunk))
			})
			buildLogs.Flush()
			if checkoutResult.Status != "success" {
				if checkoutResult.ErrorText != "" {
					buildLogs.Append("\nERROR: " + checkoutResult.ErrorText + "\n")
				}
				status, stage, summary = "failed", "checkout", firstNonEmpty(checkoutResult.ErrorText, "远程代码拉取失败")
			} else {
				commitResult := s.execCommandOnHost(host, "cd "+shellQuote(remoteWorkspace)+" && (git rev-parse --short HEAD 2>/dev/null || svn info --show-item revision 2>/dev/null || true)", 30)
				commitID = strings.TrimSpace(commitResult.Stdout)
				variables := s.opsAppBuildEnvironment(execInfo, commitID, remoteWorkspace)
				stage = "build"
				buildLogs.Append(sectionLog("Remote Build", ""))
				_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{"stage": stage, "summary": "正在执行构建脚本", "commit_id": commitID}).Error
				buildResult := s.execCommandOnHostStreaming(host, remoteOpsAppScriptCommand(remoteWorkspace, execInfo.BuildScript, variables), normalizeOpsBuildTimeout(execInfo.TimeoutSeconds), func(chunk string) {
					buildLogs.Append(s.sanitizeOpsAppLog(execInfo.App, chunk))
				})
				buildLogs.Flush()
				if buildResult.Status != "success" {
					if buildResult.ErrorText != "" {
						buildLogs.Append("\nERROR: " + buildResult.ErrorText + "\n")
					}
					status, summary = "failed", firstNonEmpty(buildResult.ErrorText, "远程构建失败")
				} else if strings.TrimSpace(execInfo.DeployScript) != "" {
					stage = "post_build"
					postBuildLogs.Append(sectionLog("Remote Post Build", ""))
					_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{"stage": stage, "summary": "正在执行构建后操作"}).Error
					postResult := s.execCommandOnHostStreaming(host, remoteOpsAppScriptCommand(remoteWorkspace, execInfo.DeployScript, variables), normalizeOpsBuildTimeout(execInfo.TimeoutSeconds), func(chunk string) {
						postBuildLogs.Append(s.sanitizeOpsAppLog(execInfo.App, chunk))
					})
					postBuildLogs.Flush()
					if postResult.Status != "success" {
						if postResult.ErrorText != "" {
							postBuildLogs.Append("\nERROR: " + postResult.ErrorText + "\n")
						}
						status, summary = "failed", firstNonEmpty(postResult.ErrorText, "远程构建后操作失败")
					}
				}
			}
		}
	} else if err := os.MkdirAll(filepath.Dir(workspace), 0o755); err != nil {
		status, stage, summary = "failed", "prepare", err.Error()
	} else {
		stage = "checkout"
		_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{"stage": stage, "summary": "正在拉取代码"})
		checkoutLog, err := s.checkoutOpsAppCode(execInfo.App, workspace, execInfo.Branch)
		buildLogs.Append(sectionLog("Git Clone", s.sanitizeOpsAppLog(execInfo.App, checkoutLog)))
		if err != nil {
			status, summary = "failed", err.Error()
		} else {
			commitID = s.detectOpsAppCommit(execInfo.App.RepoType, workspace)
			variables := s.opsAppBuildEnvironment(execInfo, commitID, workspace)
			stage = "build"
			_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{
				"stage": stage, "summary": "正在执行构建脚本", "build_log": buildLogs.String(), "commit_id": commitID,
			})
			buildOutput, buildErr := runOpsAppShellWithEnv(execInfo.BuildScript, workspace, timeout, variables)
			buildLogs.Append(sectionLog("Build", buildOutput))
			if buildErr != nil {
				status, summary = "failed", buildErr.Error()
			} else if strings.TrimSpace(execInfo.DeployScript) != "" {
				stage = "post_build"
				_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{
					"stage": stage, "summary": "正在执行构建后操作", "build_log": buildLogs.String(), "commit_id": commitID,
				})
				postOutput, postErr := runOpsAppShellWithEnv(execInfo.DeployScript, workspace, timeout, variables)
				postBuildLogs.Append(sectionLog("Post Build", postOutput))
				if postErr != nil {
					status, summary = "failed", postErr.Error()
				}
			}
		}
	}
	finished := time.Now()
	if status != "success" && summary == "" {
		summary = "构建失败"
	}
	buildLogs.Flush()
	postBuildLogs.Flush()
	_ = s.db.Model(&model.OpsAppRelease{}).Where("id = ?", execInfo.ReleaseID).Updates(map[string]any{
		"status": status, "stage": stage, "summary": summary, "build_log": buildLogs.String(), "deploy_log": postBuildLogs.String(),
		"commit_id": commitID, "finished_at": &finished, "duration_ms": finished.Sub(started).Milliseconds(),
	})
	updates := map[string]any{"last_status": status}
	if status == "success" {
		artifactURI := strings.TrimSpace(execInfo.ArtifactPath)
		if artifactURI == "" {
			artifactURI = workspace
			if strings.EqualFold(execInfo.RunnerType, "host") {
				artifactURI = filepath.ToSlash(workspace)
			}
		}
		_ = s.db.Create(&model.OpsAppArtifact{
			AppID: execInfo.App.ID, AppName: execInfo.App.Name, BuildTaskID: execInfo.TaskID,
			ReleaseID: execInfo.ReleaseID, Env: execInfo.Env, Version: execInfo.Version, CommitID: commitID,
			Type: firstNonEmpty(execInfo.ArtifactType, "file"), URI: artifactURI, Status: "ready",
		}).Error
	}
	_ = s.db.Model(&model.OpsApplication{}).Where("id = ?", execInfo.App.ID).Updates(updates)
	if execInfo.TaskID > 0 {
		taskUpdates := map[string]any{"last_status": status}
		if status == "success" {
			taskUpdates["success_count"] = gorm.Expr("success_count + 1")
		} else {
			taskUpdates["failed_count"] = gorm.Expr("failed_count + 1")
		}
		_ = s.db.Model(&model.OpsAppBuildTask{}).Where("id = ?", execInfo.TaskID).Updates(taskUpdates)
	}
}

func (s *Service) remoteOpsAppCheckoutCommand(app model.OpsApplication, workspace, branch string) string {
	target := filepath.ToSlash(workspace)
	parent := pathpkg.Dir(target)
	repoURL := s.opsAppAuthenticatedRepoURL(app)
	if app.RepoType == "svn" {
		revision := strings.TrimSpace(branch)
		revisionArg := ""
		if revision != "" && !strings.EqualFold(revision, "HEAD") {
			revisionArg = " -r " + shellQuote(revision)
		}
		return fmt.Sprintf("mkdir -p %s && if [ -d %s/.svn ]; then cd %s && svn update%s; else svn checkout%s %s %s; fi", shellQuote(parent), shellQuote(target), shellQuote(target), revisionArg, revisionArg, shellQuote(repoURL), shellQuote(target))
	}
	branchArg := ""
	if strings.TrimSpace(branch) != "" {
		branchArg = " -b " + shellQuote(branch)
	}
	gitRetry := "git_retry() { attempts=3; attempt=1; until GIT_TERMINAL_PROMPT=0 git \"$@\"; do rc=$?; if [ $attempt -ge $attempts ]; then exit $rc; fi; echo \"Git command failed (attempt $attempt/$attempts), retrying in 3 seconds...\" >&2; sleep 3; attempt=$((attempt + 1)); done; }; "
	checkoutCommand := "git_retry pull --ff-only"
	if strings.TrimSpace(branch) != "" {
		checkoutCommand = "git checkout " + shellQuote(branch) + " && git_retry pull --ff-only"
	}
	return fmt.Sprintf("%smkdir -p %s && if [ -d %s/.git ]; then cd %s && git_retry fetch --all --prune && %s; else git_retry clone%s %s %s; fi", gitRetry, shellQuote(parent), shellQuote(target), shellQuote(target), checkoutCommand, branchArg, shellQuote(repoURL), shellQuote(target))
}

func (s *Service) opsAppBuildEnvironment(execInfo opsBuildExecution, commitID, buildPath string) map[string]string {
	variables := make(map[string]string, len(execInfo.Params)+12)
	for key, value := range execInfo.Params {
		variables[key] = value
	}
	variables["BUILD_NUMBER"] = strconv.FormatUint(uint64(execInfo.ReleaseID), 10)
	variables["VERSION"] = execInfo.Version
	variables["COMMIT_ID"] = commitID
	variables["BRANCH"] = execInfo.Branch
	variables["PROJECT_NAME"] = execInfo.App.Name
	variables["PROJECT_ID"] = strconv.FormatUint(uint64(execInfo.App.ID), 10)
	variables["PROJECT_REPO"] = execInfo.App.RepoURL
	variables["TASK_NAME"] = execInfo.TaskName
	variables["TASK_ID"] = strconv.FormatUint(uint64(execInfo.TaskID), 10)
	variables["ENVIRONMENT"] = execInfo.Env
	variables["ENVIRONMENT_TYPE"] = execInfo.Env
	variables["BUILD_PATH"] = buildPath
	return variables
}

func remoteOpsAppScriptCommand(workspace, script string, variables map[string]string) string {
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	exports := make([]string, 0, len(keys))
	for _, key := range keys {
		exports = append(exports, "export "+key+"="+shellQuote(variables[key]))
	}
	return "cd " + shellQuote(workspace) + " && " + strings.Join(exports, " && ") + " && sh -c " + shellQuote(script)
}

func sectionLog(name string, output string) string {
	if strings.TrimSpace(output) == "" {
		return fmt.Sprintf("\n===== %s =====\n", name)
	}
	return fmt.Sprintf("\n===== %s =====\n%s\n", name, output)
}

func (s *Service) checkoutOpsAppCode(app model.OpsApplication, workspace string, branch string) (string, error) {
	repoURL := s.opsAppAuthenticatedRepoURL(app)
	if app.RepoType == "svn" {
		revision := strings.TrimSpace(branch)
		revisionArgs := []string{}
		if revision != "" && !strings.EqualFold(revision, "HEAD") {
			revisionArgs = append(revisionArgs, "-r", revision)
		}
		if _, err := os.Stat(filepath.Join(workspace, ".svn")); err == nil {
			return runOpsAppCommand(workspace, 15*time.Minute, "svn", append([]string{"update"}, revisionArgs...)...)
		}
		args := append([]string{"checkout"}, revisionArgs...)
		args = append(args, repoURL, workspace)
		return runOpsAppCommand(".", 15*time.Minute, "svn", args...)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err == nil {
		output, checkoutErr := runOpsAppCommand(workspace, 15*time.Minute, "git", "checkout", branch)
		pullOutput, pullErr := runOpsAppCommand(workspace, 15*time.Minute, "git", "pull", "--ff-only")
		if checkoutErr != nil {
			return output + pullOutput, checkoutErr
		}
		return output + pullOutput, pullErr
	}
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, repoURL, workspace)
	return runOpsAppCommand(".", 20*time.Minute, "git", args...)
}

func (s *Service) opsAppAuthenticatedRepoURL(app model.OpsApplication) string {
	if app.RepoCredentialID == 0 {
		return app.RepoURL
	}
	var credential model.AssetCredential
	if err := s.db.First(&credential, app.RepoCredentialID).Error; err != nil {
		return app.RepoURL
	}
	parsed, err := url.Parse(app.RepoURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return app.RepoURL
	}
	username := credential.Username
	if username == "" {
		username = "token"
	}
	parsed.User = url.UserPassword(username, credential.Password)
	return parsed.String()
}

func (s *Service) sanitizeOpsAppLog(app model.OpsApplication, value string) string {
	if app.RepoCredentialID == 0 {
		return value
	}
	var credential model.AssetCredential
	if err := s.db.First(&credential, app.RepoCredentialID).Error; err != nil {
		return value
	}
	for _, secret := range []string{credential.Password, credential.Passphrase, credential.PrivateKey} {
		if strings.TrimSpace(secret) != "" {
			value = strings.ReplaceAll(value, secret, "******")
		}
	}
	return value
}

func (s *Service) detectOpsAppCommit(repoType string, workspace string) string {
	if repoType == "svn" {
		output, err := runOpsAppCommand(workspace, 10*time.Second, "svn", "info", "--show-item", "revision")
		if err == nil {
			return strings.TrimSpace(output)
		}
		return ""
	}
	output, err := runOpsAppCommand(workspace, 10*time.Second, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func runOpsAppCommand(dir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return output.String(), fmt.Errorf("%s 执行超时", name)
	}
	return output.String(), err
}

func runOpsAppHealthCheck(rawURL string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	result := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("健康检查返回 HTTP %d", resp.StatusCode)
	}
	return result, nil
}

func runOpsAppShell(script string, dir string, timeout time.Duration) (string, error) {
	return runOpsAppShellWithEnv(script, dir, timeout, nil)
}

func runOpsAppShellWithEnv(script string, dir string, timeout time.Duration, variables map[string]string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", script)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	}
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range variables {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return output.String(), errors.New("脚本执行超时")
	}
	return output.String(), err
}
