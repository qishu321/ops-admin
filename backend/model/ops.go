package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type OpsScript struct {
	ID             uint               `json:"id" gorm:"primaryKey"`
	Name           string             `json:"name" gorm:"size:128;not null;index"`
	ScriptType     string             `json:"scriptType" gorm:"size:32;not null;index"`
	Interpreter    string             `json:"interpreter" gorm:"size:32;not null"`
	Content        string             `json:"content" gorm:"type:longtext"`
	Variables      OpsScriptVariables `json:"variables" gorm:"type:text"`
	TimeoutSeconds int                `json:"timeoutSeconds" gorm:"default:300"`
	Status         int                `json:"status" gorm:"default:1;not null;index"`
	Description    string             `json:"description" gorm:"size:255"`
	CurrentVersion int                `json:"currentVersion" gorm:"default:1"`
	CreatedAt      time.Time          `json:"createTime"`
	UpdatedAt      time.Time          `json:"updateTime"`
}

type OpsScriptVersion struct {
	ID             uint               `json:"id" gorm:"primaryKey"`
	ScriptID       uint               `json:"scriptId" gorm:"index;not null;uniqueIndex:idx_ops_script_version"`
	Version        int                `json:"version" gorm:"not null;index;uniqueIndex:idx_ops_script_version"`
	Content        string             `json:"content" gorm:"type:longtext"`
	Variables      OpsScriptVariables `json:"variables" gorm:"type:text"`
	Interpreter    string             `json:"interpreter" gorm:"size:32"`
	TimeoutSeconds int                `json:"timeoutSeconds"`
	ChangeSummary  string             `json:"changeSummary" gorm:"size:255"`
	Operator       string             `json:"operator" gorm:"size:128;index"`
	CreatedAt      time.Time          `json:"createTime"`
}

// OpsScriptVariable is a declared, task-scoped input. Its runtime name is
// always prefixed with VARIABLE_ before being exported to the remote process.
type OpsScriptVariable struct {
	Name         string `json:"name"`
	DefaultValue string `json:"defaultValue"`
	Description  string `json:"description"`
	Required     bool   `json:"required"`
	Secret       bool   `json:"secret"`
}

type OpsScriptVariables []OpsScriptVariable

// OpsScriptVariableValues stores values assigned to a script's declared
// variables. It is deliberately separate from the declaration list above:
// definitions belong to a script, while values belong to an execution context.
type OpsScriptVariableValues map[string]string

func (variables OpsScriptVariables) Value() (driver.Value, error) {
	if len(variables) == 0 {
		return "[]", nil
	}
	value, err := json.Marshal(variables)
	return string(value), err
}

func (variables *OpsScriptVariables) Scan(value any) error {
	if value == nil {
		*variables = OpsScriptVariables{}
		return nil
	}
	var raw []byte
	switch item := value.(type) {
	case string:
		raw = []byte(item)
	case []byte:
		raw = item
	default:
		return fmt.Errorf("unsupported ops script variables value %T", value)
	}
	if len(raw) == 0 {
		*variables = OpsScriptVariables{}
		return nil
	}
	return json.Unmarshal(raw, variables)
}

func (values OpsScriptVariableValues) Value() (driver.Value, error) {
	if len(values) == 0 {
		return "{}", nil
	}
	value, err := json.Marshal(values)
	return string(value), err
}

func (values *OpsScriptVariableValues) Scan(value any) error {
	if value == nil {
		*values = OpsScriptVariableValues{}
		return nil
	}
	var raw []byte
	switch item := value.(type) {
	case string:
		raw = []byte(item)
	case []byte:
		raw = item
	default:
		return fmt.Errorf("unsupported ops script variable values type %T", value)
	}
	if len(raw) == 0 {
		*values = OpsScriptVariableValues{}
		return nil
	}
	return json.Unmarshal(raw, values)
}

func (OpsScriptVersion) TableName() string { return "ops_script_version" }

func (OpsScript) TableName() string {
	return "ops_script"
}

