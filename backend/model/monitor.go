package model

import "time"

type MonitorDatasource struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	Name                 string     `json:"name" gorm:"size:128;not null;index"`
	Type                 string     `json:"type" gorm:"size:32;not null;index"`
	URL                  string     `json:"url" gorm:"size:1024;not null"`
	AuthType             string     `json:"authType" gorm:"size:32;default:none"`
	Username             string     `json:"username" gorm:"size:128"`
	Password             string     `json:"password" gorm:"size:255"`
	Token                string     `json:"token" gorm:"type:text"`
	IsDefault            bool       `json:"isDefault" gorm:"default:false;index"`
	Env                  string     `json:"env" gorm:"size:64;index"`
	Status               int        `json:"status" gorm:"default:1;index"`
	HealthStatus         string     `json:"healthStatus" gorm:"size:32;default:unknown;index"`
	LastCheckAt          *time.Time `json:"lastCheckAt"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt"`
	LatencyMs            int64      `json:"latencyMs" gorm:"default:0"`
	ConsecutiveFailures  int        `json:"consecutiveFailures" gorm:"default:0"`
	ConsecutiveSuccesses int        `json:"consecutiveSuccesses" gorm:"default:0"`
	LastError            string     `json:"lastError" gorm:"type:text"`
	Description          string     `json:"description" gorm:"size:255"`
	CreatedAt            time.Time  `json:"createTime"`
	UpdatedAt            time.Time  `json:"updateTime"`
}

func (MonitorDatasource) TableName() string {
	return "monitor_datasource"
}