type OpsExecTask struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	TaskType       string     `json:"taskType" gorm:"size:32;not null;index"`
	Title          string     `json:"title" gorm:"size:255"`
	ScriptID       uint       `json:"scriptId" gorm:"index"`
	ScriptName     string     `json:"scriptName" gorm:"size:128"`
	CommandText    string     `json:"commandText" gorm:"type:longtext"`
	Parameters     string     `json:"parameters" gorm:"type:text"`
	SourceType     string     `json:"sourceType" gorm:"size:32"`
	SourceHostID   uint       `json:"sourceHostId" gorm:"index"`
	SourceHostName string     `json:"sourceHostName" gorm:"size:128"`
	SourcePath     string     `json:"sourcePath" gorm:"size:512"`
	TargetPath     string     `json:"targetPath" gorm:"size:512"`
	FileName       string     `json:"fileName" gorm:"size:255"`
	Concurrency    int        `json:"concurrency" gorm:"default:5"`
	TimeoutSeconds int        `json:"timeoutSeconds" gorm:"default:10"`
	HostCount      int        `json:"hostCount" gorm:"default:0"`
	SuccessCount   int        `json:"successCount" gorm:"default:0"`
	FailedCount    int        `json:"failedCount" gorm:"default:0"`
	Status         string     `json:"status" gorm:"size:32;not null;index"`
	Summary        string     `json:"summary" gorm:"type:text"`
	Operator       string     `json:"operator" gorm:"size:128;index"`
	SourceIP       string     `json:"sourceIp" gorm:"size:64"`
	Source         string     `json:"source" gorm:"size:32;index"`
	RiskLevel      string     `json:"riskLevel" gorm:"size:32;index"`
	ScriptVersion  int        `json:"scriptVersion" gorm:"default:0"`
	TargetSnapshot string     `json:"targetSnapshot" gorm:"type:longtext"`
	RetryOfTaskID  uint       `json:"retryOfTaskId" gorm:"index"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	CreatedAt      time.Time  `json:"createTime"`
	UpdatedAt      time.Time  `json:"updateTime"`
}

func (OpsExecTask) TableName() string {
	return "ops_exec_task"
}

type OpsExecTargetResult struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	TaskID     uint      `json:"taskId" gorm:"index;not null"`
	HostID     uint      `json:"hostId" gorm:"index"`
	HostName   string    `json:"hostName" gorm:"size:128"`
	GroupName  string    `json:"groupName" gorm:"size:128"`
	SSHIP      string    `json:"sshIp" gorm:"size:64"`
	Status     string    `json:"status" gorm:"size:32;not null;index"`
	ExitCode   int       `json:"exitCode" gorm:"default:0"`
	DurationMs int64     `json:"durationMs" gorm:"default:0"`
	Stdout     string    `json:"stdout" gorm:"type:longtext"`
	Stderr     string    `json:"stderr" gorm:"type:longtext"`
	ErrorText  string    `json:"errorText" gorm:"type:text"`
	CreatedAt  time.Time `json:"createTime"`
}

func (OpsExecTargetResult) TableName() string {
	return "ops_exec_target_result"
}

type OpsScheduleTemplate struct {
	ID             uint                    `json:"id" gorm:"primaryKey"`
	Name           string                  `json:"name" gorm:"size:128;not null;index"`
	TaskType       string                  `json:"taskType" gorm:"size:32;not null;index"`
	ScriptID       uint                    `json:"scriptId" gorm:"index"`
	ScriptName     string                  `json:"scriptName" gorm:"size:128"`
	Parameters     string                  `json:"parameters" gorm:"type:text"`
	Variables      OpsScriptVariableValues `json:"variables" gorm:"type:text"`
	HTTPMethod     string                  `json:"httpMethod" gorm:"size:16"`
	URL            string                  `json:"url" gorm:"size:1024"`
	HeadersJSON    string                  `json:"headersJson" gorm:"type:text"`
	Body           string                  `json:"body" gorm:"type:longtext"`
	ExpectedStatus int                     `json:"expectedStatus" gorm:"default:200"`
	TimeoutSeconds int                     `json:"timeoutSeconds" gorm:"default:10"`
	CronExpr       string                  `json:"cronExpr" gorm:"size:128"`
	Description    string                  `json:"description" gorm:"size:255"`
	Status         int                     `json:"status" gorm:"default:1;index"`
	CreatedAt      time.Time               `json:"createTime"`
	UpdatedAt      time.Time               `json:"updateTime"`
}

func (OpsScheduleTemplate) TableName() string {
	return "ops_schedule_template"
}

type OpsScheduleTask struct {
	ID                   uint                    `json:"id" gorm:"primaryKey"`
	Name                 string                  `json:"name" gorm:"size:128;not null;index"`
	TaskType             string                  `json:"taskType" gorm:"size:32;not null;index"`
	TemplateID           uint                    `json:"templateId" gorm:"index"`
	ScriptID             uint                    `json:"scriptId" gorm:"index"`
	ScriptName           string                  `json:"scriptName" gorm:"size:128"`
	Parameters           string                  `json:"parameters" gorm:"type:text"`
	Variables            OpsScriptVariableValues `json:"variables" gorm:"type:text"`
	HostIDsJSON          string                  `json:"hostIdsJson" gorm:"type:text"`
	GroupIDsJSON         string                  `json:"groupIdsJson" gorm:"type:text"`
	Concurrency          int                     `json:"concurrency" gorm:"default:5"`
	HTTPMethod           string                  `json:"httpMethod" gorm:"size:16"`
	URL                  string                  `json:"url" gorm:"size:1024"`
	HeadersJSON          string                  `json:"headersJson" gorm:"type:text"`
	Body                 string                  `json:"body" gorm:"type:longtext"`
	ExpectedStatus       int                     `json:"expectedStatus" gorm:"default:200"`
	TimeoutSeconds       int                     `json:"timeoutSeconds" gorm:"default:10"`
	RetryEnabled         bool                    `json:"retryEnabled" gorm:"default:false"`
	MaxRetries           int                     `json:"maxRetries" gorm:"default:0"`
	RetryIntervalSeconds int                     `json:"retryIntervalSeconds" gorm:"default:5"`
	RetryBackoff         string                  `json:"retryBackoff" gorm:"size:16;default:exponential"`
	AllowUnsafeRetry     bool                    `json:"allowUnsafeRetry" gorm:"default:false"`
	CronExpr             string                  `json:"cronExpr" gorm:"size:128;not null"`
	Description          string                  `json:"description" gorm:"size:255"`
	Status               int                     `json:"status" gorm:"default:1;index"`
	NotifyEnabled        bool                    `json:"notifyEnabled" gorm:"default:false;index"`
	NotifyRuleID         uint                    `json:"notifyRuleId" gorm:"index"`
	NotifyOnFailureOnly  bool                    `json:"notifyOnFailureOnly" gorm:"default:false"`
	LastStatus           string                  `json:"lastStatus" gorm:"size:32"`
	LastSummary          string                  `json:"lastSummary" gorm:"type:text"`
	LastRunAt            *time.Time              `json:"lastRunAt"`
	NextRunAt            *time.Time              `json:"nextRunAt"`
	CreatedAt            time.Time               `json:"createTime"`
	UpdatedAt            time.Time               `json:"updateTime"`
}

func (OpsScheduleTask) TableName() string {
	return "ops_schedule_task"
}

type OpsScheduleTaskLog struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	TaskID         uint       `json:"taskId" gorm:"index;not null"`
	TaskName       string     `json:"taskName" gorm:"size:128"`
	TaskType       string     `json:"taskType" gorm:"size:32;index"`
	TriggerType    string     `json:"triggerType" gorm:"size:32"`
	Status         string     `json:"status" gorm:"size:32;index"`
	Summary        string     `json:"summary" gorm:"type:text"`
	Detail         string     `json:"detail" gorm:"type:longtext"`
	ExecTaskID     uint       `json:"execTaskId" gorm:"index"`
	ExpectedStatus int        `json:"expectedStatus"`
	ActualStatus   int        `json:"actualStatus"`
	ResponseBody   string     `json:"responseBody" gorm:"type:longtext"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	DurationMs     int64      `json:"durationMs" gorm:"default:0"`
	AttemptCount   int        `json:"attemptCount" gorm:"default:1"`
	CreatedAt      time.Time  `json:"createTime"`
}

func (OpsScheduleTaskLog) TableName() string {
	return "ops_schedule_task_log"
}

type OpsJobTemplate struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"size:128;not null;index"`
	Description    string    `json:"description" gorm:"size:255"`
	Status         int       `json:"status" gorm:"default:1;index"`
	GraphJSON      string    `json:"graphJson" gorm:"type:longtext"`
	DefinitionJSON string    `json:"definitionJson" gorm:"type:longtext"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (OpsJobTemplate) TableName() string {
	return "ops_job_template"
}

type OpsJob struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"size:128;not null;index"`
	Description    string    `json:"description" gorm:"size:255"`
	Status         int       `json:"status" gorm:"default:1;index"`
	TemplateID     uint      `json:"templateId" gorm:"index"`
	NotifyEnabled  bool      `json:"notifyEnabled" gorm:"default:false;index"`
	NotifyRuleID   uint      `json:"notifyRuleId" gorm:"index"`
	GraphJSON      string    `json:"graphJson" gorm:"type:longtext"`
	DefinitionJSON string    `json:"definitionJson" gorm:"type:longtext"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (OpsJob) TableName() string {
	return "ops_job"
}

type OpsJobHistory struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	JobID           uint       `json:"jobId" gorm:"index;not null"`
	JobName         string     `json:"jobName" gorm:"size:128"`
	TriggerType     string     `json:"triggerType" gorm:"size:32"`
	Status          string     `json:"status" gorm:"size:32;index"`
	Summary         string     `json:"summary" gorm:"type:text"`
	CurrentStepID   string     `json:"currentStepId" gorm:"size:64"`
	CurrentStepName string     `json:"currentStepName" gorm:"size:128"`
	DefinitionJSON  string     `json:"definitionJson" gorm:"type:longtext"`
	StartedAt       *time.Time `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt"`
	CreatedAt       time.Time  `json:"createTime"`
}