type MonitorLogShortcut struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Owner          string    `json:"owner" gorm:"size:128;not null;index"`
	DatasourceType string    `json:"datasourceType" gorm:"size:32;default:elasticsearch;index"`
	Name           string    `json:"name" gorm:"size:128;not null"`
	Query          string    `json:"query" gorm:"type:text"`
	IndexName      string    `json:"indexName" gorm:"size:255"`
	TimeRange      string    `json:"timeRange" gorm:"size:32;default:24h"`
	Sort           int       `json:"sort" gorm:"default:0;index"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (MonitorLogShortcut) TableName() string {
	return "monitor_log_shortcut"
}

type MonitorAlertRule struct {
	ID                          uint       `json:"id" gorm:"primaryKey"`
	Name                        string     `json:"name" gorm:"size:128;not null;index"`
	AlertType                   string     `json:"alertType" gorm:"size:32;default:metric;index"`
	DatasourceScope             string     `json:"datasourceScope" gorm:"size:32;default:specific;index"`
	DatasourceID                uint       `json:"datasourceId" gorm:"index;not null"`
	DatasourceName              string     `json:"datasourceName" gorm:"size:128"`
	PromQL                      string     `json:"promql" gorm:"type:longtext;not null"`
	LogIndex                    string     `json:"logIndex" gorm:"size:255"`
	LogTimeRangeSeconds         int        `json:"logTimeRangeSeconds" gorm:"default:300"`
	Comparator                  string     `json:"comparator" gorm:"size:16;not null"`
	Threshold                   float64    `json:"threshold"`
	ForSeconds                  int        `json:"forSeconds" gorm:"default:60"`
	EvalIntervalSeconds         int        `json:"evalIntervalSeconds" gorm:"default:60"`
	NotifyRepeatIntervalSeconds int        `json:"notifyRepeatIntervalSeconds" gorm:"default:1800"`
	MaxNotifyCount              int        `json:"maxNotifyCount" gorm:"default:0"`
	Severity                    string     `json:"severity" gorm:"size:16;default:P2;index"`
	LabelsJSON                  string     `json:"labelsJson" gorm:"type:text"`
	AnnotationsJSON             string     `json:"annotationsJson" gorm:"type:text"`
	NotifyEnabled               bool       `json:"notifyEnabled" gorm:"default:false;index"`
	NotifyRuleID                uint       `json:"notifyRuleId" gorm:"index"`
	NotifyRecoveryEnabled       bool       `json:"notifyRecoveryEnabled" gorm:"default:true"`
	Env                         string     `json:"env" gorm:"size:64;index"`
	Status                      int        `json:"status" gorm:"default:1;index"`
	LastEvalAt                  *time.Time `json:"lastEvalAt"`
	LastEvalStatus              string     `json:"lastEvalStatus" gorm:"size:32"`
	LastEvalMessage             string     `json:"lastEvalMessage" gorm:"type:text"`
	Description                 string     `json:"description" gorm:"size:255"`
	CreatedAt                   time.Time  `json:"createTime"`
	UpdatedAt                   time.Time  `json:"updateTime"`
}

func (MonitorAlertRule) TableName() string {
	return "monitor_alert_rule"
}

// MonitorAlertTemplate stores reusable alert definitions. A template is never
// scheduled directly; it becomes an executable alert only after it is applied
// to a concrete datasource in an alert rule.
type MonitorAlertTemplate struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	GroupID             uint      `json:"groupId" gorm:"index"`
	Name                string    `json:"name" gorm:"size:128;not null;index"`
	Category            string    `json:"category" gorm:"size:64;index"`
	Collector           string    `json:"collector" gorm:"size:128;index"`
	ObjectType          string    `json:"objectType" gorm:"size:64;index"`
	DatasourceType      string    `json:"datasourceType" gorm:"size:32;index"`
	QueryText           string    `json:"queryText" gorm:"type:longtext;not null"`
	Comparator          string    `json:"comparator" gorm:"size:16;default:>"`
	Threshold           float64   `json:"threshold"`
	ForSeconds          int       `json:"forSeconds" gorm:"default:300"`
	EvalIntervalSeconds int       `json:"evalIntervalSeconds" gorm:"default:60"`
	Severity            string    `json:"severity" gorm:"size:16;default:P2;index"`
	LabelsJSON          string    `json:"labelsJson" gorm:"type:text"`
	AnnotationsJSON     string    `json:"annotationsJson" gorm:"type:text"`
	Description         string    `json:"description" gorm:"size:255"`
	Source              string    `json:"source" gorm:"size:32;default:custom;index"`
	Status              int       `json:"status" gorm:"default:1;index"`
	CreatedAt           time.Time `json:"createTime"`
	UpdatedAt           time.Time `json:"updateTime"`
}

func (MonitorAlertTemplate) TableName() string {
	return "monitor_alert_template"
}

type MonitorAlertTemplateGroup struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ParentID  uint      `json:"parentId" gorm:"index"`
	Name      string    `json:"name" gorm:"size:128;not null;index"`
	Sort      int       `json:"sort" gorm:"default:0;index"`
	CreatedAt time.Time `json:"createTime"`
	UpdatedAt time.Time `json:"updateTime"`
}

func (MonitorAlertTemplateGroup) TableName() string { return "monitor_alert_template_group" }

type MonitorAlertEvent struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	RuleID            uint       `json:"ruleId" gorm:"index;not null"`
	RuleName          string     `json:"ruleName" gorm:"size:128"`
	DatasourceID      uint       `json:"datasourceId" gorm:"index"`
	DatasourceName    string     `json:"datasourceName" gorm:"size:128"`
	Fingerprint       string     `json:"fingerprint" gorm:"size:128;index"`
	Severity          string     `json:"severity" gorm:"size:16;index"`
	Status            string     `json:"status" gorm:"size:32;index"`
	Metric            string     `json:"metric" gorm:"size:255"`
	LabelsJSON        string     `json:"labelsJson" gorm:"type:text"`
	AnnotationsJSON   string     `json:"annotationsJson" gorm:"type:text"`
	CurrentValue      float64    `json:"currentValue"`
	Threshold         float64    `json:"threshold"`
	Summary           string     `json:"summary" gorm:"type:text"`
	Silenced          bool       `json:"silenced" gorm:"default:false;index"`
	SilenceRuleID     uint       `json:"silenceRuleId" gorm:"index"`
	SilenceRuleName   string     `json:"silenceRuleName" gorm:"size:128"`
	AggregationKey    string     `json:"aggregationKey" gorm:"size:255;index"`
	AggregateRuleID   uint       `json:"aggregateRuleId" gorm:"index"`
	AggregateRuleName string     `json:"aggregateRuleName" gorm:"size:128"`
	LastNotifyAt      *time.Time `json:"lastNotifyAt"`
	NotifyCount       int        `json:"notifyCount" gorm:"default:0"`
	FirstTriggerAt    time.Time  `json:"firstTriggerAt"`
	LastTriggerAt     time.Time  `json:"lastTriggerAt"`
	RecoveredAt       *time.Time `json:"recoveredAt"`
	ClaimedBy         string     `json:"claimedBy" gorm:"size:128"`
	ClaimedAt         *time.Time `json:"claimedAt"`
	HandleNote        string     `json:"handleNote" gorm:"type:text"`
	ResolveNote       string     `json:"resolveNote" gorm:"type:text"`
	ResolvedAt        *time.Time `json:"resolvedAt"`
	CreatedAt         time.Time  `json:"createTime"`
	UpdatedAt         time.Time  `json:"updateTime"`
}

func (MonitorAlertEvent) TableName() string {
	return "monitor_alert_event"
}

type MonitorAlertEventTimeline struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	AlertEventID uint      `json:"alertEventId" gorm:"index;not null"`
	EventType    string    `json:"eventType" gorm:"size:32;not null;index"`
	Title        string    `json:"title" gorm:"size:128;not null"`
	Detail       string    `json:"detail" gorm:"type:text"`
	Operator     string    `json:"operator" gorm:"size:128"`
	MetadataJSON string    `json:"metadataJson" gorm:"type:text"`
	CreatedAt    time.Time `json:"createTime"`
}

func (MonitorAlertEventTimeline) TableName() string {
	return "monitor_alert_event_timeline"
}

type MonitorAlertAction struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	AlertEventID uint      `json:"alertEventId" gorm:"index;not null"`
	RuleName     string    `json:"ruleName" gorm:"size:128;index"`
	ActionType   string    `json:"actionType" gorm:"size:32;index"`
	TargetID     uint      `json:"targetId" gorm:"index"`
	TargetName   string    `json:"targetName" gorm:"size:128"`
	Status       string    `json:"status" gorm:"size:32;index"`
	Operator     string    `json:"operator" gorm:"size:128"`
	Summary      string    `json:"summary" gorm:"type:text"`
	Result       string    `json:"result" gorm:"type:longtext"`
	CreatedAt    time.Time `json:"createTime"`
	UpdatedAt    time.Time `json:"updateTime"`
}

func (MonitorAlertAction) TableName() string {
	return "monitor_alert_action"
}

type MonitorSilenceRule struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	Name            string     `json:"name" gorm:"size:128;not null;index"`
	MatchMode       string     `json:"matchMode" gorm:"size:32;default:regex;index"`
	RuleIDsJSON     string     `json:"ruleIdsJson" gorm:"type:text"`
	RuleNamePattern string     `json:"ruleNamePattern" gorm:"size:128;index"`
	Severity        string     `json:"severity" gorm:"size:16;index"`
	AlertType       string     `json:"alertType" gorm:"size:32;index"`
	MatchersJSON    string     `json:"matchersJson" gorm:"type:text"`
	StartsAt        *time.Time `json:"startsAt"`
	EndsAt          *time.Time `json:"endsAt"`
	Priority        int        `json:"priority" gorm:"default:100;index"`
	Status          int        `json:"status" gorm:"default:1;index"`
	Description     string     `json:"description" gorm:"size:255"`
	CreatedAt       time.Time  `json:"createTime"`
	UpdatedAt       time.Time  `json:"updateTime"`
}

func (MonitorSilenceRule) TableName() string {
	return "monitor_silence_rule"
}

type MonitorAggregationRule struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	Name                  string    `json:"name" gorm:"size:128;not null;index"`
	MatchMode             string    `json:"matchMode" gorm:"size:32;default:regex;index"`
	RuleIDsJSON           string    `json:"ruleIdsJson" gorm:"type:text"`
	RuleNamePattern       string    `json:"ruleNamePattern" gorm:"size:128;index"`
	Severity              string    `json:"severity" gorm:"size:16;index"`
	AlertType             string    `json:"alertType" gorm:"size:32;index"`
	GroupByJSON           string    `json:"groupByJson" gorm:"type:text"`
	WindowSeconds         int       `json:"windowSeconds" gorm:"default:300"`
	RepeatIntervalSeconds int       `json:"repeatIntervalSeconds" gorm:"default:1800"`
	Status                int       `json:"status" gorm:"default:1;index"`
	Description           string    `json:"description" gorm:"size:255"`
	CreatedAt             time.Time `json:"createTime"`
	UpdatedAt             time.Time `json:"updateTime"`
}

func (MonitorAggregationRule) TableName() string {
	return "monitor_aggregation_rule"
}

type MonitorQueryHistory struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	DatasourceID   uint      `json:"datasourceId" gorm:"index"`
	DatasourceName string    `json:"datasourceName" gorm:"size:128"`
	Query          string    `json:"query" gorm:"type:longtext"`
	QueryType      string    `json:"queryType" gorm:"size:32"`
	Status         string    `json:"status" gorm:"size:32;index"`
	ErrorText      string    `json:"errorText" gorm:"type:text"`
	CreatedAt      time.Time `json:"createTime"`
}

func (MonitorQueryHistory) TableName() string {
	return "monitor_query_history"
}

type MonitorDashboard struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	Layout      string    `json:"layout" gorm:"size:32;default:grid"`
	Status      int       `json:"status" gorm:"default:1;index"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (MonitorDashboard) TableName() string {
	return "monitor_dashboard"
}

type MonitorDashboardPanel struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	DashboardID    uint      `json:"dashboardId" gorm:"index;not null"`
	Title          string    `json:"title" gorm:"size:128;not null"`
	DatasourceID   uint      `json:"datasourceId" gorm:"index;not null"`
	DatasourceName string    `json:"datasourceName" gorm:"size:128"`
	PromQL         string    `json:"promql" gorm:"type:longtext;not null"`
	Unit           string    `json:"unit" gorm:"size:32"`
	ChartType      string    `json:"chartType" gorm:"size:32;default:stat"`
	Span           int       `json:"span" gorm:"default:8"`
	Sort           int       `json:"sort" gorm:"default:0;index"`
	Status         int       `json:"status" gorm:"default:1;index"`
	Description    string    `json:"description" gorm:"size:255"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (MonitorDashboardPanel) TableName() string {
	return "monitor_dashboard_panel"
}