func (OpsJobHistory) TableName() string {
	return "ops_job_history"
}

type OpsJobHistoryStep struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	HistoryID        uint       `json:"historyId" gorm:"index;not null"`
	StepID           string     `json:"stepId" gorm:"size:64;index"`
	StepName         string     `json:"stepName" gorm:"size:128"`
	StepType         string     `json:"stepType" gorm:"size:32;index"`
	Status           string     `json:"status" gorm:"size:32;index"`
	Summary          string     `json:"summary" gorm:"type:text"`
	Output           string     `json:"output" gorm:"type:longtext"`
	ExecTaskID       uint       `json:"execTaskId" gorm:"index"`
	ApprovalDecision string     `json:"approvalDecision" gorm:"size:32"`
	ApprovalNote     string     `json:"approvalNote" gorm:"type:text"`
	StartedAt        *time.Time `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
	DurationMs       int64      `json:"durationMs" gorm:"default:0"`
	CreatedAt        time.Time  `json:"createTime"`
}

func (OpsJobHistoryStep) TableName() string {
	return "ops_job_history_step"
}

type OpsEnvironment struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	Code        string    `json:"code" gorm:"size:64;not null;uniqueIndex"`
	Sort        int       `json:"sort" gorm:"default:0;index"`
	Status      int       `json:"status" gorm:"default:1;index"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (OpsEnvironment) TableName() string {
	return "ops_environment"
}

type OpsApplication struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	Name             string     `json:"name" gorm:"size:128;not null;index"`
	Code             string     `json:"code" gorm:"size:128;not null;uniqueIndex"`
	ServiceType      string     `json:"serviceType" gorm:"size:64;index"`
	RepoType         string     `json:"repoType" gorm:"size:16;not null;index"`
	RepoURL          string     `json:"repoUrl" gorm:"size:1024;not null"`
	RepoCredentialID uint       `json:"repoCredentialId" gorm:"index"`
	Branch           string     `json:"branch" gorm:"size:128"`
	Workspace        string     `json:"workspace" gorm:"size:512"`
	BuildScript      string     `json:"buildScript" gorm:"type:longtext"`
	DeployScript     string     `json:"deployScript" gorm:"type:longtext"`
	Env              string     `json:"env" gorm:"size:64;index"`
	Status           int        `json:"status" gorm:"default:1;index"`
	Description      string     `json:"description" gorm:"size:255"`
	LastReleaseID    uint       `json:"lastReleaseId" gorm:"index"`
	LastStatus       string     `json:"lastStatus" gorm:"size:32"`
	LastReleasedAt   *time.Time `json:"lastReleasedAt"`
	CreatedAt        time.Time  `json:"createTime"`
	UpdatedAt        time.Time  `json:"updateTime"`
}

func (OpsApplication) TableName() string {
	return "ops_application"
}

type OpsApplicationEnvironmentBinding struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	AppID               uint      `json:"appId" gorm:"index;not null;uniqueIndex:idx_ops_app_env_binding"`
	Env                 string    `json:"env" gorm:"size:64;not null;index;uniqueIndex:idx_ops_app_env_binding"`
	HostGroupID         uint      `json:"hostGroupId" gorm:"index"`
	K8sClusterID        uint      `json:"k8sClusterId" gorm:"index"`
	Namespace           string    `json:"namespace" gorm:"size:128"`
	WorkloadType        string    `json:"workloadType" gorm:"size:32"`
	WorkloadName        string    `json:"workloadName" gorm:"size:128"`
	DatabaseID          uint      `json:"databaseId" gorm:"index"`
	MonitorDatasourceID uint      `json:"monitorDatasourceId" gorm:"index"`
	GatewayID           uint      `json:"gatewayId" gorm:"index"`
	Status              int       `json:"status" gorm:"default:1;index"`
	CreatedAt           time.Time `json:"createTime"`
	UpdatedAt           time.Time `json:"updateTime"`
}

func (OpsApplicationEnvironmentBinding) TableName() string { return "ops_application_env_binding" }

type OpsAppBuildTask struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	Name            string     `json:"name" gorm:"size:128;not null;index"`
	AppID           uint       `json:"appId" gorm:"index;not null"`
	AppName         string     `json:"appName" gorm:"size:128;index"`
	AppCode         string     `json:"appCode" gorm:"size:128;index"`
	Env             string     `json:"env" gorm:"size:64;index"`
	Branch          string     `json:"branch" gorm:"size:128"`
	BuildScript     string     `json:"buildScript" gorm:"type:longtext"`
	DeployScript    string     `json:"deployScript" gorm:"type:longtext"`
	BuildParamsJSON string     `json:"buildParamsJson" gorm:"type:longtext"`
	RunnerType      string     `json:"runnerType" gorm:"size:32;default:local;index"`
	RunnerHostID    uint       `json:"runnerHostId" gorm:"index"`
	ExecutionPath   string     `json:"executionPath" gorm:"size:1024"`
	ArtifactType    string     `json:"artifactType" gorm:"size:32;default:file;index"`
	ArtifactPath    string     `json:"artifactPath" gorm:"size:1024"`
	TimeoutSeconds  int        `json:"timeoutSeconds" gorm:"default:1800"`
	Status          int        `json:"status" gorm:"default:1;index"`
	Description     string     `json:"description" gorm:"size:255"`
	LastReleaseID   uint       `json:"lastReleaseId" gorm:"index"`
	LastStatus      string     `json:"lastStatus" gorm:"size:32"`
	LastRunAt       *time.Time `json:"lastRunAt"`
	SuccessCount    int        `json:"successCount" gorm:"default:0"`
	FailedCount     int        `json:"failedCount" gorm:"default:0"`
	CreatedAt       time.Time  `json:"createTime"`
	UpdatedAt       time.Time  `json:"updateTime"`
}

func (OpsAppBuildTask) TableName() string {
	return "ops_app_build_task"
}

type OpsAppRelease struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	AppID         uint       `json:"appId" gorm:"index;not null"`
	AppName       string     `json:"appName" gorm:"size:128;index"`
	AppCode       string     `json:"appCode" gorm:"size:128;index"`
	BuildTaskID   uint       `json:"buildTaskId" gorm:"index"`
	BuildTaskName string     `json:"buildTaskName" gorm:"size:128;index"`
	Env           string     `json:"env" gorm:"size:64;index"`
	Version       string     `json:"version" gorm:"size:128;index"`
	RepoType      string     `json:"repoType" gorm:"size:16"`
	RepoURL       string     `json:"repoUrl" gorm:"size:1024"`
	Branch        string     `json:"branch" gorm:"size:128"`
	CommitID      string     `json:"commitId" gorm:"size:128"`
	Workspace     string     `json:"workspace" gorm:"size:512"`
	Status        string     `json:"status" gorm:"size:32;not null;index"`
	Stage         string     `json:"stage" gorm:"size:32;index"`
	Summary       string     `json:"summary" gorm:"type:text"`
	BuildLog      string     `json:"buildLog" gorm:"type:longtext"`
	DeployLog     string     `json:"deployLog" gorm:"type:longtext"`
	ParamsJSON    string     `json:"paramsJson" gorm:"type:longtext"`
	StartedAt     *time.Time `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
	DurationMs    int64      `json:"durationMs" gorm:"default:0"`
	CreatedAt     time.Time  `json:"createTime"`
	UpdatedAt     time.Time  `json:"updateTime"`
}

func (OpsAppRelease) TableName() string {
	return "ops_app_release"
}

type OpsAppArtifact struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AppID       uint      `json:"appId" gorm:"index;not null"`
	AppName     string    `json:"appName" gorm:"size:128;index"`
	BuildTaskID uint      `json:"buildTaskId" gorm:"index"`
	ReleaseID   uint      `json:"releaseId" gorm:"index"`
	Env         string    `json:"env" gorm:"size:64;index"`
	Version     string    `json:"version" gorm:"size:128;index"`
	CommitID    string    `json:"commitId" gorm:"size:128"`
	Type        string    `json:"type" gorm:"size:32;index"`
	URI         string    `json:"uri" gorm:"size:1024"`
	Digest      string    `json:"digest" gorm:"size:255"`
	Status      string    `json:"status" gorm:"size:32;index"`
	CreatedAt   time.Time `json:"createTime"`
}

func (OpsAppArtifact) TableName() string { return "ops_app_artifact" }

// OpsImageRegistry stores the destination used by CI/CD image build and push stages.
// Password is intentionally excluded from API responses.
type OpsImageRegistry struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	Address     string    `json:"address" gorm:"size:512;not null"`
	Namespace   string    `json:"namespace" gorm:"size:255"`
	Username    string    `json:"username" gorm:"size:255"`
	Password    string    `json:"-" gorm:"type:text"`
	Status      int       `json:"status" gorm:"default:1;index"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (OpsImageRegistry) TableName() string { return "ops_image_registry" }

// OpsAppPipelineTemplate stores user-defined reusable pipeline stage blueprints.
// Built-in templates remain code-owned; custom templates are persisted here.
type OpsAppPipelineTemplate struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"size:128;not null;uniqueIndex"`
	Category       string    `json:"category" gorm:"size:64;index"`
	TechStack      string    `json:"techStack" gorm:"size:64;index"`
	Description    string    `json:"description" gorm:"size:255"`
	StageCount     int       `json:"stageCount" gorm:"default:0"`
	DefinitionJSON string    `json:"definitionJson" gorm:"type:longtext"`
	Status         int       `json:"status" gorm:"default:1;index"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (OpsAppPipelineTemplate) TableName() string { return "ops_app_pipeline_template" }

type OpsAppPipeline struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Name          string `json:"name" gorm:"size:128;not null;index"`
	AppID         uint   `json:"appId" gorm:"index;not null"`
	AppName       string `json:"appName" gorm:"size:128;index"`
	AppCode       string `json:"appCode" gorm:"size:128;index"`
	RepoType      string `json:"repoType" gorm:"size:16;index"`
	RepoURL       string `json:"repoUrl" gorm:"size:1024"`
	DefaultBranch string `json:"defaultBranch" gorm:"size:128"`
	Env           string `json:"env" gorm:"size:64;index"`
	TechStack     string `json:"techStack" gorm:"size:64;index"`
	TemplateID    uint   `json:"templateId" gorm:"index"`
	BuildTaskID   uint   `json:"buildTaskId" gorm:"index"`
	// ExecutorHostID is the SSH host used to execute checkout/build/image/deploy commands.
	// Pipelines must never silently execute these commands inside the Ops Admin container.
	ExecutorHostID uint       `json:"executorHostId" gorm:"index"`
	StageCount     int        `json:"stageCount" gorm:"default:0"`
	Status         int        `json:"status" gorm:"default:1;index"`
	Description    string     `json:"description" gorm:"size:255"`
	DefinitionJSON string     `json:"definitionJson" gorm:"type:longtext"`
	LastRunID      uint       `json:"lastRunId" gorm:"index"`
	LastStatus     string     `json:"lastStatus" gorm:"size:32"`
	LastRunAt      *time.Time `json:"lastRunAt"`
	CreatedAt      time.Time  `json:"createTime"`
	UpdatedAt      time.Time  `json:"updateTime"`
}

func (OpsAppPipeline) TableName() string {
	return "ops_app_pipeline"
}

type OpsAppPipelineRun struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	PipelineID     uint       `json:"pipelineId" gorm:"index;not null"`
	PipelineName   string     `json:"pipelineName" gorm:"size:128;index"`
	AppID          uint       `json:"appId" gorm:"index"`
	AppName        string     `json:"appName" gorm:"size:128;index"`
	AppCode        string     `json:"appCode" gorm:"size:128;index"`
	Env            string     `json:"env" gorm:"size:64;index"`
	Branch         string     `json:"branch" gorm:"size:128"`
	ImageTag       string     `json:"imageTag" gorm:"size:128;index"`
	ArtifactID     uint       `json:"artifactId" gorm:"index"`
	ExecutorHostID uint       `json:"executorHostId" gorm:"index"`
	ApprovalStatus string     `json:"approvalStatus" gorm:"size:32;default:not_required;index"`
	Approver       string     `json:"approver" gorm:"size:128"`
	ApprovalNote   string     `json:"approvalNote" gorm:"type:text"`
	TriggerType    string     `json:"triggerType" gorm:"size:32;index"`
	TriggerUser    string     `json:"triggerUser" gorm:"size:128"`
	Status         string     `json:"status" gorm:"size:32;not null;index"`
	Summary        string     `json:"summary" gorm:"type:text"`
	ParamsJSON     string     `json:"paramsJson" gorm:"type:text"`
	DefinitionJSON string     `json:"definitionJson" gorm:"type:longtext"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	DurationMs     int64      `json:"durationMs" gorm:"default:0"`
	CreatedAt      time.Time  `json:"createTime"`
	UpdatedAt      time.Time  `json:"updateTime"`
}

func (OpsAppPipelineRun) TableName() string {
	return "ops_app_pipeline_run"
}

type OpsAppPipelineRunStage struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	RunID      uint       `json:"runId" gorm:"index;not null"`
	StageID    string     `json:"stageId" gorm:"size:64;index"`
	StageName  string     `json:"stageName" gorm:"size:128;index"`
	StageType  string     `json:"stageType" gorm:"size:64;index"`
	Status     string     `json:"status" gorm:"size:32;not null;index"`
	Summary    string     `json:"summary" gorm:"type:text"`
	Log        string     `json:"log" gorm:"type:longtext"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	DurationMs int64      `json:"durationMs" gorm:"default:0"`
	CreatedAt  time.Time  `json:"createTime"`
}

func (OpsAppPipelineRunStage) TableName() string {
	return "ops_app_pipeline_run_stage"
}

type NotifyTemplate struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	ChannelType string    `json:"channelType" gorm:"size:32;not null;index"`
	Scope       string    `json:"scope" gorm:"size:32;not null;default:all;index"`
	Title       string    `json:"title" gorm:"size:255"`
	Content     string    `json:"content" gorm:"type:longtext"`
	Status      int       `json:"status" gorm:"default:1;index"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (NotifyTemplate) TableName() string {
	return "notify_template"
}

type NotifyChannel struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	ChannelType string    `json:"channelType" gorm:"size:32;not null;index"`
	WebhookURL  string    `json:"webhookUrl" gorm:"size:1024;not null"`
	Secret      string    `json:"secret" gorm:"size:255"`
	HeadersJSON string    `json:"headersJson" gorm:"type:text"`
	Status      int       `json:"status" gorm:"default:1;index"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (NotifyChannel) TableName() string {
	return "notify_channel"
}

type NotifyRule struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"size:128;not null;index"`
	Scope          string    `json:"scope" gorm:"size:32;not null;index"`
	EventsJSON     string    `json:"eventsJson" gorm:"type:text"`
	TemplateID     uint      `json:"templateId" gorm:"index;not null"`
	ChannelIDsJSON string    `json:"channelIdsJson" gorm:"type:text"`
	Status         int       `json:"status" gorm:"default:1;index"`
	Description    string    `json:"description" gorm:"size:255"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (NotifyRule) TableName() string {
	return "notify_rule"
}

type NotifySendLog struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	DeliveryID    string     `json:"deliveryId" gorm:"size:64;index"`
	RuleID        uint       `json:"ruleId" gorm:"index"`
	RuleName      string     `json:"ruleName" gorm:"size:128"`
	ChannelID     uint       `json:"channelId" gorm:"index"`
	ChannelName   string     `json:"channelName" gorm:"size:128"`
	ChannelType   string     `json:"channelType" gorm:"size:32;index"`
	Event         string     `json:"event" gorm:"size:64;index"`
	Scope         string     `json:"scope" gorm:"size:32;index"`
	TargetID      uint       `json:"targetId" gorm:"index"`
	TargetName    string     `json:"targetName" gorm:"size:128"`
	Summary       string     `json:"summary" gorm:"size:512"`
	Status        string     `json:"status" gorm:"size:32;index"`
	AttemptCount  int        `json:"attemptCount" gorm:"default:0"`
	MaxAttempts   int        `json:"maxAttempts" gorm:"default:3"`
	NextRetryAt   *time.Time `json:"nextRetryAt" gorm:"index"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	DurationMs    int64      `json:"durationMs" gorm:"default:0"`
	HTTPStatus    int        `json:"httpStatus" gorm:"default:0"`
	BusinessCode  string     `json:"businessCode" gorm:"size:64"`
	RetryOfID     uint       `json:"retryOfId" gorm:"index"`
	RequestBody   string     `json:"requestBody" gorm:"type:longtext"`
	Response      string     `json:"response" gorm:"type:longtext"`
	ErrorText     string     `json:"errorText" gorm:"type:text"`
	CreatedAt     time.Time  `json:"createTime"`
	UpdatedAt     time.Time  `json:"updateTime"`
}

func (NotifySendLog) TableName() string {
	return "notify_send_log"
}
