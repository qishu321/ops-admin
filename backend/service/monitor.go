package service

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type MonitorDatasourcePayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	AuthType    string `json:"authType"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Token       string `json:"token"`
	IsDefault   bool   `json:"isDefault"`
	Env         string `json:"env"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type MonitorAlertRulePayload struct {
	ID                          uint    `json:"id"`
	Name                        string  `json:"name"`
	AlertType                   string  `json:"alertType"`
	DatasourceScope             string  `json:"datasourceScope"`
	DatasourceID                uint    `json:"datasourceId"`
	PromQL                      string  `json:"promql"`
	Query                       string  `json:"query"`
	LogIndex                    string  `json:"logIndex"`
	LogTimeRangeSeconds         int     `json:"logTimeRangeSeconds"`
	Comparator                  string  `json:"comparator"`
	Threshold                   float64 `json:"threshold"`
	ForSeconds                  int     `json:"forSeconds"`
	EvalIntervalSeconds         int     `json:"evalIntervalSeconds"`
	NotifyRepeatIntervalSeconds int     `json:"notifyRepeatIntervalSeconds"`
	MaxNotifyCount              int     `json:"maxNotifyCount"`
	Severity                    string  `json:"severity"`
	LabelsJSON                  string  `json:"labelsJson"`
	AnnotationsJSON             string  `json:"annotationsJson"`
	NotifyEnabled               bool    `json:"notifyEnabled"`
	NotifyRuleID                uint    `json:"notifyRuleId"`
	NotifyRecoveryEnabled       bool    `json:"notifyRecoveryEnabled"`
	Env                         string  `json:"env"`
	Status                      int     `json:"status"`
	Description                 string  `json:"description"`
}

type MonitorAlertTemplatePayload struct {
	ID                  uint    `json:"id"`
	GroupID             uint    `json:"groupId"`
	Name                string  `json:"name"`
	Category            string  `json:"category"`
	Collector           string  `json:"collector"`
	ObjectType          string  `json:"objectType"`
	DatasourceType      string  `json:"datasourceType"`
	QueryText           string  `json:"queryText"`
	Comparator          string  `json:"comparator"`
	Threshold           float64 `json:"threshold"`
	ForSeconds          int     `json:"forSeconds"`
	EvalIntervalSeconds int     `json:"evalIntervalSeconds"`
	Severity            string  `json:"severity"`
	LabelsJSON          string  `json:"labelsJson"`
	AnnotationsJSON     string  `json:"annotationsJson"`
	Description         string  `json:"description"`
	Status              int     `json:"status"`
}

type MonitorAlertTemplateGroupPayload struct {
	ID       uint   `json:"id"`
	ParentID uint   `json:"parentId"`
	Name     string `json:"name"`
}

type MonitorAlertTemplateImportItem struct {
	Name                string  `json:"name"`
	PrometheusGroup     string  `json:"prometheusGroup"`
	OriginalExpression  string  `json:"originalExpression"`
	QueryText           string  `json:"queryText"`
	Comparator          string  `json:"comparator"`
	Threshold           float64 `json:"threshold"`
	ForSeconds          int     `json:"forSeconds"`
	EvalIntervalSeconds int     `json:"evalIntervalSeconds"`
	Severity            string  `json:"severity"`
	LabelsJSON          string  `json:"labelsJson"`
	AnnotationsJSON     string  `json:"annotationsJson"`
	Description         string  `json:"description"`
}

type MonitorAlertTemplateImportPayload struct {
	GroupID           uint                             `json:"groupId"`
	DuplicateStrategy string                           `json:"duplicateStrategy"`
	Items             []MonitorAlertTemplateImportItem `json:"items"`
}

type MonitorAlertTemplateExportPayload struct {
	IDs []uint `json:"ids"`
}

type MonitorAlertRuleBatchPayload struct {
	IDs                         []uint `json:"ids"`
	Action                      string `json:"action"`
	NotifyRuleID                uint   `json:"notifyRuleId"`
	NotifyRepeatIntervalSeconds *int   `json:"notifyRepeatIntervalSeconds"`
	MaxNotifyCount              *int   `json:"maxNotifyCount"`
	NotifyRecoveryEnabled       *bool  `json:"notifyRecoveryEnabled"`
	ForSeconds                  *int   `json:"forSeconds"`
	EvalIntervalSeconds         *int   `json:"evalIntervalSeconds"`
}

type MonitorAlertEventActionPayload struct {
	ID         uint   `json:"id"`
	ClaimedBy  string `json:"claimedBy"`
	HandleNote string `json:"handleNote"`
}

type MonitorAlertEventBatchPayload struct {
	IDs        []uint `json:"ids"`
	Action     string `json:"action"`
	ClaimedBy  string `json:"claimedBy"`
	HandleNote string `json:"handleNote"`
}

type MonitorRuleBatchPayload struct {
	IDs    []uint `json:"ids"`
	Action string `json:"action"`
}

type MonitorSilenceRulePayload struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	MatchMode       string `json:"matchMode"`
	RuleIDs         []uint `json:"ruleIds"`
	RuleNamePattern string `json:"ruleNamePattern"`
	Severity        string `json:"severity"`
	AlertType       string `json:"alertType"`
	MatchersJSON    string `json:"matchersJson"`
	StartsAt        int64  `json:"startsAt"`
	EndsAt          int64  `json:"endsAt"`
	Priority        int    `json:"priority"`
	Status          int    `json:"status"`
	Description     string `json:"description"`
}

func normalizeSilencePriority(value int) int {
	if value <= 0 {
		return 100
	}
	if value > 1000 {
		return 1000
	}
	return value
}

type MonitorAggregationRulePayload struct {
	ID                    uint     `json:"id"`
	Name                  string   `json:"name"`
	MatchMode             string   `json:"matchMode"`
	RuleIDs               []uint   `json:"ruleIds"`
	RuleNamePattern       string   `json:"ruleNamePattern"`
	Severity              string   `json:"severity"`
	AlertType             string   `json:"alertType"`
	GroupBy               []string `json:"groupBy"`
	WindowSeconds         int      `json:"windowSeconds"`
	RepeatIntervalSeconds int      `json:"repeatIntervalSeconds"`
	Status                int      `json:"status"`
	Description           string   `json:"description"`
}

type MonitorDashboardPayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Layout      string `json:"layout"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type MonitorDashboardPanelPayload struct {
	ID           uint   `json:"id"`
	DashboardID  uint   `json:"dashboardId"`
	Title        string `json:"title"`
	DatasourceID uint   `json:"datasourceId"`
	PromQL       string `json:"promql"`
	Unit         string `json:"unit"`
	ChartType    string `json:"chartType"`
	Span         int    `json:"span"`
	Sort         int    `json:"sort"`
	Status       int    `json:"status"`
	Description  string `json:"description"`
}

type MonitorDashboardPanelQueryPayload struct {
	ID           uint  `json:"id"`
	DatasourceID uint  `json:"datasourceId"`
	StartAt      int64 `json:"startAt"`
	EndAt        int64 `json:"endAt"`
	StepSeconds  int   `json:"stepSeconds"`
}

type MonitorLogQueryPayload struct {
	DatasourceID   uint   `json:"datasourceId"`
	Index          string `json:"index"`
	Query          string `json:"query"`
	StartAt        int64  `json:"startAt"`
	EndAt          int64  `json:"endAt"`
	PageNum        int    `json:"pageNum"`
	PageSize       int    `json:"pageSize"`
	TrackTotalHits bool   `json:"trackTotalHits"`
}

type MonitorTraceQueryPayload struct {
	DatasourceID uint   `json:"datasourceId"`
	Service      string `json:"service"`
	Operation    string `json:"operation"`
	Tags         string `json:"tags"`
	StartAt      int64  `json:"startAt"`
	EndAt        int64  `json:"endAt"`
	Limit        int    `json:"limit"`
}

type MonitorRangeQueryPayload struct {
	DatasourceID uint   `json:"datasourceId"`
	Query        string `json:"query"`
	StartAt      int64  `json:"startAt"`
	EndAt        int64  `json:"endAt"`
	StepSeconds  int    `json:"stepSeconds"`
}

type MonitorLogShortcutPayload struct {
	ID             uint   `json:"id"`
	DatasourceType string `json:"datasourceType"`
	Name           string `json:"name"`
	Query          string `json:"query"`
	IndexName      string `json:"indexName"`
	TimeRange      string `json:"timeRange"`
	Sort           int    `json:"sort"`
}

type MonitorScheduler struct {
	cron        *cron.Cron
	mu          sync.Mutex
	entries     map[uint]cron.EntryID
	running     map[uint]bool
	healthEntry cron.EntryID
}

type PromQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []PromMetricSample `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type PromMetricSample struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
	Values [][]any           `json:"values"`
}

var springLogPattern = regexp.MustCompile(`(?s)^\s*(\S+\s+\S+)\s+(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\s+(\S+)\s+---\s+\[([^\]]*)\]\s+(.+?)\s*:\s*(.*)$`)

func normalizeMonitorDatasourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "victoriametrics", "victoria-metrics", "vm":
		return "victoriametrics"
	case "victorialogs", "victoria-logs", "vl":
		return "victorialogs"
	case "elasticsearch", "elastic", "es":
		return "elasticsearch"
	case "jaeger", "jaeger-query", "tracing":
		return "jaeger"
	default:
		return "prometheus"
	}
}

func isMonitorLogDatasource(value string) bool {
	datasourceType := normalizeMonitorDatasourceType(value)
	return datasourceType == "elasticsearch" || datasourceType == "victorialogs"
}

func isMonitorMetricDatasource(value string) bool {
	datasourceType := normalizeMonitorDatasourceType(value)
	return datasourceType == "prometheus" || datasourceType == "victoriametrics"
}

func isMonitorTraceDatasource(value string) bool {
	return normalizeMonitorDatasourceType(value) == "jaeger"
}

func normalizeAlertType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "datasource_health", "datasource-health":
		return "datasource_health"
	case "victorialogs", "victoria-logs", "vl":
		return "victorialogs"
	case "log", "elasticsearch", "es":
		return "log"
	default:
		return "metric"
	}
}

func isMonitorLogAlertType(alertType string) bool {
	return normalizeAlertType(alertType) == "log" || normalizeAlertType(alertType) == "victorialogs"
}

func normalizeDatasourceScope(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "all") {
		return "all"
	}
	return "specific"
}

func normalizeLogTimeRangeSeconds(value int) int {
	if value < 60 {
		return 300
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeMonitorStatus(value int) int {
	if value == 2 {
		return 2
	}
	return 1
}

func normalizeMonitorAuthType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "basic":
		return "basic"
	case "bearer":
		return "bearer"
	case "apikey", "api_key", "api-key":
		return "apikey"
	default:
		return "none"
	}
}

func normalizeComparator(value string) string {
	switch strings.TrimSpace(value) {
	case ">", ">=", "<", "<=", "==", "!=":
		return strings.TrimSpace(value)
	default:
		return ">"
	}
}

func normalizeSeverity(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "P0", "P1", "P2", "P3":
		return value
	default:
		return "P2"
	}
}

func normalizeEvalInterval(value int) int {
	if value < 15 {
		return 15
	}
	if value > 3600 {
		return 3600
	}
	return value
}

func normalizeForSeconds(value int) int {
	if value < 0 {
		return 0
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeNotifyRepeatInterval(value int) int {
	if value < 60 {
		return 60
	}
	if value > 604800 {
		return 604800
	}
	return value
}

func normalizeMaxNotifyCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func ensureJSONObject(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "{}", nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(value), &obj); err != nil {
		return "", errors.New("JSON 格式不正确")
	}
	data, _ := json.Marshal(obj)
	return string(data), nil
}

func normalizeMatcherJSON(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "{}", nil
	}
	matchers := map[string]string{}
	if err := json.Unmarshal([]byte(value), &matchers); err != nil {
		return "", errors.New("matchers must be valid JSON object")
	}
	data, _ := json.Marshal(matchers)
	return string(data), nil
}

func unixPtr(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	t := time.Unix(value, 0)
	return &t
}

func normalizeAggregationWindow(value int) int {
	if value < 60 {
		return 60
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeRuleMatchMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "select", "selected", "rules":
		return "select"
	default:
		return "regex"
	}
}

func monitorRuleNameMatch(pattern, ruleName string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	re, err := regexp.Compile(pattern)
	if err == nil {
		return re.MatchString(ruleName)
	}
	return strings.Contains(strings.ToLower(ruleName), strings.ToLower(pattern))
}

func monitorRuleMatch(matchMode, ruleIDsJSON, pattern string, rule model.MonitorAlertRule) bool {
	if normalizeRuleMatchMode(matchMode) == "select" {
		ids := decodeUintList(ruleIDsJSON)
		if len(ids) == 0 {
			return true
		}
		for _, id := range ids {
			if id == rule.ID {
				return true
			}
		}
		return false
	}
	return monitorRuleNameMatch(pattern, rule.Name)
}

func monitorSeverityMatch(pattern, severity string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.EqualFold(pattern, "all") {
		return true
	}
	return strings.EqualFold(pattern, severity)
}

func decodeLabelMap(raw string) map[string]string {
	labels := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &labels)
	return labels
}

func monitorMatchersMatch(matchersJSON string, labels map[string]string) bool {
	matchers := map[string]string{}
	_ = json.Unmarshal([]byte(matchersJSON), &matchers)
	for key, expected := range matchers {
		if labels[key] != expected {
			return false
		}
	}
	return true
}

func (s *Service) initMonitorScheduler() {
	s.monitorSchedulerOnce.Do(func() {
		s.monitorScheduler = &MonitorScheduler{
			cron: cron.New(
				cron.WithSeconds(),
				cron.WithLocation(time.Local),
				cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
			),
			entries: map[uint]cron.EntryID{},
			running: map[uint]bool{},
		}
		s.monitorScheduler.cron.Start()
		s.reloadMonitorAlertRules()
		// Repair historical events left firing by rules that were disabled before
		// the automatic-close behavior existed.
		s.closeInactiveMonitorAlertRuleEvents()
		// Aggregated alerts are intentionally delayed until their convergence
		// window closes, so a short cadence is needed even when no rule happens
		// to be evaluated at that exact moment.
		_, _ = s.monitorScheduler.cron.AddFunc("@every 15s", s.flushDueMonitorAggregationNotifications)
		entryID, err := s.monitorScheduler.cron.AddFunc("@every 15s", s.checkAllMonitorDatasources)
		if err == nil {
			s.monitorScheduler.healthEntry = entryID
		}
		go s.checkAllMonitorDatasources()
	})
}

func (s *Service) reloadMonitorAlertRules() {
	if s.monitorScheduler == nil {
		return
	}
	var rules []model.MonitorAlertRule
	if err := s.db.Where("status = ?", 1).Find(&rules).Error; err != nil {
		return
	}
	for _, rule := range rules {
		_ = s.registerMonitorAlertRule(rule)
	}
}

func (s *Service) registerMonitorAlertRule(rule model.MonitorAlertRule) error {
	if s.monitorScheduler == nil {
		return nil
	}
	s.removeMonitorAlertRule(rule.ID)
	interval := normalizeEvalInterval(rule.EvalIntervalSeconds)
	entryID, err := s.monitorScheduler.cron.AddFunc(fmt.Sprintf("@every %ds", interval), func() {
		s.evaluateMonitorAlertRule(rule.ID)
	})
	if err != nil {
		return err
	}
	s.monitorScheduler.mu.Lock()
	s.monitorScheduler.entries[rule.ID] = entryID
	s.monitorScheduler.mu.Unlock()
	return nil
}

func (s *Service) removeMonitorAlertRule(id uint) {
	if s.monitorScheduler == nil {
		return
	}
	s.monitorScheduler.mu.Lock()
	entryID, ok := s.monitorScheduler.entries[id]
	if ok {
		delete(s.monitorScheduler.entries, id)
	}
	s.monitorScheduler.mu.Unlock()
	if ok {
		s.monitorScheduler.cron.Remove(entryID)
	}
}

func (s *Service) ListMonitorDatasources(pageNum, pageSize int, keyword, dsType, status, env string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorDatasource{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR url LIKE ? OR description LIKE ?", like, like, like)
	}
	if strings.TrimSpace(dsType) != "" {
		query = query.Where("type = ?", normalizeMonitorDatasourceType(dsType))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(env) != "" {
		query = query.Where("env = ?", normalizeEnvCode(env))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorDatasource
	if err := query.Order("is_default DESC, id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListMonitorDatasourceOptions() ([]model.MonitorDatasource, error) {
	var list []model.MonitorDatasource
	if err := s.db.Where("status = ?", 1).Order("is_default DESC, id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetMonitorDatasource(id uint) (*model.MonitorDatasource, error) {
	var item model.MonitorDatasource
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) SaveMonitorDatasource(payload MonitorDatasourcePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("数据源名称不能为空")
	}
	if strings.TrimSpace(payload.URL) == "" {
		return errors.New("数据源地址不能为空")
	}
	updates := map[string]any{
		"name":        Trimmed(payload.Name),
		"type":        normalizeMonitorDatasourceType(payload.Type),
		"url":         strings.TrimRight(strings.TrimSpace(payload.URL), "/"),
		"auth_type":   normalizeMonitorAuthType(payload.AuthType),
		"username":    strings.TrimSpace(payload.Username),
		"password":    payload.Password,
		"token":       strings.TrimSpace(payload.Token),
		"is_default":  payload.IsDefault,
		"env":         normalizeEnvCode(payload.Env),
		"status":      normalizeMonitorStatus(payload.Status),
		"description": Trimmed(payload.Description),
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if payload.IsDefault {
			if err := tx.Model(&model.MonitorDatasource{}).Where("id <> ?", payload.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if payload.ID > 0 {
			return tx.Model(&model.MonitorDatasource{}).Where("id = ?", payload.ID).Updates(updates).Error
		}
		return tx.Model(&model.MonitorDatasource{}).Create(updates).Error
	})
}

func (s *Service) DeleteMonitorDatasource(id uint) error {
	var count int64
	if err := s.db.Model(&model.MonitorAlertRule{}).Where("datasource_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("数据源已被告警规则引用，不能删除")
	}
	if err := s.db.Model(&model.MonitorDashboardPanel{}).Where("datasource_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("数据源已被监控大屏引用，不能删除")
	}
	if err := s.db.Model(&model.K8sCluster{}).Where("monitor_datasource_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("数据源已被 K8s 集群监控绑定，不能删除")
	}
	return s.db.Delete(&model.MonitorDatasource{}, id).Error
}

func (s *Service) TestMonitorDatasource(id uint, payload MonitorDatasourcePayload) error {
	var ds model.MonitorDatasource
	if id > 0 {
		item, err := s.GetMonitorDatasource(id)
		if err != nil {
			return err
		}
		ds = *item
	} else {
		ds = model.MonitorDatasource{
			Name: payload.Name, Type: normalizeMonitorDatasourceType(payload.Type), URL: strings.TrimRight(strings.TrimSpace(payload.URL), "/"),
			AuthType: normalizeMonitorAuthType(payload.AuthType), Username: payload.Username, Password: payload.Password, Token: payload.Token,
		}
	}
	startedAt := time.Now()
	err := s.checkMonitorDatasourceHealth(ds)
	if id > 0 {
		s.persistMonitorDatasourceHealth(id, err, time.Since(startedAt).Milliseconds())
	}
	return err
}

func (s *Service) checkMonitorDatasourceHealth(ds model.MonitorDatasource) error {
	switch normalizeMonitorDatasourceType(ds.Type) {
	case "elasticsearch":
		return s.elasticsearchHealth(ds)
	case "victorialogs":
		return s.victoriaLogsHealth(ds)
	case "jaeger":
		return s.jaegerHealth(ds)
	}
	_, err := s.prometheusQuery(ds, "up", time.Now())
	return err
}

func (s *Service) persistMonitorDatasourceHealth(id uint, healthErr error, latencyMs int64) {
	var datasource model.MonitorDatasource
	if err := s.db.First(&datasource, id).Error; err != nil {
		return
	}
	now := time.Now()
	updates := map[string]any{"last_check_at": &now, "latency_ms": latencyMs}
	failures, successes := 0, 0
	if healthErr == nil {
		successes = datasource.ConsecutiveSuccesses + 1
		updates["health_status"] = "healthy"
		updates["last_success_at"] = &now
		updates["last_error"] = ""
		updates["consecutive_failures"] = 0
		updates["consecutive_successes"] = successes
	} else {
		failures = datasource.ConsecutiveFailures + 1
		updates["health_status"] = "unhealthy"
		updates["last_error"] = healthErr.Error()
		updates["consecutive_failures"] = failures
		updates["consecutive_successes"] = 0
	}
	_ = s.db.Model(&model.MonitorDatasource{}).Where("id = ?", id).Updates(updates).Error
	s.syncMonitorDatasourceHealthAlert(datasource, healthErr, failures, successes, now)
}

func (s *Service) syncMonitorDatasourceHealthAlert(ds model.MonitorDatasource, healthErr error, failures, successes int, now time.Time) {
	var rules []model.MonitorAlertRule
	if s.db.Where("alert_type = ? AND datasource_id = ? AND status = ?", "datasource_health", ds.ID, 1).Find(&rules).Error != nil {
		return
	}
	for _, rule := range rules {
		failureThreshold := int(rule.Threshold)
		if failureThreshold < 1 {
			failureThreshold = 2
		}
		fingerprint := fmt.Sprintf("datasource-health:%d:%d", rule.ID, ds.ID)
		var event model.MonitorAlertEvent
		err := s.db.Where("fingerprint = ? AND status IN ?", fingerprint, []string{"firing", "claimed", "silenced"}).First(&event).Error
		if healthErr != nil && failures >= failureThreshold {
			labels, _ := json.Marshal(map[string]string{"datasource": ds.Name, "type": ds.Type, "environment": ds.Env, "url": ds.URL})
			summary := fmt.Sprintf("%s：%s（连续失败 %d 次）", rule.Name, ds.Name, failures)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				event = model.MonitorAlertEvent{RuleID: rule.ID, RuleName: rule.Name, DatasourceID: ds.ID, DatasourceName: ds.Name, Fingerprint: fingerprint, Severity: rule.Severity, Status: "firing", Metric: "datasource_health", LabelsJSON: string(labels), AnnotationsJSON: rule.AnnotationsJSON, CurrentValue: float64(failures), Threshold: rule.Threshold, Summary: summary, FirstTriggerAt: now, LastTriggerAt: now}
				if s.db.Create(&event).Error == nil {
					s.appendMonitorAlertTimeline(event.ID, "firing", fmt.Sprintf("数据源连续 %d 次连接失败", failureThreshold), healthErr.Error(), "系统", nil)
				}
			} else if err == nil {
				_ = s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", event.ID).Updates(map[string]any{"last_trigger_at": now, "current_value": failures, "summary": summary}).Error
			}
			continue
		}
		if healthErr == nil && successes >= 3 && err == nil {
			_ = s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", event.ID).Updates(map[string]any{"status": "recovered", "recovered_at": &now, "last_trigger_at": now}).Error
			event.RecoveredAt = &now
			event.Status = "recovered"
			s.appendMonitorAlertTimeline(event.ID, "recovered", "数据源连续三次检测成功，告警恢复", "数据源连接已稳定恢复", "系统", nil)
			s.dispatchDatasourceHealthNotification(event, "recovered")
		}
	}
}

func (s *Service) dispatchDatasourceHealthNotification(event model.MonitorAlertEvent, status string) {
	var rules []model.NotifyRule
	if s.db.Where("scope = ? AND status = ?", "monitor", 1).Find(&rules).Error != nil {
		return
	}
	for _, rule := range rules {
		s.DispatchNotifyRule(rule.ID, NotifyEvent{Scope: "monitor", Event: status, TargetID: event.ID, TargetName: event.RuleName, Status: status, Summary: event.Summary, Detail: event.LabelsJSON, StartedAt: &event.FirstTriggerAt, FinishedAt: event.RecoveredAt, Extra: map[string]string{"alertName": event.RuleName, "severity": event.Severity, "datasourceName": event.DatasourceName}})
	}
}

func (s *Service) checkAllMonitorDatasources() {
	var datasources []model.MonitorDatasource
	if err := s.db.Where("status = ?", 1).Find(&datasources).Error; err != nil {
		return
	}
	for _, ds := range datasources {
		startedAt := time.Now()
		err := s.checkMonitorDatasourceHealth(ds)
		s.persistMonitorDatasourceHealth(ds.ID, err, time.Since(startedAt).Milliseconds())
	}
}

func (s *Service) PrometheusInstantQuery(datasourceID uint, query string, ts time.Time) (map[string]any, error) {
	return s.MonitorInstantQuery(datasourceID, query, ts)
}

func (s *Service) MonitorRangeQuery(payload MonitorRangeQueryPayload) (map[string]any, error) {
	if strings.TrimSpace(payload.Query) == "" {
		return nil, errors.New("查询语句不能为空")
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return nil, err
	}
	if !isMonitorMetricDatasource(ds.Type) {
		return nil, errors.New("图表查询仅支持 Prometheus 或 VictoriaMetrics 数据源")
	}
	endAt := time.Unix(payload.EndAt, 0)
	if payload.EndAt <= 0 {
		endAt = time.Now()
	}
	startAt := time.Unix(payload.StartAt, 0)
	if payload.StartAt <= 0 || !startAt.Before(endAt) {
		startAt = endAt.Add(-time.Hour)
	}
	result, err := s.prometheusRangeQuery(*ds, payload.Query, startAt, endAt, payload.StepSeconds)
	if err != nil {
		return nil, err
	}
	return map[string]any{"resultType": result.Data.ResultType, "result": result.Data.Result, "startAt": startAt.Unix(), "endAt": endAt.Unix()}, nil
}

func (s *Service) MonitorInstantQuery(datasourceID uint, query string, ts time.Time) (map[string]any, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("查询语句不能为空")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	queryType := "promql"
	var response map[string]any
	if normalizeMonitorDatasourceType(ds.Type) == "elasticsearch" {
		queryType = "elasticsearch"
		response, err = s.elasticsearchQuery(*ds, query)
	} else if normalizeMonitorDatasourceType(ds.Type) == "victorialogs" {
		return nil, errors.New("VictoriaLogs 请在日志查询中使用 LogsQL")
	} else {
		var result *PromQueryResult
		result, err = s.prometheusQuery(*ds, query, ts)
		if err == nil {
			response = map[string]any{"resultType": result.Data.ResultType, "result": result.Data.Result}
		}
	}
	status := "success"
	errorText := ""
	if err != nil {
		status = "failed"
		errorText = err.Error()
	}
	_ = s.db.Create(&model.MonitorQueryHistory{
		DatasourceID: ds.ID, DatasourceName: ds.Name, Query: query, QueryType: queryType, Status: status, ErrorText: errorText,
	}).Error
	s.trimMonitorQueryHistories(10)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) trimMonitorQueryHistories(limit int) {
	if limit < 1 {
		limit = 10
	}
	var keepIDs []uint
	if err := s.db.Model(&model.MonitorQueryHistory{}).Order("id DESC").Limit(limit).Pluck("id", &keepIDs).Error; err != nil {
		return
	}
	if len(keepIDs) < limit {
		return
	}
	_ = s.db.Where("id NOT IN ?", keepIDs).Delete(&model.MonitorQueryHistory{}).Error
}

func (s *Service) ListMonitorQueryHistories(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 10 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorQueryHistory{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("datasource_name LIKE ? OR query LIKE ? OR error_text LIKE ?", like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorQueryHistory
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) prometheusQuery(ds model.MonitorDatasource, query string, ts time.Time) (*PromQueryResult, error) {
	endpoint := strings.TrimRight(ds.URL, "/") + "/api/v1/query"
	params := url.Values{}
	params.Set("query", query)
	if !ts.IsZero() {
		params.Set("time", strconv.FormatFloat(float64(ts.Unix()), 'f', -1, 64))
	}
	request, err := http.NewRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	applyMonitorDatasourceAuth(request, ds)
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Prometheus API 返回状态码 %d: %s", response.StatusCode, string(body))
	}
	var result PromQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Status != "success" {
		return nil, errors.New(firstNonEmpty(result.Error, "Prometheus query failed"))
	}
	return &result, nil
}

func (s *Service) prometheusRangeQuery(ds model.MonitorDatasource, query string, startAt, endAt time.Time, stepSeconds int) (*PromQueryResult, error) {
	if endAt.Before(startAt) || endAt.Equal(startAt) {
		return nil, errors.New("查询结束时间必须晚于开始时间")
	}
	if stepSeconds < 15 {
		stepSeconds = 15
	}
	if stepSeconds > 3600 {
		stepSeconds = 3600
	}
	endpoint := strings.TrimRight(ds.URL, "/") + "/api/v1/query_range"
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(startAt.Unix(), 10))
	params.Set("end", strconv.FormatInt(endAt.Unix(), 10))
	params.Set("step", strconv.Itoa(stepSeconds))
	request, err := http.NewRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Prometheus API 返回状态码 %d: %s", response.StatusCode, string(body))
	}
	var result PromQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Status != "success" {
		return nil, errors.New(firstNonEmpty(result.Error, "Prometheus range query failed"))
	}
	return &result, nil
}

func (s *Service) elasticsearchHealth(ds model.MonitorDatasource) error {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(ds.URL, "/")+"/_cluster/health", nil)
	if err != nil {
		return err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Elasticsearch 健康检查失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	return nil
}

// jaegerHealth verifies the Jaeger Query API is reachable. The services
// endpoint is available on both the all-in-one and query service deployments.
func (s *Service) jaegerHealth(ds model.MonitorDatasource) error {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(ds.URL, "/")+"/api/services", nil)
	if err != nil {
		return err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Jaeger 健康检查失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	return nil
}

func (s *Service) ListMonitorJaegerServices(datasourceID uint) ([]string, error) {
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	if !isMonitorTraceDatasource(ds.Type) {
		return nil, errors.New("当前数据源不是 Jaeger")
	}
	var data []string
	if err := s.jaegerGet(*ds, "/api/services", nil, &data); err != nil {
		return nil, err
	}
	sort.Strings(data)
	return data, nil
}

func (s *Service) ListMonitorJaegerOperations(datasourceID uint, service string) ([]string, error) {
	if strings.TrimSpace(service) == "" {
		return []string{}, nil
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	if !isMonitorTraceDatasource(ds.Type) {
		return nil, errors.New("当前数据源不是 Jaeger")
	}
	var data []string
	if err := s.jaegerGet(*ds, "/api/services/"+url.PathEscape(strings.TrimSpace(service))+"/operations", nil, &data); err != nil {
		return nil, err
	}
	sort.Strings(data)
	return data, nil
}

func (s *Service) QueryMonitorTraces(payload MonitorTraceQueryPayload) ([]map[string]any, error) {
	if payload.DatasourceID == 0 {
		return nil, errors.New("请选择 Jaeger 数据源")
	}
	if strings.TrimSpace(payload.Service) == "" {
		return nil, errors.New("请选择服务")
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return nil, err
	}
	if !isMonitorTraceDatasource(ds.Type) {
		return nil, errors.New("链路追踪仅支持 Jaeger 数据源")
	}
	endAt := payload.EndAt
	if endAt <= 0 {
		endAt = time.Now().UnixMilli()
	}
	startAt := payload.StartAt
	if startAt <= 0 || startAt >= endAt {
		startAt = endAt - int64(time.Hour/time.Millisecond)
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	params := url.Values{
		"service": {strings.TrimSpace(payload.Service)},
		"start":   {strconv.FormatInt(startAt*1000, 10)},
		"end":     {strconv.FormatInt(endAt*1000, 10)},
		"limit":   {strconv.Itoa(limit)},
	}
	if operation := strings.TrimSpace(payload.Operation); operation != "" {
		params.Set("operation", operation)
	}
	if tags := strings.TrimSpace(payload.Tags); tags != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(tags), &parsed); err != nil {
			return nil, errors.New("标签筛选必须是 JSON 对象")
		}
		params.Set("tags", tags)
	}
	var data []map[string]any
	if err := s.jaegerGet(*ds, "/api/traces", params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) GetMonitorTrace(datasourceID uint, traceID string) (map[string]any, error) {
	if datasourceID == 0 || strings.TrimSpace(traceID) == "" {
		return nil, errors.New("请填写数据源和 Trace ID")
	}
	if strings.ContainsAny(traceID, "/\\") {
		return nil, errors.New("Trace ID 格式无效")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	if !isMonitorTraceDatasource(ds.Type) {
		return nil, errors.New("链路追踪仅支持 Jaeger 数据源")
	}
	var data []map[string]any
	if err := s.jaegerGet(*ds, "/api/traces/"+url.PathEscape(strings.TrimSpace(traceID)), nil, &data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("未找到该 Trace")
	}
	return data[0], nil
}

func (s *Service) jaegerGet(ds model.MonitorDatasource, path string, params url.Values, target any) error {
	endpoint := strings.TrimRight(ds.URL, "/") + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Jaeger API 返回状态码 %d: %s", response.StatusCode, string(body))
	}
	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []any           `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("Jaeger 查询失败: %v", result.Errors[0])
	}
	if len(result.Data) == 0 {
		return errors.New("Jaeger API 未返回数据")
	}
	return json.Unmarshal(result.Data, target)
}

func (s *Service) elasticsearchQuery(ds model.MonitorDatasource, query string) (map[string]any, error) {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(query), &payload); err != nil {
		return nil, errors.New("Elasticsearch DSL 必须是有效的 JSON 对象")
	}
	index := strings.TrimSpace(fmt.Sprint(payload["index"]))
	delete(payload, "index")
	if index == "" || index == "<nil>" {
		index = "_all"
	}
	if strings.Contains(index, "/") || strings.Contains(index, "\\") {
		return nil, errors.New("Elasticsearch 索引不能包含路径分隔符")
	}
	if _, exists := payload["size"]; !exists {
		payload["size"] = 100
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(ds.URL, "/") + "/" + url.PathEscape(index) + "/_search"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Elasticsearch 查询失败，状态码 %d: %s", response.StatusCode, string(responseBody))
	}
	var raw map[string]any
	if err := json.Unmarshal(responseBody, &raw); err != nil {
		return nil, err
	}
	hits, _ := raw["hits"].(map[string]any)
	documents, _ := hits["hits"].([]any)
	return map[string]any{
		"resultType": "elasticsearch",
		"result":     documents,
		"total":      hits["total"],
		"took":       raw["took"],
	}, nil
}

func (s *Service) QueryMonitorLogs(payload MonitorLogQueryPayload) (map[string]any, error) {
	if payload.DatasourceID == 0 {
		return nil, errors.New("请选择日志数据源")
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return nil, err
	}
	switch normalizeMonitorDatasourceType(ds.Type) {
	case "elasticsearch":
		return s.queryElasticsearchMonitorLogs(payload)
	case "victorialogs":
		return s.queryVictoriaLogs(*ds, payload)
	default:
		return nil, errors.New("日志查询仅支持 Elasticsearch 或 VictoriaLogs 数据源")
	}
}

func (s *Service) queryElasticsearchMonitorLogs(payload MonitorLogQueryPayload) (map[string]any, error) {
	if payload.DatasourceID == 0 {
		return nil, errors.New("请选择 Elasticsearch 数据源")
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return nil, err
	}
	if normalizeMonitorDatasourceType(ds.Type) != "elasticsearch" {
		return nil, errors.New("日志查询仅支持 Elasticsearch 数据源")
	}
	index := strings.TrimSpace(payload.Index)
	if index == "" {
		index = "_all"
	}
	if strings.Contains(index, "/") || strings.Contains(index, "\\") {
		return nil, errors.New("索引不能包含路径分隔符")
	}
	pageNum, pageSize := normalizeMonitorLogPagination(payload.PageNum, payload.PageSize)
	endAt := payload.EndAt
	if endAt <= 0 {
		endAt = time.Now().UnixMilli()
	}
	startAt := payload.StartAt
	if startAt <= 0 || startAt >= endAt {
		startAt = time.UnixMilli(endAt).Add(-24 * time.Hour).UnixMilli()
	}
	must := make([]any, 0, 1)
	if strings.TrimSpace(payload.Query) == "" {
		must = append(must, map[string]any{"match_all": map[string]any{}})
	} else {
		must = append(must, map[string]any{"query_string": map[string]any{"query": strings.TrimSpace(payload.Query), "analyze_wildcard": true}})
	}
	body := map[string]any{
		"from": (pageNum - 1) * pageSize,
		"size": pageSize,
		"sort": []any{map[string]any{"@timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}},
		"query": map[string]any{"bool": map[string]any{
			"must":   must,
			"filter": []any{map[string]any{"range": map[string]any{"@timestamp": map[string]any{"gte": startAt, "lte": endAt, "format": "epoch_millis"}}}},
		}},
		"aggs": map[string]any{"histogram": map[string]any{"date_histogram": map[string]any{
			"field": "@timestamp", "fixed_interval": monitorLogHistogramInterval(startAt, endAt), "min_doc_count": 0,
		}}},
	}
	if payload.TrackTotalHits {
		body["track_total_hits"] = true
	}
	response, err := s.elasticsearchSearch(*ds, index, body)
	if err != nil {
		return nil, err
	}
	hits, _ := response["hits"].(map[string]any)
	rawItems, _ := hits["hits"].([]any)
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		hit, _ := rawItem.(map[string]any)
		source, _ := hit["_source"].(map[string]any)
		kubernetes, _ := source["kubernetes"].(map[string]any)
		rawMessage := cleanMonitorLogMessage(firstNonEmpty(monitorSourceString(source["message"]), monitorSourceString(source["log"])))
		messageFields := parseMonitorLogMessage(rawMessage)
		level := firstNonEmpty(monitorSourceString(source["level"]), messageFields["level"], detectMonitorLogLevel(rawMessage))
		displayMessage := messageFields["content"]
		if opLogMessage := formatMonitorOpLogContent(source, monitorSourceString(hit["_index"]), rawMessage); opLogMessage != "" {
			displayMessage = opLogMessage
		}
		items = append(items, map[string]any{
			"index":      hit["_index"],
			"id":         hit["_id"],
			"timestamp":  firstNonEmpty(monitorSourceString(source["@timestamp"]), monitorSourceString(source["timestamp"])),
			"namespace":  firstNonEmpty(monitorSourceString(kubernetes["pod_namespace"]), monitorSourceString(source["namespace"])),
			"pod":        firstNonEmpty(monitorSourceString(kubernetes["pod_name"]), monitorSourceString(source["pod"])),
			"container":  firstNonEmpty(monitorSourceString(kubernetes["container_name"]), monitorSourceString(source["container"])),
			"level":      level,
			"message":    displayMessage,
			"messageRaw": rawMessage,
			"logTime":    messageFields["timestamp"],
			"processId":  messageFields["processId"],
			"thread":     messageFields["thread"],
			"logger":     messageFields["logger"],
			"source":     source,
		})
	}
	aggs, _ := response["aggregations"].(map[string]any)
	histogram, _ := aggs["histogram"].(map[string]any)
	buckets, _ := histogram["buckets"].([]any)
	return map[string]any{
		"items": items, "total": hits["total"], "took": response["took"], "histogram": buckets,
		"startAt": startAt, "endAt": endAt, "pageNum": pageNum, "pageSize": pageSize,
	}, nil
}

func (s *Service) victoriaLogsHealth(ds model.MonitorDatasource) error {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(ds.URL, "/")+"/metrics", nil)
	if err != nil {
		return err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("VictoriaLogs 健康检查失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	return nil
}

func (s *Service) queryVictoriaLogs(ds model.MonitorDatasource, payload MonitorLogQueryPayload) (map[string]any, error) {
	pageNum, pageSize := normalizeMonitorLogPagination(payload.PageNum, payload.PageSize)
	endAt := payload.EndAt
	if endAt <= 0 {
		endAt = time.Now().UnixMilli()
	}
	startAt := payload.StartAt
	if startAt <= 0 || startAt >= endAt {
		startAt = time.UnixMilli(endAt).Add(-24 * time.Hour).UnixMilli()
	}
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		query = "*"
	}
	startedAt := time.Now()
	items, err := s.queryVictoriaLogsRows(ds, query, startAt, endAt, pageNum, pageSize)
	if err != nil {
		return nil, err
	}
	histogram, total := s.victoriaLogsHistogram(ds, query, startAt, endAt)
	// VictoriaLogs 的 /query 和 /hits 在起始时间边界上偶发不一致：
	// /hits 已计入一条记录，但 /query 返回空行。仅在这个异常分支把起点
	// 向前补偿一秒重试，避免页面出现“命中 1 条却没有日志”的假空结果。
	if len(items) == 0 && total > 0 && pageNum == 1 && startAt > 1000 {
		if retryItems, retryErr := s.queryVictoriaLogsRows(ds, query, startAt-1000, endAt, pageNum, pageSize); retryErr == nil {
			items = retryItems
		}
	}
	if total == 0 && len(items) > 0 {
		total = int64(len(items))
	}
	return map[string]any{
		"items": items, "total": total, "took": time.Since(startedAt).Milliseconds(), "histogram": histogram,
		"startAt": startAt, "endAt": endAt, "pageNum": pageNum, "pageSize": pageSize,
	}, nil
}

func (s *Service) queryVictoriaLogsRows(ds model.MonitorDatasource, query string, startAt, endAt int64, pageNum, pageSize int) ([]map[string]any, error) {
	form := url.Values{}
	form.Set("query", query)
	form.Set("start", time.UnixMilli(startAt).UTC().Format(time.RFC3339Nano))
	form.Set("end", time.UnixMilli(endAt).UTC().Format(time.RFC3339Nano))
	form.Set("limit", strconv.Itoa(pageSize))
	form.Set("offset", strconv.Itoa((pageNum-1)*pageSize))
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/query", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaLogs 查询失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var source map[string]any
		if err := json.Unmarshal([]byte(line), &source); err != nil {
			return nil, fmt.Errorf("解析 VictoriaLogs 返回记录失败: %w", err)
		}
		items = append(items, formatMonitorLogItem(source, "", ""))
	}
	return items, nil
}

func normalizeMonitorLogPagination(pageNum, pageSize int) (int, int) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 200
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return pageNum, pageSize
}

func (s *Service) victoriaLogsHistogram(ds model.MonitorDatasource, query string, startAt, endAt int64) ([]map[string]any, int64) {
	form := url.Values{}
	form.Set("query", query)
	form.Set("start", time.UnixMilli(startAt).UTC().Format(time.RFC3339Nano))
	form.Set("end", time.UnixMilli(endAt).UTC().Format(time.RFC3339Nano))
	step := monitorLogHistogramStepSeconds(startAt, endAt)
	form.Set("step", strconv.FormatInt(step, 10)+"s")
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/hits", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, 0
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, 0
	}
	var result struct {
		Hits []struct {
			Timestamps []string `json:"timestamps"`
			Values     []int64  `json:"values"`
			Total      int64    `json:"total"`
		} `json:"hits"`
	}
	if json.Unmarshal(body, &result) != nil {
		return nil, 0
	}
	bucketCounts := map[string]int64{}
	var total int64
	for _, group := range result.Hits {
		total += group.Total
		for i, timestamp := range group.Timestamps {
			if i < len(group.Values) {
				bucketCounts[timestamp] += group.Values[i]
			}
		}
	}
	timestamps := make([]string, 0, len(bucketCounts))
	for timestamp := range bucketCounts {
		timestamps = append(timestamps, timestamp)
	}
	sort.Strings(timestamps)
	buckets := make([]map[string]any, 0, len(timestamps))
	for _, timestamp := range timestamps {
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		key := int64(0)
		if err == nil {
			key = parsed.UnixMilli()
		}
		buckets = append(buckets, map[string]any{
			"key": key, "key_as_string": timestamp, "doc_count": bucketCounts[timestamp],
		})
	}
	return buckets, total
}

// Keep the log histogram readable regardless of the selected range. Around
// 20–40 buckets make spikes visible without turning the chart into a solid bar.
func monitorLogHistogramStepSeconds(startAt, endAt int64) int64 {
	span := endAt - startAt
	switch {
	case span <= time.Hour.Milliseconds():
		return 60
	case span <= 6*time.Hour.Milliseconds():
		return 5 * 60
	case span <= 24*time.Hour.Milliseconds():
		return 15 * 60
	case span <= 3*24*time.Hour.Milliseconds():
		return 60 * 60
	case span <= 7*24*time.Hour.Milliseconds():
		return 3 * 60 * 60
	default:
		return 6 * 60 * 60
	}
}

func monitorLogHistogramInterval(startAt, endAt int64) string {
	return strconv.FormatInt(monitorLogHistogramStepSeconds(startAt, endAt)/60, 10) + "m"
}

func formatMonitorLogItem(source map[string]any, index, id string) map[string]any {
	rawMessage := cleanMonitorLogMessage(firstNonEmpty(
		monitorLogFieldValue(source, "_msg"), monitorLogFieldValue(source, "message"), monitorLogFieldValue(source, "log"),
	))
	messageFields := parseMonitorLogMessage(rawMessage)
	level := firstNonEmpty(monitorLogFieldValue(source, "level"), messageFields["level"], detectMonitorLogLevel(rawMessage))
	displayMessage := messageFields["content"]
	if opLogMessage := formatMonitorOpLogContent(source, index, rawMessage); opLogMessage != "" {
		displayMessage = opLogMessage
	}
	return map[string]any{
		"index": index, "id": id,
		"timestamp": firstNonEmpty(monitorLogFieldValue(source, "_time"), monitorLogFieldValue(source, "@timestamp"), monitorLogFieldValue(source, "timestamp")),
		"namespace": firstNonEmpty(monitorLogFieldValue(source, "kubernetes.pod_namespace"), monitorLogFieldValue(source, "namespace")),
		"pod":       firstNonEmpty(monitorLogFieldValue(source, "kubernetes.pod_name"), monitorLogFieldValue(source, "pod")),
		"container": firstNonEmpty(monitorLogFieldValue(source, "kubernetes.container_name"), monitorLogFieldValue(source, "container")),
		"level":     level, "message": displayMessage, "messageRaw": rawMessage,
		"logTime": messageFields["timestamp"], "processId": messageFields["processId"], "thread": messageFields["thread"], "logger": messageFields["logger"], "source": source,
	}
}

func monitorLogFieldValue(source map[string]any, path string) string {
	if value, ok := source[path]; ok {
		return monitorSourceString(value)
	}
	current := any(source)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[part]
	}
	return monitorSourceString(current)
}

func (s *Service) ListMonitorVictoriaLogsStreams(datasourceID uint, field, query string, startAt, endAt int64, limit int) ([]map[string]any, error) {
	if datasourceID == 0 {
		return nil, errors.New("请选择 VictoriaLogs 数据源")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	if normalizeMonitorDatasourceType(ds.Type) != "victorialogs" {
		return nil, errors.New("当前数据源不是 VictoriaLogs")
	}
	field = firstNonEmpty(strings.TrimSpace(field), "kafka_topic")
	limit = normalizeMonitorLogFieldValueLimit(limit)
	end := time.UnixMilli(endAt)
	if endAt <= 0 {
		end = time.Now()
	}
	start := time.UnixMilli(startAt)
	if startAt <= 0 || startAt >= end.UnixMilli() {
		start = end.Add(-time.Hour)
	}
	logsQL := strings.TrimSpace(query)
	if logsQL == "" {
		logsQL = "*"
	}
	form := url.Values{}
	form.Set("query", logsQL)
	form.Set("field", field)
	form.Set("start", start.UTC().Format(time.RFC3339Nano))
	form.Set("end", end.UTC().Format(time.RFC3339Nano))
	// 不向 VictoriaLogs field_values 透传 limit。部分版本在携带该参数时仍会
	// 返回字段值，但 hits 会全部变成 0；在收到带真实命中数的结果后再统一截取。
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/field_values", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, *ds)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 VictoriaLogs Stream 失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	var raw struct {
		Values []struct {
			Value string `json:"value"`
			Hits  int64  `json:"hits"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(raw.Values))
	for _, item := range raw.Values {
		if strings.TrimSpace(item.Value) == "" {
			continue
		}
		items = append(items, map[string]any{"value": item.Value, "hits": item.Hits, "field": field})
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := items[i]["hits"].(int64)
		right, _ := items[j]["hits"].(int64)
		if left == right {
			return monitorSourceString(items[i]["value"]) < monitorSourceString(items[j]["value"])
		}
		return left > right
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *Service) ListMonitorLogFields(datasourceID uint, index, query string, startAt, endAt int64) ([]map[string]any, error) {
	if datasourceID == 0 {
		return nil, errors.New("请选择日志数据源")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	switch normalizeMonitorDatasourceType(ds.Type) {
	case "elasticsearch":
		return s.elasticsearchLogFields(*ds, index)
	case "victorialogs":
		return s.victoriaLogsFields(*ds, query, startAt, endAt)
	default:
		return nil, errors.New("日志字段仅支持 Elasticsearch 或 VictoriaLogs 数据源")
	}
}

func (s *Service) ListMonitorLogFieldValues(datasourceID uint, index, field, query string, startAt, endAt int64, limit int) ([]map[string]any, error) {
	if datasourceID == 0 {
		return nil, errors.New("请选择日志数据源")
	}
	field = strings.TrimSpace(field)
	if !isCommonMonitorLogField(field) {
		ds, err := s.GetMonitorDatasource(datasourceID)
		if err != nil {
			return nil, err
		}
		if normalizeMonitorDatasourceType(ds.Type) == "victorialogs" && isSafeVictoriaLogsField(field) {
			return s.ListMonitorVictoriaLogsStreams(datasourceID, field, query, startAt, endAt, limit)
		}
	}
	if !isCommonMonitorLogField(field) {
		return nil, errors.New("不支持的日志筛选字段")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	switch normalizeMonitorDatasourceType(ds.Type) {
	case "elasticsearch":
		return s.elasticsearchLogFieldValues(*ds, index, field, query, startAt, endAt, limit)
	case "victorialogs":
		return s.ListMonitorVictoriaLogsStreams(datasourceID, field, query, startAt, endAt, limit)
	default:
		return nil, errors.New("日志字段仅支持 Elasticsearch 或 VictoriaLogs 数据源")
	}
}

func normalizeMonitorLogFieldValueLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func isCommonMonitorLogField(field string) bool {
	for _, item := range []string{
		"kubernetes.pod_namespace", "kubernetes.pod_name", "kubernetes.container_name", "kafka_topic", "level",
	} {
		if field == item {
			return true
		}
	}
	return false
}

func isSafeVictoriaLogsField(field string) bool {
	if field == "" || len(field) > 128 {
		return false
	}
	for _, char := range field {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '.' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func (s *Service) elasticsearchLogFieldValues(ds model.MonitorDatasource, index, field, query string, startAt, endAt int64, limit int) ([]map[string]any, error) {
	index = firstNonEmpty(strings.TrimSpace(index), "_all")
	if strings.Contains(index, "/") || strings.Contains(index, "\\") {
		return nil, errors.New("索引不能包含路径分隔符")
	}
	endAt = firstNonEmptyInt64(endAt, time.Now().UnixMilli())
	if startAt <= 0 || startAt >= endAt {
		startAt = time.UnixMilli(endAt).Add(-time.Hour).UnixMilli()
	}
	must := []any{map[string]any{"match_all": map[string]any{}}}
	if strings.TrimSpace(query) != "" {
		must = []any{map[string]any{"query_string": map[string]any{"query": strings.TrimSpace(query), "analyze_wildcard": true}}}
	}
	search := func(aggregationField string) ([]map[string]any, error) {
		body := map[string]any{
			"size": 0,
			"query": map[string]any{
				"bool": map[string]any{
					"must": must,
					"filter": []any{map[string]any{
						"range": map[string]any{
							"@timestamp": map[string]any{"gte": startAt, "lte": endAt, "format": "epoch_millis"},
						},
					}},
				},
			},
			"aggs": map[string]any{
				"values": map[string]any{
					"terms": map[string]any{"field": aggregationField, "size": 100, "order": map[string]any{"_count": "desc"}},
				},
			},
		}
		response, err := s.elasticsearchSearch(ds, index, body)
		if err != nil {
			return nil, err
		}
		aggs, _ := response["aggregations"].(map[string]any)
		values, _ := aggs["values"].(map[string]any)
		buckets, _ := values["buckets"].([]any)
		items := make([]map[string]any, 0, len(buckets))
		for _, raw := range buckets {
			bucket, _ := raw.(map[string]any)
			value := monitorSourceString(bucket["key"])
			if value != "" {
				items = append(items, map[string]any{"value": value, "hits": bucket["doc_count"], "field": field})
			}
		}
		return items, nil
	}
	var lastErr error
	for _, aggregationField := range []string{field, field + ".keyword"} {
		items, err := search(aggregationField)
		if err != nil {
			lastErr = err
			continue
		}
		if len(items) > 0 {
			return items, nil
		}
	}

	// Some log indices keep these fields in _source without a terms-aggregatable
	// mapping. Fall back to the current matching documents so the selector stays
	// consistent with the log rows displayed in the workbench.
	body := map[string]any{
		"size":    1000,
		"_source": []string{field},
		"query": map[string]any{
			"bool": map[string]any{
				"must": must,
				"filter": []any{map[string]any{
					"range": map[string]any{
						"@timestamp": map[string]any{"gte": startAt, "lte": endAt, "format": "epoch_millis"},
					},
				}},
			},
		},
	}
	response, err := s.elasticsearchSearch(ds, index, body)
	if err != nil {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, err
	}
	hits, _ := response["hits"].(map[string]any)
	rawItems, _ := hits["hits"].([]any)
	counts := make(map[string]int64)
	for _, rawItem := range rawItems {
		hit, _ := rawItem.(map[string]any)
		source, _ := hit["_source"].(map[string]any)
		if value := monitorLogFieldValue(source, field); value != "" {
			counts[value]++
		}
	}
	items := make([]map[string]any, 0, len(counts))
	for value, hits := range counts {
		items = append(items, map[string]any{"value": value, "hits": hits, "field": field})
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := items[i]["hits"].(int64)
		right, _ := items[j]["hits"].(int64)
		if left == right {
			return monitorSourceString(items[i]["value"]) < monitorSourceString(items[j]["value"])
		}
		return left > right
	})
	limit = normalizeMonitorLogFieldValueLimit(limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func firstNonEmptyInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func (s *Service) elasticsearchLogFields(ds model.MonitorDatasource, index string) ([]map[string]any, error) {
	index = firstNonEmpty(strings.TrimSpace(index), "_all")
	endpoint := strings.TrimRight(ds.URL, "/") + "/_mapping"
	if index != "_all" && index != "*" {
		endpoint = strings.TrimRight(ds.URL, "/") + "/" + url.PathEscape(index) + "/_mapping"
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 Elasticsearch 字段失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	var mappings map[string]struct {
		Mappings struct {
			Properties map[string]any `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(body, &mappings); err != nil {
		return nil, err
	}
	fieldTypes := map[string]string{}
	for _, mapping := range mappings {
		collectElasticsearchFields("", mapping.Mappings.Properties, fieldTypes)
	}
	return monitorLogFields(fieldTypes), nil
}

func collectElasticsearchFields(prefix string, properties map[string]any, fieldTypes map[string]string) {
	for name, raw := range properties {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		config, _ := raw.(map[string]any)
		fieldType := monitorSourceString(config["type"])
		if fieldType == "" {
			fieldType = "object"
		}
		fieldTypes[path] = fieldType
		if children, ok := config["properties"].(map[string]any); ok {
			collectElasticsearchFields(path, children, fieldTypes)
		}
	}
}

func (s *Service) victoriaLogsFields(ds model.MonitorDatasource, query string, startAt, endAt int64) ([]map[string]any, error) {
	end := time.UnixMilli(endAt)
	if endAt <= 0 {
		end = time.Now()
	}
	start := time.UnixMilli(startAt)
	if startAt <= 0 || startAt >= end.UnixMilli() {
		start = end.Add(-time.Hour)
	}
	form := url.Values{}
	form.Set("query", firstNonEmpty(strings.TrimSpace(query), "*"))
	form.Set("start", start.UTC().Format(time.RFC3339Nano))
	form.Set("end", end.UTC().Format(time.RFC3339Nano))
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/field_names", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 VictoriaLogs 字段失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	var raw struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	fieldTypes := map[string]string{}
	for _, field := range raw.Fields {
		field = strings.TrimSpace(field)
		if field != "" {
			fieldTypes[field] = "field"
		}
	}
	return monitorLogFields(fieldTypes), nil
}

func monitorLogFields(fieldTypes map[string]string) []map[string]any {
	items := make([]map[string]any, 0, len(fieldTypes))
	for name, fieldType := range fieldTypes {
		items = append(items, map[string]any{"name": name, "type": fieldType})
	}
	sort.Slice(items, func(i, j int) bool {
		return monitorSourceString(items[i]["name"]) < monitorSourceString(items[j]["name"])
	})
	if len(items) > 500 {
		items = items[:500]
	}
	return items
}

func (s *Service) ListMonitorElasticsearchIndices(datasourceID uint) ([]map[string]any, error) {
	if datasourceID == 0 {
		return nil, errors.New("请选择 Elasticsearch 数据源")
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	if normalizeMonitorDatasourceType(ds.Type) != "elasticsearch" {
		return nil, errors.New("当前数据源不是 Elasticsearch")
	}
	endpoint := strings.TrimRight(ds.URL, "/") + "/_cat/indices?format=json&h=health,status,index,docs.count,store.size&s=index"
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyMonitorDatasourceAuth(request, *ds)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 Elasticsearch 索引失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		name := monitorSourceString(item["index"])
		if name == "" || strings.HasPrefix(name, ".security") {
			continue
		}
		result = append(result, map[string]any{
			"name": name, "health": monitorSourceString(item["health"]), "status": monitorSourceString(item["status"]),
			"docsCount": monitorSourceString(item["docs.count"]), "storeSize": monitorSourceString(item["store.size"]),
		})
	}
	return result, nil
}

func (s *Service) ListMonitorLogShortcuts(owner string) ([]model.MonitorLogShortcut, error) {
	owner = firstNonEmpty(strings.TrimSpace(owner), "admin")
	var count int64
	if err := s.db.Model(&model.MonitorLogShortcut{}).Where("owner = ?", owner).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		defaults := []struct {
			name, query, index, rangeText string
		}{
			{"全部日志", "", "_all", "24h"},
			{"错误日志", "ERROR", "_all", "24h"},
			{"异常与堆栈", "(Exception OR ERROR OR Caused\\ by)", "_all", "24h"},
			{"告警与警告", "(WARN OR WARNING)", "_all", "24h"},
			{"超时请求", "(timeout OR timed\\ out OR TimeoutException)", "_all", "24h"},
			{"连接失败", "(connection\\ refused OR connection\\ reset OR connect\\ timeout)", "_all", "24h"},
			{"Kubernetes 重启", "(CrashLoopBackOff OR OOMKilled OR Back-off\\ restarting)", "_all", "24h"},
			{"应用启动", "(Started\\ .*Application OR application\\ started)", "_all", "24h"},
			{"数据库慢查询", "(slow\\ query OR SlowQuery OR SQL\\ took)", "_all", "24h"},
			{"指定命名空间", "kubernetes.pod_namespace:\"default\"", "_all", "6h"},
		}
		items := make([]model.MonitorLogShortcut, 0, len(defaults))
		for i, item := range defaults {
			items = append(items, model.MonitorLogShortcut{Owner: owner, Name: item.name, Query: item.query, IndexName: item.index, TimeRange: item.rangeText, Sort: i + 1})
		}
		if err := s.db.Create(&items).Error; err != nil {
			return nil, err
		}
	}
	var list []model.MonitorLogShortcut
	if err := s.db.Where("owner = ?", owner).Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) SaveMonitorLogShortcut(owner string, payload MonitorLogShortcutPayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("快捷语句名称不能为空")
	}
	owner = firstNonEmpty(strings.TrimSpace(owner), "admin")
	updates := map[string]any{
		"name": Trimmed(payload.Name), "query": strings.TrimSpace(payload.Query),
		"index_name": firstNonEmpty(strings.TrimSpace(payload.IndexName), "_all"),
		"time_range": firstNonEmpty(strings.TrimSpace(payload.TimeRange), "24h"), "sort": payload.Sort,
	}
	if payload.ID > 0 {
		return s.db.Model(&model.MonitorLogShortcut{}).Where("id = ? AND owner = ?", payload.ID, owner).Updates(updates).Error
	}
	return s.db.Create(&model.MonitorLogShortcut{Owner: owner, Name: updates["name"].(string), Query: updates["query"].(string), IndexName: updates["index_name"].(string), TimeRange: updates["time_range"].(string), Sort: payload.Sort}).Error
}

func (s *Service) DeleteMonitorLogShortcut(owner string, id uint) error {
	if id == 0 {
		return errors.New("请选择快捷语句")
	}
	owner = firstNonEmpty(strings.TrimSpace(owner), "admin")
	result := s.db.Where("id = ? AND owner = ?", id, owner).Delete(&model.MonitorLogShortcut{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("快捷语句不存在或无权删除")
	}
	return nil
}

func (s *Service) ListMonitorLogShortcutsByType(owner, datasourceType string) ([]model.MonitorLogShortcut, error) {
	owner = firstNonEmpty(strings.TrimSpace(owner), "admin")
	datasourceType = normalizeLogShortcutDatasourceType(datasourceType)
	query := s.db.Where("owner = ?", owner)
	if datasourceType == "victorialogs" {
		query = query.Where("datasource_type = ?", datasourceType)
	} else {
		query = query.Where("datasource_type = ? OR datasource_type = ''", datasourceType)
	}
	var count int64
	if err := query.Model(&model.MonitorLogShortcut{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		defaults := monitorLogShortcutDefaults(datasourceType)
		items := make([]model.MonitorLogShortcut, 0, len(defaults))
		for i, item := range defaults {
			items = append(items, model.MonitorLogShortcut{Owner: owner, DatasourceType: datasourceType, Name: item.name, Query: item.query, IndexName: item.indexName, TimeRange: item.timeRange, Sort: i + 1})
		}
		if len(items) > 0 {
			if err := s.db.Create(&items).Error; err != nil {
				return nil, err
			}
		}
	}
	query = s.db.Where("owner = ?", owner)
	if datasourceType == "victorialogs" {
		query = query.Where("datasource_type = ?", datasourceType)
	} else {
		query = query.Where("datasource_type = ? OR datasource_type = ''", datasourceType)
	}
	var list []model.MonitorLogShortcut
	if err := query.Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) SaveMonitorLogShortcutByType(owner string, payload MonitorLogShortcutPayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("快捷语句名称不能为空")
	}
	owner = firstNonEmpty(strings.TrimSpace(owner), "admin")
	datasourceType := normalizeLogShortcutDatasourceType(payload.DatasourceType)
	updates := map[string]any{
		"datasource_type": datasourceType,
		"name":            Trimmed(payload.Name),
		"query":           strings.TrimSpace(payload.Query),
		"index_name":      firstNonEmpty(strings.TrimSpace(payload.IndexName), "_all"),
		"time_range":      firstNonEmpty(strings.TrimSpace(payload.TimeRange), "1h"),
		"sort":            payload.Sort,
	}
	if payload.ID > 0 {
		return s.db.Model(&model.MonitorLogShortcut{}).Where("id = ? AND owner = ?", payload.ID, owner).Updates(updates).Error
	}
	return s.db.Create(&model.MonitorLogShortcut{Owner: owner, DatasourceType: datasourceType, Name: updates["name"].(string), Query: updates["query"].(string), IndexName: updates["index_name"].(string), TimeRange: updates["time_range"].(string), Sort: payload.Sort}).Error
}

type monitorLogShortcutDefault struct {
	name, query, indexName, timeRange string
}

func normalizeLogShortcutDatasourceType(value string) string {
	if normalizeMonitorDatasourceType(value) == "victorialogs" {
		return "victorialogs"
	}
	return "elasticsearch"
}

func monitorLogShortcutDefaults(datasourceType string) []monitorLogShortcutDefault {
	if datasourceType == "victorialogs" {
		return []monitorLogShortcutDefault{
			{"全部日志", "*", "_all", "1h"},
			{"错误日志", "_msg:error", "_all", "1h"},
			{"异常与堆栈", "_msg:(Exception OR ERROR OR Caused)", "_all", "6h"},
			{"告警与警告", "_msg:(WARN OR WARNING)", "_all", "6h"},
			{"超时请求", "_msg:(timeout OR timed OR TimeoutException)", "_all", "6h"},
			{"连接失败", "_msg:(connection refused OR connection reset OR connect timeout)", "_all", "6h"},
			{"Kubernetes 重启", "_msg:(CrashLoopBackOff OR OOMKilled OR Back-off)", "_all", "24h"},
			{"应用启动", "_msg:(Started OR application started)", "_all", "24h"},
			{"指定命名空间", "kubernetes.pod_namespace:default", "_all", "6h"},
			{"Kafka 错误主题", "kafka_topic:* AND _msg:error", "_all", "1h"},
		}
	}
	return []monitorLogShortcutDefault{
		{"全部日志", "", "_all", "1h"},
		{"错误日志", "ERROR", "_all", "1h"},
		{"异常与堆栈", "(Exception OR ERROR OR Caused\\ by)", "_all", "6h"},
		{"告警与警告", "(WARN OR WARNING)", "_all", "6h"},
		{"超时请求", "(timeout OR timed\\ out OR TimeoutException)", "_all", "6h"},
		{"连接失败", "(connection\\ refused OR connection\\ reset OR connect\\ timeout)", "_all", "6h"},
		{"Kubernetes 重启", "(CrashLoopBackOff OR OOMKilled OR Back-off\\ restarting)", "_all", "24h"},
		{"应用启动", "(Started\\ .*Application OR application\\ started)", "_all", "24h"},
		{"数据库慢查询", "(slow\\ query OR SlowQuery OR SQL\\ took)", "_all", "24h"},
		{"指定命名空间", "kubernetes.pod_namespace:\"default\"", "_all", "6h"},
	}
}

func (s *Service) elasticsearchSearch(ds model.MonitorDatasource, index string, payload map[string]any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(ds.URL, "/") + "/" + url.PathEscape(index) + "/_search"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Elasticsearch 查询失败，状态码 %d: %s", response.StatusCode, string(responseBody))
	}
	var result map[string]any
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func cleanMonitorLogMessage(value string) string {
	value = strings.TrimSpace(value)
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	return ansi.ReplaceAllString(value, "")
}

var monitorOpLogHiddenContentFields = map[string]struct{}{
	"@timestamp":  {},
	"source_type": {},
	"timestamp":   {},
	"ts":          {},
}

func formatMonitorOpLogContent(source map[string]any, index, rawMessage string) string {
	var messageDocument map[string]any
	if strings.TrimSpace(rawMessage) != "" {
		_ = json.Unmarshal([]byte(rawMessage), &messageDocument)
	}

	if !isMonitorOpLogDocument(source, index) && !isMonitorOpLogDocument(messageDocument, index) {
		return ""
	}
	document := messageDocument
	if len(document) == 0 {
		document = source
	}
	if len(document) == 0 {
		return ""
	}

	content := make(map[string]any, len(document))
	for name, value := range document {
		if _, hidden := monitorOpLogHiddenContentFields[name]; hidden {
			continue
		}
		content[name] = value
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func isMonitorOpLogDocument(source map[string]any, index string) bool {
	indexName := strings.ToLower(strings.TrimSpace(index))
	if strings.Contains(indexName, "oplog") || strings.Contains(indexName, "op-log") {
		return true
	}
	if len(source) == 0 {
		return false
	}
	topic := strings.ToLower(monitorLogFieldValue(source, "kafka_topic"))
	logType := strings.ToLower(monitorLogFieldValue(source, "log_type"))
	return strings.Contains(topic, "op.log") || logType == "op"
}

func parseMonitorLogMessage(value string) map[string]string {
	result := map[string]string{"content": value}
	matches := springLogPattern.FindStringSubmatch(value)
	if len(matches) != 7 {
		return result
	}
	result["timestamp"] = matches[1]
	result["level"] = matches[2]
	result["processId"] = matches[3]
	result["thread"] = matches[4]
	result["logger"] = strings.TrimSpace(matches[5])
	result["content"] = strings.TrimSpace(matches[6])
	return result
}

func monitorSourceString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func detectMonitorLogLevel(message string) string {
	upper := strings.ToUpper(message)
	for _, level := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE"} {
		if strings.Contains(upper, level) {
			return level
		}
	}
	return "-"
}

func applyMonitorDatasourceAuth(request *http.Request, ds model.MonitorDatasource) {
	switch normalizeMonitorAuthType(ds.AuthType) {
	case "basic":
		request.SetBasicAuth(ds.Username, ds.Password)
	case "bearer":
		if strings.TrimSpace(ds.Token) != "" {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(ds.Token))
		}
	case "apikey":
		if strings.TrimSpace(ds.Token) != "" {
			request.Header.Set("Authorization", "ApiKey "+strings.TrimSpace(ds.Token))
		}
	}
}

type prometheusRuleDocument struct {
	Groups []struct {
		Name     string `yaml:"name"`
		Interval string `yaml:"interval"`
		Rules    []struct {
			Alert       string         `yaml:"alert"`
			Record      string         `yaml:"record"`
			Expr        yaml.Node      `yaml:"expr"`
			For         string         `yaml:"for"`
			Labels      map[string]any `yaml:"labels"`
			Annotations map[string]any `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

type prometheusRuleExportDocument struct {
	Groups []prometheusRuleExportGroup `yaml:"groups"`
}

type prometheusRuleExportGroup struct {
	Name     string                     `yaml:"name"`
	Interval string                     `yaml:"interval,omitempty"`
	Rules    []prometheusRuleExportItem `yaml:"rules"`
}

type prometheusRuleExportItem struct {
	Alert       string         `yaml:"alert"`
	Expr        string         `yaml:"expr"`
	For         string         `yaml:"for,omitempty"`
	Labels      map[string]any `yaml:"labels,omitempty"`
	Annotations map[string]any `yaml:"annotations,omitempty"`
}

func parsePrometheusTemplateDuration(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parts := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)(ms|s|m|h|d|w|y)`).FindAllStringSubmatch(value, -1)
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	consumed := ""
	total := float64(0)
	seconds := map[string]float64{"ms": .001, "s": 1, "m": 60, "h": 3600, "d": 86400, "w": 604800, "y": 31536000}
	for _, part := range parts {
		consumed += part[0]
		number, err := strconv.ParseFloat(part[1], 64)
		if err != nil {
			return 0, err
		}
		total += number * seconds[strings.ToLower(part[2])]
	}
	if !strings.EqualFold(consumed, value) {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return int(total), nil
}

func jsonObjectText(value map[string]any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func importedSeverity(labels map[string]any) string {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(labels["severity"])))
	switch value {
	case "p0", "disaster", "emergency", "page":
		return "P0"
	case "p1", "critical", "fatal":
		return "P1"
	case "p3", "info", "notice":
		return "P3"
	default:
		return "P2"
	}
}

func splitPrometheusTemplateExpression(expression string) (string, string, float64) {
	expression = strings.TrimSpace(expression)
	matcher := regexp.MustCompile(`(?s)^(.+?)\s*(==|!=|>=|<=|>|<)\s*(?:bool\s+)?(-?(?:\d+(?:\.\d*)?|\.\d+))\s*$`)
	parts := matcher.FindStringSubmatch(expression)
	if len(parts) != 4 {
		return expression, ">", 0
	}
	threshold, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return expression, ">", 0
	}
	return strings.TrimSpace(parts[1]), parts[2], threshold
}

func ParsePrometheusAlertTemplates(content []byte) ([]MonitorAlertTemplateImportItem, error) {
	var document prometheusRuleDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("Prometheus YAML 解析失败：%w", err)
	}
	if len(document.Groups) == 0 {
		return nil, errors.New("未发现 groups；请选择 Prometheus Rule YAML 文件")
	}
	items := make([]MonitorAlertTemplateImportItem, 0)
	for _, group := range document.Groups {
		interval, err := parsePrometheusTemplateDuration(group.Interval)
		if err != nil {
			return nil, fmt.Errorf("规则组 %s 的 interval 无效：%w", group.Name, err)
		}
		if interval < 15 {
			interval = 60
		}
		for _, rule := range group.Rules {
			if strings.TrimSpace(rule.Alert) == "" {
				continue // recording rules are not alert templates
			}
			expression := strings.TrimSpace(rule.Expr.Value)
			if expression == "" {
				return nil, fmt.Errorf("规则 %s 缺少 expr", rule.Alert)
			}
			duration, err := parsePrometheusTemplateDuration(rule.For)
			if err != nil {
				return nil, fmt.Errorf("规则 %s 的 for 无效：%w", rule.Alert, err)
			}
			queryText, comparator, threshold := splitPrometheusTemplateExpression(expression)
			description := strings.TrimSpace(fmt.Sprint(rule.Annotations["description"]))
			if description == "<nil>" || description == "" {
				description = strings.TrimSpace(fmt.Sprint(rule.Annotations["summary"]))
			}
			if description == "<nil>" {
				description = ""
			}
			items = append(items, MonitorAlertTemplateImportItem{
				Name: strings.TrimSpace(rule.Alert), PrometheusGroup: strings.TrimSpace(group.Name), OriginalExpression: expression,
				QueryText: queryText, Comparator: comparator, Threshold: threshold, ForSeconds: duration,
				EvalIntervalSeconds: interval, Severity: importedSeverity(rule.Labels), LabelsJSON: jsonObjectText(rule.Labels),
				AnnotationsJSON: jsonObjectText(rule.Annotations), Description: description,
			})
		}
	}
	if len(items) == 0 {
		return nil, errors.New("文件中没有 alert 规则；recording rule 不会导入为告警模板")
	}
	return items, nil
}

func (s *Service) ImportPrometheusAlertTemplates(payload MonitorAlertTemplateImportPayload) (map[string]any, error) {
	if payload.GroupID == 0 {
		return nil, errors.New("请选择目标模板分组")
	}
	if len(payload.Items) == 0 {
		return nil, errors.New("请选择至少一条 Prometheus 告警规则")
	}
	if len(payload.Items) > 500 {
		return nil, errors.New("单次最多导入 500 条告警规则")
	}
	category, collector, err := s.monitorAlertTemplateGroupMeta(payload.GroupID)
	if err != nil {
		return nil, err
	}
	strategy := strings.ToLower(strings.TrimSpace(payload.DuplicateStrategy))
	if strategy != "rename" {
		strategy = "skip"
	}
	created, skipped := 0, 0
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, source := range payload.Items {
			name := Trimmed(source.Name)
			queryText := strings.TrimSpace(source.QueryText)
			if name == "" || queryText == "" {
				return errors.New("导入项的名称和查询语句不能为空")
			}
			var count int64
			if err := tx.Model(&model.MonitorAlertTemplate{}).Where("group_id = ? AND name = ?", payload.GroupID, name).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 && strategy == "skip" {
				skipped++
				continue
			}
			if count > 0 {
				base := name
				for index := 2; count > 0; index++ {
					name = fmt.Sprintf("%s（导入 %d）", base, index)
					if err := tx.Model(&model.MonitorAlertTemplate{}).Where("group_id = ? AND name = ?", payload.GroupID, name).Count(&count).Error; err != nil {
						return err
					}
				}
			}
			labels := strings.TrimSpace(source.LabelsJSON)
			annotations := strings.TrimSpace(source.AnnotationsJSON)
			if !json.Valid([]byte(labels)) || !json.Valid([]byte(annotations)) {
				return fmt.Errorf("规则 %s 的标签或注解不是有效 JSON", name)
			}
			item := model.MonitorAlertTemplate{
				GroupID: payload.GroupID, Name: name, Category: category, Collector: collector, ObjectType: "",
				DatasourceType: "prometheus", QueryText: queryText, Comparator: firstNonEmpty(source.Comparator, ">"), Threshold: source.Threshold,
				ForSeconds: source.ForSeconds, EvalIntervalSeconds: source.EvalIntervalSeconds, Severity: firstNonEmpty(source.Severity, "P2"),
				LabelsJSON: labels, AnnotationsJSON: annotations, Description: Trimmed(source.Description), Source: "custom", Status: 1,
			}
			if item.ForSeconds < 0 {
				item.ForSeconds = 0
			}
			if item.EvalIntervalSeconds < 15 {
				item.EvalIntervalSeconds = 60
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			created++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"created": created, "skipped": skipped, "total": len(payload.Items)}, nil
}

func prometheusExportDuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}

func prometheusExportObject(raw string) map[string]any {
	values := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &values)
	}
	return values
}

// ExportPrometheusAlertTemplates emits standard Prometheus Rule YAML, which can be
// pasted back into the template import dialog of another Ops Admin instance.
func (s *Service) ExportPrometheusAlertTemplates(ids []uint) ([]byte, error) {
	ids, err := normalizeMonitorBatchIDs(ids)
	if err != nil {
		return nil, errors.New("请选择至少一条告警模板")
	}
	if len(ids) > 500 {
		return nil, errors.New("单次最多导出 500 条告警模板")
	}

	var templates []model.MonitorAlertTemplate
	if err := s.db.Where("id IN ?", ids).Find(&templates).Error; err != nil {
		return nil, err
	}
	if len(templates) != len(ids) {
		return nil, errors.New("部分告警模板不存在或已被删除")
	}
	for _, item := range templates {
		if item.DatasourceType != "prometheus" && item.DatasourceType != "victoriametrics" {
			return nil, fmt.Errorf("模板「%s」不是 Prometheus/VictoriaMetrics 类型，不能导出为 Prometheus Rule YAML", item.Name)
		}
	}

	var groups []model.MonitorAlertTemplateGroup
	if err := s.db.Find(&groups).Error; err != nil {
		return nil, err
	}
	groupByID := make(map[uint]model.MonitorAlertTemplateGroup, len(groups))
	for _, group := range groups {
		groupByID[group.ID] = group
	}
	groupPath := func(id uint) string {
		parts := make([]string, 0, 2)
		for group, ok := groupByID[id]; ok; group, ok = groupByID[group.ParentID] {
			parts = append([]string{group.Name}, parts...)
			if group.ParentID == 0 {
				break
			}
		}
		return strings.Join(parts, " / ")
	}

	sort.Slice(templates, func(i, j int) bool {
		if templates[i].GroupID == templates[j].GroupID {
			return templates[i].Name < templates[j].Name
		}
		return templates[i].GroupID < templates[j].GroupID
	})
	document := prometheusRuleExportDocument{}
	groupIndexes := map[string]int{}
	for _, item := range templates {
		interval := item.EvalIntervalSeconds
		if interval < 15 {
			interval = 60
		}
		name := groupPath(item.GroupID)
		if name == "" {
			name = "ops-admin-export"
		}
		key := fmt.Sprintf("%d/%d", item.GroupID, interval)
		index, ok := groupIndexes[key]
		if !ok {
			index = len(document.Groups)
			groupIndexes[key] = index
			document.Groups = append(document.Groups, prometheusRuleExportGroup{Name: name, Interval: prometheusExportDuration(interval)})
		}
		labels := prometheusExportObject(item.LabelsJSON)
		if monitorSourceString(labels["severity"]) == "" {
			labels["severity"] = strings.ToLower(item.Severity)
		}
		annotations := prometheusExportObject(item.AnnotationsJSON)
		if monitorSourceString(annotations["description"]) == "" && strings.TrimSpace(item.Description) != "" {
			annotations["description"] = item.Description
		}
		expression := strings.TrimSpace(item.QueryText)
		if item.Comparator != "" {
			expression += " " + item.Comparator + " " + strconv.FormatFloat(item.Threshold, 'f', -1, 64)
		}
		document.Groups[index].Rules = append(document.Groups[index].Rules, prometheusRuleExportItem{
			Alert: item.Name, Expr: expression, For: prometheusExportDuration(item.ForSeconds), Labels: labels, Annotations: annotations,
		})
	}
	content, err := yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	return append([]byte("# Exported by Ops Admin alert template library.\n# Paste this content into: 告警模板 > 粘贴 Prometheus 模板。\n\n"), content...), nil
}

func (s *Service) ListMonitorAlertTemplates(pageNum, pageSize int, keyword, category, datasourceType, source string, groupID uint) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorAlertTemplate{})
	if value := strings.TrimSpace(keyword); value != "" {
		like := "%" + value + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR collector LIKE ? OR query_text LIKE ?", like, like, like, like)
	}
	if value := strings.TrimSpace(category); value != "" {
		query = query.Where("category = ?", value)
	}
	if value := strings.TrimSpace(datasourceType); value != "" {
		query = query.Where("datasource_type = ?", value)
	}
	if value := strings.TrimSpace(source); value != "" {
		query = query.Where("source = ?", value)
	}
	if groupID > 0 {
		var groups []model.MonitorAlertTemplateGroup
		if err := s.db.Find(&groups).Error; err != nil {
			return nil, err
		}
		children := make(map[uint][]uint)
		for _, item := range groups {
			children[item.ParentID] = append(children[item.ParentID], item.ID)
		}
		ids := []uint{groupID}
		for index := 0; index < len(ids); index++ {
			ids = append(ids, children[ids[index]]...)
		}
		query = query.Where("group_id IN ?", ids)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorAlertTemplate
	if err := query.Order("source ASC, category ASC, id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListMonitorAlertTemplateGroups() ([]map[string]any, error) {
	var items []model.MonitorAlertTemplateGroup
	if err := s.db.Order("parent_id ASC, sort ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	var counts []struct {
		GroupID uint
		Count   int64
	}
	_ = s.db.Model(&model.MonitorAlertTemplate{}).Select("group_id, COUNT(*) AS count").Group("group_id").Scan(&counts).Error
	countMap := map[uint]int64{}
	for _, item := range counts {
		countMap[item.GroupID] = item.Count
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, map[string]any{"id": item.ID, "parentId": item.ParentID, "name": item.Name, "count": countMap[item.ID]})
	}
	return rows, nil
}

func (s *Service) SaveMonitorAlertTemplateGroup(payload MonitorAlertTemplateGroupPayload) (*model.MonitorAlertTemplateGroup, error) {
	name := Trimmed(payload.Name)
	if name == "" {
		return nil, errors.New("group name is required")
	}
	if payload.ID > 0 {
		var current model.MonitorAlertTemplateGroup
		if err := s.db.First(&current, payload.ID).Error; err != nil {
			return nil, err
		}
		var duplicate int64
		if err := s.db.Model(&model.MonitorAlertTemplateGroup{}).Where("parent_id = ? AND name = ? AND id <> ?", current.ParentID, name, payload.ID).Count(&duplicate).Error; err != nil {
			return nil, err
		}
		if duplicate > 0 {
			return nil, errors.New("a group with the same name already exists at this level")
		}
		if err := s.db.Model(&model.MonitorAlertTemplateGroup{}).Where("id = ?", payload.ID).Updates(map[string]any{"name": name}).Error; err != nil {
			return nil, err
		}
		var item model.MonitorAlertTemplateGroup
		err := s.db.First(&item, payload.ID).Error
		return &item, err
	}
	if payload.ParentID > 0 {
		var parent model.MonitorAlertTemplateGroup
		if err := s.db.First(&parent, payload.ParentID).Error; err != nil {
			return nil, errors.New("parent group does not exist")
		}
		if parent.ParentID > 0 {
			return nil, errors.New("template groups support two levels only")
		}
	}
	var duplicate int64
	if err := s.db.Model(&model.MonitorAlertTemplateGroup{}).Where("parent_id = ? AND name = ?", payload.ParentID, name).Count(&duplicate).Error; err != nil {
		return nil, err
	}
	if duplicate > 0 {
		return nil, errors.New("a group with the same name already exists at this level")
	}
	item := model.MonitorAlertTemplateGroup{ParentID: payload.ParentID, Name: name}
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) DeleteMonitorAlertTemplateGroup(id uint) error {
	var childCount, templateCount int64
	_ = s.db.Model(&model.MonitorAlertTemplateGroup{}).Where("parent_id = ?", id).Count(&childCount).Error
	_ = s.db.Model(&model.MonitorAlertTemplate{}).Where("group_id = ?", id).Count(&templateCount).Error
	if childCount > 0 || templateCount > 0 {
		return errors.New("group contains child groups or templates and cannot be deleted")
	}
	return s.db.Delete(&model.MonitorAlertTemplateGroup{}, id).Error
}

func (s *Service) monitorAlertTemplateGroupMeta(groupID uint) (string, string, error) {
	var groups []model.MonitorAlertTemplateGroup
	if err := s.db.Find(&groups).Error; err != nil {
		return "", "", err
	}
	byID := make(map[uint]model.MonitorAlertTemplateGroup, len(groups))
	for _, item := range groups {
		byID[item.ID] = item
	}
	path := []model.MonitorAlertTemplateGroup{}
	for currentID := groupID; currentID > 0; {
		item, ok := byID[currentID]
		if !ok {
			return "", "", errors.New("template group does not exist")
		}
		path = append([]model.MonitorAlertTemplateGroup{item}, path...)
		currentID = item.ParentID
	}
	if len(path) < 2 {
		return "", "", errors.New("select a collector child group under the component category")
	}
	return path[0].Name, path[1].Name, nil
}

func (s *Service) GetMonitorAlertTemplate(id uint) (*model.MonitorAlertTemplate, error) {
	var item model.MonitorAlertTemplate
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) SaveMonitorAlertTemplate(payload MonitorAlertTemplatePayload) (*model.MonitorAlertTemplate, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.New("template name is required")
	}
	if payload.GroupID == 0 {
		return nil, errors.New("template group is required")
	}
	category, collector, err := s.monitorAlertTemplateGroupMeta(payload.GroupID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.QueryText) == "" {
		return nil, errors.New("query text is required")
	}
	updates := map[string]any{
		"group_id": payload.GroupID,
		"name":     Trimmed(payload.Name), "category": category, "collector": collector,
		"object_type": "", "datasource_type": Trimmed(payload.DatasourceType), "query_text": strings.TrimSpace(payload.QueryText),
		"comparator": firstNonEmpty(Trimmed(payload.Comparator), ">"), "threshold": payload.Threshold, "for_seconds": payload.ForSeconds,
		"eval_interval_seconds": payload.EvalIntervalSeconds, "severity": firstNonEmpty(Trimmed(payload.Severity), "P2"),
		"labels_json": strings.TrimSpace(payload.LabelsJSON), "annotations_json": strings.TrimSpace(payload.AnnotationsJSON),
		"description": Trimmed(payload.Description), "status": normalizeMonitorStatus(payload.Status),
	}
	if payload.ID > 0 {
		current, err := s.GetMonitorAlertTemplate(payload.ID)
		if err != nil {
			return nil, err
		}
		if current.Source == "platform" {
			return nil, errors.New("platform template is read-only; copy it before editing")
		}
		if err := s.db.Model(&model.MonitorAlertTemplate{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
		return s.GetMonitorAlertTemplate(payload.ID)
	}
	item := model.MonitorAlertTemplate{
		GroupID: payload.GroupID,
		Name:    updates["name"].(string), Category: updates["category"].(string), Collector: updates["collector"].(string),
		ObjectType: updates["object_type"].(string), DatasourceType: updates["datasource_type"].(string), QueryText: updates["query_text"].(string),
		Comparator: updates["comparator"].(string), Threshold: payload.Threshold, ForSeconds: payload.ForSeconds,
		EvalIntervalSeconds: payload.EvalIntervalSeconds, Severity: updates["severity"].(string), LabelsJSON: strings.TrimSpace(payload.LabelsJSON),
		AnnotationsJSON: strings.TrimSpace(payload.AnnotationsJSON), Description: updates["description"].(string), Status: normalizeMonitorStatus(payload.Status), Source: "custom",
	}
	if item.ForSeconds < 0 {
		item.ForSeconds = 0
	}
	if item.EvalIntervalSeconds < 15 {
		item.EvalIntervalSeconds = 60
	}
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) DeleteMonitorAlertTemplate(id uint) error {
	item, err := s.GetMonitorAlertTemplate(id)
	if err != nil {
		return err
	}
	if item.Source == "platform" {
		return errors.New("platform template cannot be deleted")
	}
	return s.db.Delete(&model.MonitorAlertTemplate{}, id).Error
}

func (s *Service) ListMonitorAlertRules(pageNum, pageSize int, keyword, status, severity, env, alertType string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorAlertRule{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR prom_ql LIKE ? OR description LIKE ?", like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		switch strings.TrimSpace(status) {
		case "unclaimed":
			query = query.Where("status IN ? AND (claimed_by = '' OR claimed_by IS NULL)", []string{"pending", "firing"})
		default:
			query = query.Where("status = ?", status)
		}
	}
	if strings.TrimSpace(severity) != "" {
		if strings.EqualFold(strings.TrimSpace(severity), "critical") {
			query = query.Where("severity IN ?", []string{"P0", "P1"})
		} else {
			query = query.Where("severity = ?", normalizeSeverity(severity))
		}
	}
	if strings.TrimSpace(env) != "" {
		query = query.Where("env = ?", normalizeEnvCode(env))
	}
	if strings.TrimSpace(alertType) != "" {
		query = query.Where("alert_type = ?", normalizeAlertType(alertType))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorAlertRule
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetMonitorAlertRule(id uint) (*model.MonitorAlertRule, error) {
	var item model.MonitorAlertRule
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) SaveMonitorAlertRule(payload MonitorAlertRulePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("规则名称不能为空")
	}
	alertType := normalizeAlertType(payload.AlertType)
	datasourceScope := normalizeDatasourceScope(payload.DatasourceScope)
	queryText := firstNonEmpty(strings.TrimSpace(payload.Query), strings.TrimSpace(payload.PromQL))
	if alertType == "datasource_health" {
		datasourceScope = "specific"
		queryText = "datasource_health"
	} else if queryText == "" {
		if isMonitorLogAlertType(alertType) {
			return errors.New("Elasticsearch 查询语句不能为空")
		}
		return errors.New("PromQL 不能为空")
	}
	labelsJSON, err := ensureJSONObject(payload.LabelsJSON)
	if err != nil {
		return err
	}
	annotationsJSON, err := ensureJSONObject(payload.AnnotationsJSON)
	if err != nil {
		return err
	}
	datasourceName := ""
	datasourceID := payload.DatasourceID
	if datasourceScope == "specific" {
		if datasourceID == 0 {
			return errors.New("请选择数据源")
		}
		ds, err := s.GetMonitorDatasource(datasourceID)
		if err != nil {
			return err
		}
		if alertType == "log" && normalizeMonitorDatasourceType(ds.Type) != "elasticsearch" {
			return errors.New("日志告警只能选择 Elasticsearch 数据源")
		}
		if alertType == "victorialogs" && normalizeMonitorDatasourceType(ds.Type) != "victorialogs" {
			return errors.New("日志告警只能选择 VictoriaLogs 数据源")
		}
		if alertType == "metric" && !isMonitorMetricDatasource(ds.Type) {
			return errors.New("监控告警只能选择 Prometheus 或 VictoriaMetrics 数据源")
		}
		datasourceName = ds.Name
	} else {
		var count int64
		if alertType == "log" {
			err = s.db.Model(&model.MonitorDatasource{}).Where("status = ? AND type = ?", 1, "elasticsearch").Count(&count).Error
			datasourceName = "全部日志数据源"
		} else if alertType == "victorialogs" {
			err = s.db.Model(&model.MonitorDatasource{}).Where("status = ? AND type = ?", 1, "victorialogs").Count(&count).Error
			datasourceName = "全部 VictoriaLogs 数据源"
		} else {
			err = s.db.Model(&model.MonitorDatasource{}).Where("status = ? AND type IN ?", 1, []string{"prometheus", "victoriametrics"}).Count(&count).Error
			datasourceName = "全部监控数据源"
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("没有可用的匹配数据源")
		}
		datasourceID = 0
	}
	updates := map[string]any{
		"name":                           Trimmed(payload.Name),
		"alert_type":                     alertType,
		"datasource_scope":               datasourceScope,
		"datasource_id":                  datasourceID,
		"datasource_name":                datasourceName,
		"prom_ql":                        queryText,
		"log_index":                      firstNonEmpty(strings.TrimSpace(payload.LogIndex), "_all"),
		"log_time_range_seconds":         normalizeLogTimeRangeSeconds(payload.LogTimeRangeSeconds),
		"comparator":                     normalizeComparator(payload.Comparator),
		"threshold":                      payload.Threshold,
		"for_seconds":                    normalizeForSeconds(payload.ForSeconds),
		"eval_interval_seconds":          normalizeEvalInterval(payload.EvalIntervalSeconds),
		"notify_repeat_interval_seconds": normalizeNotifyRepeatInterval(payload.NotifyRepeatIntervalSeconds),
		"max_notify_count":               normalizeMaxNotifyCount(payload.MaxNotifyCount),
		"severity":                       normalizeSeverity(payload.Severity),
		"labels_json":                    labelsJSON,
		"annotations_json":               annotationsJSON,
		"notify_enabled":                 payload.NotifyEnabled,
		"notify_rule_id":                 payload.NotifyRuleID,
		"notify_recovery_enabled":        payload.NotifyRecoveryEnabled,
		"env":                            normalizeEnvCode(payload.Env),
		"status":                         normalizeMonitorStatus(payload.Status),
		"description":                    Trimmed(payload.Description),
	}
	if alertType == "datasource_health" {
		updates["threshold"] = 2.0
		updates["for_seconds"] = 0
		updates["eval_interval_seconds"] = 15
	}
	var current model.MonitorAlertRule
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if payload.ID > 0 {
			if err := tx.Model(&model.MonitorAlertRule{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
				return err
			}
			return tx.First(&current, payload.ID).Error
		}
		if err := tx.Model(&model.MonitorAlertRule{}).Create(updates).Error; err != nil {
			return err
		}
		return tx.Last(&current).Error
	})
	if err != nil {
		return err
	}
	if current.Status == 1 {
		return s.registerMonitorAlertRule(current)
	}
	s.removeMonitorAlertRule(current.ID)
	s.closeActiveMonitorAlertEventsForRule(current.ID, "告警规则已停用，系统自动关闭未结束事件")
	return nil
}

func (s *Service) DeleteMonitorAlertRule(id uint) error {
	s.removeMonitorAlertRule(id)
	return s.db.Delete(&model.MonitorAlertRule{}, id).Error
}

func (s *Service) ListMonitorSilenceRules(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorSilenceRule{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR rule_name_pattern LIKE ? OR description LIKE ?", like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorSilenceRule
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, map[string]any{
			"id": item.ID, "name": item.Name, "matchMode": firstNonEmpty(item.MatchMode, "regex"),
			"ruleIds": decodeUintList(item.RuleIDsJSON), "ruleIdsJson": item.RuleIDsJSON,
			"ruleNamePattern": item.RuleNamePattern, "severity": item.Severity, "matchersJson": item.MatchersJSON,
			"startsAt": item.StartsAt, "endsAt": item.EndsAt, "priority": item.Priority, "status": item.Status, "description": item.Description,
			"createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
		})
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetMonitorSilenceRule(id uint) (map[string]any, error) {
	var item model.MonitorSilenceRule
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"id": item.ID, "name": item.Name, "matchMode": firstNonEmpty(item.MatchMode, "regex"),
		"ruleIds": decodeUintList(item.RuleIDsJSON), "ruleIdsJson": item.RuleIDsJSON,
		"ruleNamePattern": item.RuleNamePattern, "severity": item.Severity, "alertType": item.AlertType, "matchersJson": item.MatchersJSON,
		"startsAt": item.StartsAt, "endsAt": item.EndsAt, "priority": item.Priority, "status": item.Status, "description": item.Description,
		"createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
	}, nil
}

func (s *Service) SaveMonitorSilenceRule(payload MonitorSilenceRulePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("silence rule name is required")
	}
	if payload.StartsAt > 0 && payload.EndsAt > 0 && payload.EndsAt <= payload.StartsAt {
		return errors.New("结束时间必须晚于开始时间")
	}
	if normalizeRuleMatchMode(payload.MatchMode) == "regex" && strings.TrimSpace(payload.RuleNamePattern) == "" {
		return errors.New("规则名正则不能为空")
	}
	matchersJSON, err := normalizeMatcherJSON(payload.MatchersJSON)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"name":              Trimmed(payload.Name),
		"match_mode":        normalizeRuleMatchMode(payload.MatchMode),
		"rule_ids_json":     encodeUintList(payload.RuleIDs),
		"rule_name_pattern": strings.TrimSpace(payload.RuleNamePattern),
		"severity":          strings.TrimSpace(payload.Severity),
		"alert_type":        strings.TrimSpace(payload.AlertType),
		"matchers_json":     matchersJSON,
		"starts_at":         unixPtr(payload.StartsAt),
		"ends_at":           unixPtr(payload.EndsAt),
		"priority":          normalizeSilencePriority(payload.Priority),
		"status":            normalizeMonitorStatus(payload.Status),
		"description":       Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.MonitorSilenceRule{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.MonitorSilenceRule{}).Create(updates).Error
}

func (s *Service) PreviewMonitorSilenceRule(payload MonitorSilenceRulePayload) (map[string]any, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.New("屏蔽规则名称不能为空")
	}
	matchersJSON, err := normalizeMatcherJSON(payload.MatchersJSON)
	if err != nil {
		return nil, err
	}
	if normalizeRuleMatchMode(payload.MatchMode) == "regex" && strings.TrimSpace(payload.RuleNamePattern) == "" {
		return nil, errors.New("规则名正则不能为空")
	}
	preview := model.MonitorSilenceRule{
		MatchMode: normalizeRuleMatchMode(payload.MatchMode), RuleIDsJSON: encodeUintList(payload.RuleIDs),
		RuleNamePattern: strings.TrimSpace(payload.RuleNamePattern), Severity: strings.TrimSpace(payload.Severity), AlertType: strings.TrimSpace(payload.AlertType), MatchersJSON: matchersJSON,
	}
	var rules []model.MonitorAlertRule
	if err := s.db.Order("id DESC").Find(&rules).Error; err != nil {
		return nil, err
	}
	matchedRules := make([]map[string]any, 0)
	matchedRuleIDs := map[uint]bool{}
	for _, rule := range rules {
		if !monitorSilenceRuleCriteriaMatch(preview, rule, nil) {
			continue
		}
		matchedRuleIDs[rule.ID] = true
		if len(matchedRules) < 20 {
			matchedRules = append(matchedRules, map[string]any{"id": rule.ID, "name": rule.Name, "severity": rule.Severity, "alertType": rule.AlertType})
		}
	}
	var events []model.MonitorAlertEvent
	if err := s.db.Where("status IN ?", []string{"pending", "firing", "claimed", "silenced"}).Order("last_trigger_at DESC").Find(&events).Error; err != nil {
		return nil, err
	}
	matchedEvents := make([]map[string]any, 0)
	for _, event := range events {
		if !matchedRuleIDs[event.RuleID] {
			continue
		}
		var rule model.MonitorAlertRule
		if err := s.db.First(&rule, event.RuleID).Error; err != nil {
			continue
		}
		labels := decodeLabelMap(event.LabelsJSON)
		if !monitorSilenceRuleCriteriaMatch(preview, rule, labels) {
			continue
		}
		if len(matchedEvents) < 20 {
			matchedEvents = append(matchedEvents, map[string]any{"id": event.ID, "ruleName": event.RuleName, "severity": event.Severity, "status": event.Status, "summary": event.Summary})
		}
	}
	return map[string]any{
		"matchedRuleCount": len(matchedRuleIDs), "matchedRules": matchedRules,
		"matchedActiveEventCount": len(matchedEvents), "matchedActiveEvents": matchedEvents,
	}, nil
}

func (s *Service) DeleteMonitorSilenceRule(id uint) error {
	return s.db.Delete(&model.MonitorSilenceRule{}, id).Error
}

func normalizeMonitorBatchIDs(ids []uint) ([]uint, error) {
	seen := map[uint]bool{}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("请选择至少一条规则")
	}
	return result, nil
}

func (s *Service) BatchUpdateMonitorSilenceRules(payload MonitorRuleBatchPayload) error {
	ids, err := normalizeMonitorBatchIDs(payload.IDs)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "enable":
		return s.db.Model(&model.MonitorSilenceRule{}).Where("id IN ?", ids).Update("status", 1).Error
	case "disable":
		return s.db.Model(&model.MonitorSilenceRule{}).Where("id IN ?", ids).Update("status", 2).Error
	case "delete":
		return s.db.Where("id IN ?", ids).Delete(&model.MonitorSilenceRule{}).Error
	default:
		return errors.New("不支持的批量操作")
	}
}

func (s *Service) ListMonitorAggregationRules(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorAggregationRule{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR rule_name_pattern LIKE ? OR description LIKE ?", like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorAggregationRule
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, map[string]any{
			"id": item.ID, "name": item.Name, "matchMode": firstNonEmpty(item.MatchMode, "regex"),
			"ruleIds": decodeUintList(item.RuleIDsJSON), "ruleIdsJson": item.RuleIDsJSON,
			"ruleNamePattern": item.RuleNamePattern, "severity": item.Severity,
			"groupBy": decodeStringList(item.GroupByJSON), "groupByJson": item.GroupByJSON,
			"windowSeconds": item.WindowSeconds, "repeatIntervalSeconds": item.RepeatIntervalSeconds,
			"status": item.Status, "description": item.Description, "createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
		})
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetMonitorAggregationRule(id uint) (map[string]any, error) {
	var item model.MonitorAggregationRule
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"id": item.ID, "name": item.Name, "matchMode": firstNonEmpty(item.MatchMode, "regex"),
		"ruleIds": decodeUintList(item.RuleIDsJSON), "ruleIdsJson": item.RuleIDsJSON,
		"ruleNamePattern": item.RuleNamePattern, "severity": item.Severity, "alertType": item.AlertType,
		"groupBy": decodeStringList(item.GroupByJSON), "groupByJson": item.GroupByJSON,
		"windowSeconds": item.WindowSeconds, "repeatIntervalSeconds": item.RepeatIntervalSeconds,
		"status": item.Status, "description": item.Description, "createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
	}, nil
}

func (s *Service) SaveMonitorAggregationRule(payload MonitorAggregationRulePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("aggregation rule name is required")
	}
	// An empty list is intentional: it means aggregate all samples of the
	// matched rule and severity into one bucket, regardless of their labels.
	groupBy := payload.GroupBy
	updates := map[string]any{
		"name":                    Trimmed(payload.Name),
		"match_mode":              normalizeRuleMatchMode(payload.MatchMode),
		"rule_ids_json":           encodeUintList(payload.RuleIDs),
		"rule_name_pattern":       strings.TrimSpace(payload.RuleNamePattern),
		"severity":                strings.TrimSpace(payload.Severity),
		"alert_type":              strings.TrimSpace(payload.AlertType),
		"group_by_json":           encodeStringList(groupBy),
		"window_seconds":          normalizeAggregationWindow(payload.WindowSeconds),
		"repeat_interval_seconds": normalizeAggregationWindow(payload.RepeatIntervalSeconds),
		"status":                  normalizeMonitorStatus(payload.Status),
		"description":             Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.MonitorAggregationRule{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.MonitorAggregationRule{}).Create(updates).Error
}

func (s *Service) DeleteMonitorAggregationRule(id uint) error {
	return s.db.Delete(&model.MonitorAggregationRule{}, id).Error
}

func (s *Service) BatchUpdateMonitorAggregationRules(payload MonitorRuleBatchPayload) error {
	ids, err := normalizeMonitorBatchIDs(payload.IDs)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "enable":
		return s.db.Model(&model.MonitorAggregationRule{}).Where("id IN ?", ids).Update("status", 1).Error
	case "disable":
		return s.db.Model(&model.MonitorAggregationRule{}).Where("id IN ?", ids).Update("status", 2).Error
	case "delete":
		return s.db.Where("id IN ?", ids).Delete(&model.MonitorAggregationRule{}).Error
	default:
		return errors.New("不支持的批量操作")
	}
}

func (s *Service) UpdateMonitorAlertRuleStatus(id uint, status int) error {
	status = normalizeMonitorStatus(status)
	if err := s.db.Model(&model.MonitorAlertRule{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		return err
	}
	rule, err := s.GetMonitorAlertRule(id)
	if err != nil {
		return err
	}
	if status == 1 {
		return s.registerMonitorAlertRule(*rule)
	}
	s.removeMonitorAlertRule(id)
	s.closeActiveMonitorAlertEventsForRule(id, "告警规则已停用，系统自动关闭未结束事件")
	return nil
}

// Disabling a rule is terminal for its in-flight events. Keeping them firing
// would let a later aggregation flush notify an alert whose rule is inactive.
func (s *Service) closeActiveMonitorAlertEventsForRule(ruleID uint, reason string) {
	if ruleID == 0 || s.db == nil {
		return
	}
	var events []model.MonitorAlertEvent
	if err := s.db.Where("rule_id = ? AND status IN ?", ruleID, []string{"pending", "firing", "claimed", "silenced"}).Find(&events).Error; err != nil || len(events) == 0 {
		return
	}
	now := time.Now()
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("id IN ?", monitorAlertEventIDs(events)).Updates(map[string]any{
		"status": "resolved", "resolved_at": &now, "resolve_note": reason,
	}).Error; err != nil {
		return
	}
	for _, event := range events {
		s.appendMonitorAlertTimeline(event.ID, "resolved", "告警规则已停用，事件自动关闭", reason, "系统", nil)
	}
}

func (s *Service) closeInactiveMonitorAlertRuleEvents() {
	if s.db == nil {
		return
	}
	var rules []model.MonitorAlertRule
	if err := s.db.Select("id").Where("status <> ?", 1).Find(&rules).Error; err != nil {
		return
	}
	for _, rule := range rules {
		s.closeActiveMonitorAlertEventsForRule(rule.ID, "告警规则已停用，系统自动关闭未结束事件")
	}
}

func monitorAlertEventIDs(events []model.MonitorAlertEvent) []uint {
	ids := make([]uint, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

func (s *Service) BatchUpdateMonitorAlertRules(payload MonitorAlertRuleBatchPayload) error {
	if len(payload.IDs) == 0 {
		return errors.New("请选择至少一条告警规则")
	}
	uniqueIDs := make([]uint, 0, len(payload.IDs))
	seen := map[uint]bool{}
	for _, id := range payload.IDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	if len(uniqueIDs) == 0 {
		return errors.New("告警规则不能为空")
	}
	updates := map[string]any{}
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	switch action {
	case "enable":
		updates["status"] = 1
	case "disable":
		updates["status"] = 2
	case "update_for_seconds":
		if payload.ForSeconds == nil {
			return errors.New("请填写持续时间")
		}
		if *payload.ForSeconds < 0 || *payload.ForSeconds > 86400 {
			return errors.New("持续时间需在 0 到 86400 秒之间")
		}
		var metricRuleCount int64
		if err := s.db.Model(&model.MonitorAlertRule{}).Where("id IN ? AND alert_type = ?", uniqueIDs, "metric").Count(&metricRuleCount).Error; err != nil {
			return err
		}
		if metricRuleCount == 0 {
			return errors.New("所选规则中没有可修改持续时间的监控告警规则")
		}
		return s.db.Model(&model.MonitorAlertRule{}).Where("id IN ? AND alert_type = ?", uniqueIDs, "metric").Update("for_seconds", *payload.ForSeconds).Error
	case "update_eval_interval":
		if payload.EvalIntervalSeconds == nil {
			return errors.New("请填写评估间隔")
		}
		if *payload.EvalIntervalSeconds < 15 || *payload.EvalIntervalSeconds > 3600 {
			return errors.New("评估间隔需在 15 到 3600 秒之间")
		}
		updates["eval_interval_seconds"] = *payload.EvalIntervalSeconds
	case "enable_notify":
		if payload.NotifyRuleID == 0 {
			return errors.New("请选择通知规则")
		}
		if payload.NotifyRepeatIntervalSeconds == nil {
			return errors.New("请填写重复通知间隔")
		}
		if *payload.NotifyRepeatIntervalSeconds < 60 || *payload.NotifyRepeatIntervalSeconds > 604800 {
			return errors.New("重复通知间隔需在 1 分钟到 7 天之间")
		}
		if payload.MaxNotifyCount == nil {
			return errors.New("请填写最大发送次数")
		}
		if *payload.MaxNotifyCount < 0 || *payload.MaxNotifyCount > 1000 {
			return errors.New("最大发送次数需在 0 到 1000 之间")
		}
		if payload.NotifyRecoveryEnabled == nil {
			return errors.New("请设置是否发送恢复通知")
		}
		var notifyRule model.NotifyRule
		if err := s.db.Where("id = ? AND status = ?", payload.NotifyRuleID, 1).First(&notifyRule).Error; err != nil {
			return errors.New("通知规则不存在或已禁用")
		}
		updates["notify_enabled"] = true
		updates["notify_rule_id"] = payload.NotifyRuleID
		updates["notify_repeat_interval_seconds"] = *payload.NotifyRepeatIntervalSeconds
		updates["max_notify_count"] = *payload.MaxNotifyCount
		updates["notify_recovery_enabled"] = *payload.NotifyRecoveryEnabled
	default:
		return errors.New("不支持的批量操作")
	}
	if err := s.db.Model(&model.MonitorAlertRule{}).Where("id IN ?", uniqueIDs).Updates(updates).Error; err != nil {
		return err
	}
	if action == "enable_notify" {
		return nil
	}
	var rules []model.MonitorAlertRule
	if err := s.db.Where("id IN ?", uniqueIDs).Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.Status == 1 {
			if err := s.registerMonitorAlertRule(rule); err != nil {
				return err
			}
		} else if action == "enable" || action == "disable" {
			s.removeMonitorAlertRule(rule.ID)
			s.closeActiveMonitorAlertEventsForRule(rule.ID, "告警规则已批量停用，系统自动关闭未结束事件")
		}
	}
	return nil
}

func (s *Service) RunMonitorAlertRule(id uint) error {
	go s.evaluateMonitorAlertRule(id)
	return nil
}

func (s *Service) PreviewMonitorAlertRule(payload MonitorAlertRulePayload) (map[string]any, error) {
	queryText := firstNonEmpty(strings.TrimSpace(payload.Query), strings.TrimSpace(payload.PromQL))
	if queryText == "" {
		return nil, errors.New("查询语句不能为空")
	}
	rule := model.MonitorAlertRule{
		ID: payload.ID, Name: firstNonEmpty(strings.TrimSpace(payload.Name), "规则预览"),
		AlertType: normalizeAlertType(payload.AlertType), DatasourceScope: normalizeDatasourceScope(payload.DatasourceScope),
		DatasourceID: payload.DatasourceID, PromQL: queryText, LogIndex: firstNonEmpty(strings.TrimSpace(payload.LogIndex), "_all"),
		LogTimeRangeSeconds: normalizeLogTimeRangeSeconds(payload.LogTimeRangeSeconds), Comparator: normalizeComparator(payload.Comparator),
		Threshold: payload.Threshold, Severity: normalizeSeverity(payload.Severity),
	}
	datasources, err := s.monitorRuleDatasources(rule)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(datasources))
	totalSeries := 0
	totalMatched := 0
	failedDatasources := 0
	for _, ds := range datasources {
		item := map[string]any{"datasourceId": ds.ID, "datasourceName": ds.Name, "status": "success", "samples": []map[string]any{}}
		samples := make([]map[string]any, 0)
		if isMonitorLogAlertType(rule.AlertType) {
			value, sample, queryErr := s.monitorLogAlertValue(rule, ds)
			if queryErr != nil {
				item["status"] = "failed"
				item["error"] = queryErr.Error()
				failedDatasources++
			} else {
				matched := compareFloat(value, rule.Comparator, rule.Threshold)
				totalSeries++
				if matched {
					totalMatched++
				}
				samples = append(samples, map[string]any{"labels": sample.Metric, "value": value, "matched": matched})
			}
		} else {
			result, queryErr := s.prometheusQuery(ds, rule.PromQL, time.Now())
			if queryErr != nil {
				item["status"] = "failed"
				item["error"] = queryErr.Error()
				failedDatasources++
			} else {
				item["resultType"] = result.Data.ResultType
				totalSeries += len(result.Data.Result)
				for _, sample := range result.Data.Result {
					value, ok := promSampleValue(sample)
					if !ok {
						continue
					}
					matched := compareFloat(value, rule.Comparator, rule.Threshold)
					if matched {
						totalMatched++
					}
					if len(samples) < 50 {
						samples = append(samples, map[string]any{"labels": sample.Metric, "value": value, "matched": matched})
					}
				}
			}
		}
		item["samples"] = samples
		item["seriesCount"] = len(samples)
		results = append(results, item)
	}
	explanation := fmt.Sprintf("共查询 %d 个数据源，返回 %d 条序列，其中 %d 条满足 %s %.4f", len(datasources), totalSeries, totalMatched, rule.Comparator, rule.Threshold)
	if failedDatasources > 0 {
		explanation += fmt.Sprintf("，%d 个数据源查询失败", failedDatasources)
	}
	return map[string]any{
		"datasourceCount": len(datasources), "failedDatasourceCount": failedDatasources,
		"totalSeries": totalSeries, "totalMatched": totalMatched, "explanation": explanation, "results": results,
	}, nil
}

func (s *Service) evaluateMonitorAlertRule(id uint) {
	if !s.beginMonitorAlertRuleEvaluation(id) {
		return
	}
	defer s.endMonitorAlertRuleEvaluation(id)

	rule, err := s.GetMonitorAlertRule(id)
	if err != nil || rule.Status != 1 {
		return
	}
	datasources, err := s.monitorRuleDatasources(*rule)
	if err != nil {
		s.updateMonitorRuleEval(*rule, "failed", err.Error())
		return
	}
	activeFingerprints := map[string]bool{}
	matched := 0
	failed := 0
	for _, ds := range datasources {
		scopedRule := *rule
		scopedRule.DatasourceID = ds.ID
		scopedRule.DatasourceName = ds.Name
		if isMonitorLogAlertType(rule.AlertType) {
			value, sample, err := s.monitorLogAlertValue(scopedRule, ds)
			if err != nil {
				failed++
				continue
			}
			if !compareFloat(value, scopedRule.Comparator, scopedRule.Threshold) {
				continue
			}
			fp := monitorFingerprint(scopedRule.ID, sample.Metric)
			activeFingerprints[fp] = true
			matched++
			s.upsertMonitorAlertEvent(scopedRule, sample, fp, value)
			continue
		}
		result, err := s.prometheusQuery(ds, scopedRule.PromQL, time.Now())
		if err != nil {
			failed++
			continue
		}
		for _, sample := range result.Data.Result {
			value, ok := promSampleValue(sample)
			if !ok || !compareFloat(value, scopedRule.Comparator, scopedRule.Threshold) {
				continue
			}
			if sample.Metric == nil {
				sample.Metric = map[string]string{}
			}
			sample.Metric["datasource"] = ds.Name
			fp := monitorFingerprint(scopedRule.ID, sample.Metric)
			activeFingerprints[fp] = true
			matched++
			s.upsertMonitorAlertEvent(scopedRule, sample, fp, value)
		}
	}
	s.recoverInactiveMonitorEvents(*rule, activeFingerprints)
	if failed == len(datasources) {
		s.updateMonitorRuleEval(*rule, "failed", "全部匹配数据源评估失败")
		return
	}
	s.updateMonitorRuleEval(*rule, "success", fmt.Sprintf("命中 %d 条序列，%d 个数据源失败", matched, failed))
}

func (s *Service) beginMonitorAlertRuleEvaluation(id uint) bool {
	if s.monitorScheduler == nil {
		return true
	}
	s.monitorScheduler.mu.Lock()
	defer s.monitorScheduler.mu.Unlock()
	if s.monitorScheduler.running[id] {
		return false
	}
	s.monitorScheduler.running[id] = true
	return true
}

func (s *Service) endMonitorAlertRuleEvaluation(id uint) {
	if s.monitorScheduler == nil {
		return
	}
	s.monitorScheduler.mu.Lock()
	delete(s.monitorScheduler.running, id)
	s.monitorScheduler.mu.Unlock()
}

func (s *Service) monitorRuleDatasources(rule model.MonitorAlertRule) ([]model.MonitorDatasource, error) {
	if normalizeDatasourceScope(rule.DatasourceScope) == "specific" {
		ds, err := s.GetMonitorDatasource(rule.DatasourceID)
		if err != nil {
			return nil, err
		}
		return []model.MonitorDatasource{*ds}, nil
	}
	query := s.db.Where("status = ?", 1)
	if normalizeAlertType(rule.AlertType) == "log" {
		query = query.Where("type = ?", "elasticsearch")
	} else if normalizeAlertType(rule.AlertType) == "victorialogs" {
		query = query.Where("type = ?", "victorialogs")
	} else {
		query = query.Where("type IN ?", []string{"prometheus", "victoriametrics"})
	}
	var list []model.MonitorDatasource
	if err := query.Order("is_default DESC, id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New("没有可用的匹配数据源")
	}
	return list, nil
}

func (s *Service) monitorLogAlertValue(rule model.MonitorAlertRule, ds model.MonitorDatasource) (float64, PromMetricSample, error) {
	if normalizeMonitorDatasourceType(ds.Type) == "victorialogs" {
		return s.victoriaLogsAlertValue(rule, ds)
	}
	end := time.Now()
	start := end.Add(-time.Duration(normalizeLogTimeRangeSeconds(rule.LogTimeRangeSeconds)) * time.Second)
	must := []any{map[string]any{"match_all": map[string]any{}}}
	if strings.TrimSpace(rule.PromQL) != "" {
		must = []any{map[string]any{"query_string": map[string]any{"query": strings.TrimSpace(rule.PromQL), "analyze_wildcard": true}}}
	}
	body := map[string]any{
		"size":             0,
		"track_total_hits": true,
		"query": map[string]any{"bool": map[string]any{
			"must": must,
			"filter": []any{map[string]any{"range": map[string]any{"@timestamp": map[string]any{
				"gte": start.UnixMilli(), "lte": end.UnixMilli(), "format": "epoch_millis",
			}}}},
		}},
	}
	result, err := s.elasticsearchSearch(ds, firstNonEmpty(strings.TrimSpace(rule.LogIndex), "_all"), body)
	if err != nil {
		return 0, PromMetricSample{}, err
	}
	hits, _ := result["hits"].(map[string]any)
	value := elasticsearchHitTotal(hits["total"])
	return value, PromMetricSample{Metric: map[string]string{
		"__name__": "elasticsearch_log_count", "datasource": ds.Name, "index": firstNonEmpty(strings.TrimSpace(rule.LogIndex), "_all"), "alert_type": "log",
	}}, nil
}

func (s *Service) victoriaLogsAlertValue(rule model.MonitorAlertRule, ds model.MonitorDatasource) (float64, PromMetricSample, error) {
	end := time.Now()
	start := end.Add(-time.Duration(normalizeLogTimeRangeSeconds(rule.LogTimeRangeSeconds)) * time.Second)
	query := strings.TrimSpace(rule.PromQL)
	if query == "" {
		query = "*"
	}
	form := url.Values{}
	form.Set("query", query)
	form.Set("start", start.UTC().Format(time.RFC3339Nano))
	form.Set("end", end.UTC().Format(time.RFC3339Nano))
	form.Set("step", "1m")
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/hits", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, PromMetricSample{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return 0, PromMetricSample{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, PromMetricSample{}, fmt.Errorf("VictoriaLogs 日志告警查询失败，状态码 %d: %s", response.StatusCode, string(body))
	}
	var result struct {
		Hits []struct {
			Total int64 `json:"total"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, PromMetricSample{}, fmt.Errorf("VictoriaLogs 日志告警结果解析失败: %w", err)
	}
	var value int64
	for _, group := range result.Hits {
		value += group.Total
	}
	return float64(value), PromMetricSample{Metric: map[string]string{
		"__name__": "victorialogs_log_count", "datasource": ds.Name, "alert_type": "victorialogs",
	}}, nil
}

func elasticsearchHitTotal(value any) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case int:
		return float64(item)
	case map[string]any:
		return elasticsearchHitTotal(item["value"])
	default:
		parsed, _ := strconv.ParseFloat(fmt.Sprint(value), 64)
		return parsed
	}
}

func (s *Service) updateMonitorRuleEval(rule model.MonitorAlertRule, status, message string) {
	now := time.Now()
	_ = s.db.Model(&model.MonitorAlertRule{}).Where("id = ?", rule.ID).Updates(map[string]any{
		"last_eval_at": &now, "last_eval_status": status, "last_eval_message": message,
	}).Error
}

func promSampleValue(sample PromMetricSample) (float64, bool) {
	if len(sample.Value) < 2 {
		return 0, false
	}
	raw := fmt.Sprintf("%v", sample.Value[1])
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil
}

func compareFloat(value float64, comparator string, threshold float64) bool {
	switch normalizeComparator(comparator) {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return value > threshold
	}
}

func monitorFingerprint(ruleID uint, metric map[string]string) string {
	keys := make([]string, 0, len(metric))
	for key := range metric {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{fmt.Sprintf("rule=%d", ruleID)}
	for _, key := range keys {
		parts = append(parts, key+"="+metric[key])
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func monitorSilenceRuleCriteriaMatch(item model.MonitorSilenceRule, rule model.MonitorAlertRule, labels map[string]string) bool {
	if !monitorRuleMatch(item.MatchMode, item.RuleIDsJSON, item.RuleNamePattern, rule) || !monitorSeverityMatch(item.Severity, rule.Severity) || !monitorAlertTypeMatch(item.AlertType, rule.AlertType) {
		return false
	}
	return labels == nil || monitorMatchersMatch(item.MatchersJSON, labels)
}

func monitorAlertTypeMatch(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	return expected == "" || expected == strings.TrimSpace(actual)
}

func (s *Service) matchMonitorSilenceRule(rule model.MonitorAlertRule, labels map[string]string) (*model.MonitorSilenceRule, bool) {
	now := time.Now()
	var rules []model.MonitorSilenceRule
	if err := s.db.Where("status = ?", 1).Order("priority DESC, id DESC").Find(&rules).Error; err != nil {
		return nil, false
	}
	for _, item := range rules {
		if item.StartsAt != nil && now.Before(*item.StartsAt) {
			continue
		}
		if item.EndsAt != nil && now.After(*item.EndsAt) {
			continue
		}
		if !monitorSilenceRuleCriteriaMatch(item, rule, labels) {
			continue
		}
		return &item, true
	}
	return nil, false
}

func (s *Service) matchMonitorAggregationRule(rule model.MonitorAlertRule, labels map[string]string) (*model.MonitorAggregationRule, string, bool) {
	var rules []model.MonitorAggregationRule
	if err := s.db.Where("status = ?", 1).Order("id DESC").Find(&rules).Error; err != nil {
		return nil, "", false
	}
	for _, item := range rules {
		if !monitorRuleMatch(item.MatchMode, item.RuleIDsJSON, item.RuleNamePattern, rule) || !monitorSeverityMatch(item.Severity, rule.Severity) || !monitorAlertTypeMatch(item.AlertType, rule.AlertType) {
			continue
		}
		groupBy := decodeStringList(item.GroupByJSON)
		parts := []string{fmt.Sprintf("aggregation=%d", item.ID), "rule=" + rule.Name, "severity=" + rule.Severity}
		for _, key := range groupBy {
			key = strings.TrimSpace(key)
			if key != "" {
				parts = append(parts, key+"="+labels[key])
			}
		}
		return &item, strings.Join(parts, "|"), true
	}
	return nil, "", false
}

func (s *Service) shouldNotifyAggregatedEvent(event model.MonitorAlertEvent, aggregation *model.MonitorAggregationRule) bool {
	if aggregation == nil || event.AggregationKey == "" {
		return true
	}
	lookbackSeconds := aggregationLookbackSeconds(*aggregation)
	windowStart := time.Now().Add(-time.Duration(lookbackSeconds) * time.Second)
	var last model.MonitorAlertEvent
	err := s.db.Where("aggregation_key = ? AND id <> ? AND last_notify_at >= ?", event.AggregationKey, event.ID, windowStart).
		Order("last_notify_at DESC").First(&last).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if err != nil || last.LastNotifyAt == nil {
		return true
	}
	repeatAfter := time.Duration(lookbackSeconds) * time.Second
	return time.Since(*last.LastNotifyAt) >= repeatAfter
}

func aggregationLookbackSeconds(aggregation model.MonitorAggregationRule) int {
	windowSeconds := normalizeAggregationWindow(aggregation.WindowSeconds)
	repeatSeconds := normalizeAggregationWindow(aggregation.RepeatIntervalSeconds)
	if repeatSeconds > windowSeconds {
		return repeatSeconds
	}
	return windowSeconds
}

// shouldNotifyAlertEvent applies the per-rule reminder interval before any
// cross-event aggregation suppression. Events are still persisted every time.
func (s *Service) shouldNotifyAlertEvent(event model.MonitorAlertEvent, rule model.MonitorAlertRule, aggregation *model.MonitorAggregationRule) bool {
	if rule.MaxNotifyCount > 0 && event.NotifyCount >= rule.MaxNotifyCount {
		return false
	}
	if event.LastNotifyAt != nil && time.Since(*event.LastNotifyAt) < time.Duration(normalizeNotifyRepeatInterval(rule.NotifyRepeatIntervalSeconds))*time.Second {
		return false
	}
	return s.shouldNotifyAggregatedEvent(event, aggregation)
}

func (s *Service) markMonitorAlertNotified(event *model.MonitorAlertEvent) bool {
	now := time.Now()
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
		"last_notify_at": &now,
		"notify_count":   gorm.Expr("notify_count + ?", 1),
	}).Error; err == nil {
		event.LastNotifyAt = &now
		event.NotifyCount++
		return true
	}
	return false
}

func (s *Service) notifyMonitorAlertIfAllowed(event *model.MonitorAlertEvent, rule model.MonitorAlertRule, aggregation *model.MonitorAggregationRule, status string) bool {
	// A matching aggregation rule changes the delivery model from "send this
	// event" to "collect this group and send one summary after its window".
	// The scheduled flusher owns the notification marker for the whole group.
	if aggregation != nil && event.AggregationKey != "" && status == "firing" {
		return false
	}
	// Keep the aggregation decision and its persisted notification marker in one
	// critical section so concurrent rule evaluations cannot both notify.
	s.monitorNotifyMu.Lock()
	if !s.shouldNotifyAlertEvent(*event, rule, aggregation) || !s.markMonitorAlertNotified(event) {
		s.monitorNotifyMu.Unlock()
		return false
	}
	s.monitorNotifyMu.Unlock()
	s.dispatchMonitorNotification(rule, *event, status)
	s.appendMonitorAlertTimeline(event.ID, "notification", "已提交消息通知", fmt.Sprintf("状态：%s，第 %d 次发送", status, event.NotifyCount), "系统", map[string]any{
		"notifyRuleId": rule.NotifyRuleID, "notifyCount": event.NotifyCount, "status": status,
	})
	return true
}

// flushDueMonitorAggregationNotifications delivers one summary for every due
// aggregation bucket. Individual events remain visible in the event list, but
// are never sent one-by-one while they belong to an aggregation rule.
func (s *Service) flushDueMonitorAggregationNotifications() {
	if s.db == nil {
		return
	}
	now := time.Now()
	var aggregations []model.MonitorAggregationRule
	if err := s.db.Where("status = ?", 1).Find(&aggregations).Error; err != nil {
		return
	}
	for _, aggregation := range aggregations {
		windowStart := now.Add(-time.Duration(normalizeAggregationWindow(aggregation.WindowSeconds)) * time.Second)
		var candidates []model.MonitorAlertEvent
		if err := s.db.Where("aggregate_rule_id = ? AND aggregation_key <> '' AND status = ? AND silenced = ? AND first_trigger_at <= ?", aggregation.ID, "firing", false, windowStart).
			Order("aggregation_key ASC, first_trigger_at ASC").Find(&candidates).Error; err != nil {
			continue
		}
		groups := make(map[string][]model.MonitorAlertEvent)
		for _, event := range candidates {
			groups[event.AggregationKey] = append(groups[event.AggregationKey], event)
		}
		for key, events := range groups {
			s.flushMonitorAggregationGroup(aggregation, key, events, now)
		}
	}
}

func (s *Service) flushMonitorAggregationGroup(aggregation model.MonitorAggregationRule, key string, events []model.MonitorAlertEvent, now time.Time) {
	if len(events) == 0 {
		return
	}
	representative := events[0]
	var rule model.MonitorAlertRule
	if err := s.db.First(&rule, representative.RuleID).Error; err != nil || rule.Status != 1 || !rule.NotifyEnabled || rule.NotifyRuleID == 0 {
		return
	}
	repeatAfter := time.Duration(normalizeAggregationWindow(aggregation.RepeatIntervalSeconds)) * time.Second
	s.monitorNotifyMu.Lock()
	var latest model.MonitorAlertEvent
	err := s.db.Where("aggregation_key = ? AND status = ? AND last_notify_at IS NOT NULL", key, "firing").Order("last_notify_at DESC").First(&latest).Error
	if err == nil && latest.LastNotifyAt != nil && now.Sub(*latest.LastNotifyAt) < repeatAfter {
		s.monitorNotifyMu.Unlock()
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		s.monitorNotifyMu.Unlock()
		return
	}
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("aggregation_key = ? AND status = ?", key, "firing").Updates(map[string]any{
		"last_notify_at": &now, "notify_count": gorm.Expr("notify_count + ?", 1),
	}).Error; err != nil {
		s.monitorNotifyMu.Unlock()
		return
	}
	s.monitorNotifyMu.Unlock()

	s.dispatchMonitorAggregationNotification(rule, aggregation, events, now)
	for _, event := range events {
		s.appendMonitorAlertTimeline(event.ID, "aggregation_notification", "已发送聚合告警通知", fmt.Sprintf("聚合规则：%s，本批共 %d 条告警", aggregation.Name, len(events)), "系统", map[string]any{
			"aggregationKey": key, "eventCount": len(events), "notifyRuleId": rule.NotifyRuleID,
		})
	}
}

func (s *Service) dispatchMonitorAggregationNotification(rule model.MonitorAlertRule, aggregation model.MonitorAggregationRule, events []model.MonitorAlertEvent, now time.Time) {
	representative := events[0]
	samples := make([]string, 0, min(len(events), 5))
	for index, event := range events {
		if index == 5 {
			break
		}
		samples = append(samples, event.LabelsJSON)
	}
	summary := fmt.Sprintf("【聚合告警】%s：%d 条同类告警", representative.RuleName, len(events))
	detail := fmt.Sprintf("聚合规则：%s\n收敛窗口：%d 秒\n命中数量：%d\n样本标签：\n%s", aggregation.Name, normalizeAggregationWindow(aggregation.WindowSeconds), len(events), strings.Join(samples, "\n"))
	s.DispatchNotifyRule(rule.NotifyRuleID, NotifyEvent{
		Scope: "monitor", Event: "firing", TargetID: representative.ID, TargetName: representative.RuleName + "（聚合）", Status: "firing",
		Summary: summary, Detail: detail, StartedAt: &representative.FirstTriggerAt,
		Extra: map[string]string{
			"alertName": representative.RuleName, "severity": representative.Severity, "aggregationRule": aggregation.Name,
			"aggregationCount": strconv.Itoa(len(events)), "aggregationWindowSeconds": strconv.Itoa(normalizeAggregationWindow(aggregation.WindowSeconds)),
			"labels": representative.LabelsJSON, "datasourceName": representative.DatasourceName, "sentAt": now.Format(time.RFC3339),
		},
	})
}

func (s *Service) appendMonitorAlertTimeline(eventID uint, eventType, title, detail, operator string, metadata map[string]any) {
	if eventID == 0 {
		return
	}
	metadataJSON := "{}"
	if len(metadata) > 0 {
		if data, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(data)
		}
	}
	_ = s.db.Create(&model.MonitorAlertEventTimeline{
		AlertEventID: eventID,
		EventType:    strings.TrimSpace(eventType),
		Title:        strings.TrimSpace(title),
		Detail:       strings.TrimSpace(detail),
		Operator:     strings.TrimSpace(operator),
		MetadataJSON: metadataJSON,
	}).Error
}

func applyMonitorEventAggregation(event *model.MonitorAlertEvent, updates map[string]any, aggregation model.MonitorAggregationRule, key string) {
	updates["aggregation_key"] = key
	updates["aggregate_rule_id"] = aggregation.ID
	updates["aggregate_rule_name"] = aggregation.Name
	event.AggregationKey = key
	event.AggregateRuleID = aggregation.ID
	event.AggregateRuleName = aggregation.Name
}

func (s *Service) upsertMonitorAlertEvent(rule model.MonitorAlertRule, sample PromMetricSample, fp string, value float64) {
	now := time.Now()
	labelsBytes, _ := json.Marshal(sample.Metric)
	summary := fmt.Sprintf("%s 当前值 %.4f %s %.4f", rule.Name, value, rule.Comparator, rule.Threshold)
	silenceRule, silenced := s.matchMonitorSilenceRule(rule, sample.Metric)
	aggregationRule, aggregationKey, aggregated := s.matchMonitorAggregationRule(rule, sample.Metric)
	var existing model.MonitorAlertEvent
	err := s.db.Where("rule_id = ? AND fingerprint = ? AND status IN ?", rule.ID, fp, []string{"pending", "firing", "claimed", "silenced"}).First(&existing).Error
	if err == nil {
		previousAggregateRuleID := existing.AggregateRuleID
		updates := map[string]any{
			"current_value": value, "last_trigger_at": now, "summary": summary,
		}
		shouldNotify := false
		if existing.Status == "pending" && !silenced && rule.ForSeconds > 0 && now.Sub(existing.FirstTriggerAt) >= time.Duration(rule.ForSeconds)*time.Second {
			updates["status"] = "firing"
			shouldNotify = true
		}
		if silenced && silenceRule != nil {
			updates["silenced"] = true
			updates["silence_rule_id"] = silenceRule.ID
			updates["silence_rule_name"] = silenceRule.Name
			updates["status"] = "silenced"
		} else if existing.Status == "silenced" {
			updates["silenced"] = false
			updates["silence_rule_id"] = 0
			updates["silence_rule_name"] = ""
			updates["status"] = "firing"
			shouldNotify = true
		}
		if aggregated && aggregationRule != nil {
			applyMonitorEventAggregation(&existing, updates, *aggregationRule, aggregationKey)
		}
		previousStatus := existing.Status
		if err := s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return
		}
		if nextStatus, ok := updates["status"].(string); ok && nextStatus != previousStatus {
			switch nextStatus {
			case "firing":
				if previousStatus == "silenced" {
					s.appendMonitorAlertTimeline(existing.ID, "firing", "屏蔽已结束，告警仍在触发", summary, "系统", nil)
				} else {
					s.appendMonitorAlertTimeline(existing.ID, "firing", "告警正式触发", summary, "系统", nil)
				}
			case "silenced":
				s.appendMonitorAlertTimeline(existing.ID, "silenced", "命中告警屏蔽", firstNonEmpty(existing.SilenceRuleName, silenceRule.Name), "系统", nil)
			}
		}
		if aggregated && aggregationRule != nil && previousAggregateRuleID != aggregationRule.ID {
			s.appendMonitorAlertTimeline(existing.ID, "aggregated", "命中聚合收敛规则", aggregationRule.Name, "系统", map[string]any{"aggregationKey": aggregationKey})
		}
		if existing.Status == "firing" && !silenced {
			shouldNotify = true
		}
		if shouldNotify && rule.NotifyEnabled && rule.NotifyRuleID > 0 {
			existing.Status = "firing"
			existing.CurrentValue = value
			existing.Summary = summary
			s.notifyMonitorAlertIfAllowed(&existing, rule, aggregationRule, "firing")
		}
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	event := model.MonitorAlertEvent{
		RuleID: rule.ID, RuleName: rule.Name, DatasourceID: rule.DatasourceID, DatasourceName: rule.DatasourceName,
		Fingerprint: fp, Severity: rule.Severity, Status: "firing", Metric: firstNonEmpty(sample.Metric["__name__"], rule.PromQL),
		LabelsJSON: string(labelsBytes), AnnotationsJSON: rule.AnnotationsJSON, CurrentValue: value, Threshold: rule.Threshold,
		Summary: summary, FirstTriggerAt: now, LastTriggerAt: now,
	}
	if silenced && silenceRule != nil {
		event.Status = "silenced"
		event.Silenced = true
		event.SilenceRuleID = silenceRule.ID
		event.SilenceRuleName = silenceRule.Name
	}
	if !event.Silenced && rule.ForSeconds > 0 {
		event.Status = "pending"
	}
	if aggregated && aggregationRule != nil {
		event.AggregationKey = aggregationKey
		event.AggregateRuleID = aggregationRule.ID
		event.AggregateRuleName = aggregationRule.Name
	}
	if err := s.db.Create(&event).Error; err == nil {
		title := "告警事件已创建"
		if event.Status == "pending" {
			title = "等待持续时间"
		} else if event.Status == "silenced" {
			title = "告警已被屏蔽"
		}
		s.appendMonitorAlertTimeline(event.ID, event.Status, title, summary, "系统", map[string]any{"fingerprint": event.Fingerprint})
		if aggregated && aggregationRule != nil {
			s.appendMonitorAlertTimeline(event.ID, "aggregated", "命中聚合收敛规则", aggregationRule.Name, "系统", map[string]any{"aggregationKey": aggregationKey})
		}
		if event.Status == "firing" && !event.Silenced && rule.NotifyEnabled && rule.NotifyRuleID > 0 {
			s.notifyMonitorAlertIfAllowed(&event, rule, aggregationRule, "firing")
		}
	}
}

func (s *Service) recoverInactiveMonitorEvents(rule model.MonitorAlertRule, active map[string]bool) {
	var events []model.MonitorAlertEvent
	if err := s.db.Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "firing", "claimed", "silenced"}).Find(&events).Error; err != nil {
		return
	}
	now := time.Now()
	for _, event := range events {
		if active[event.Fingerprint] {
			continue
		}
		wasPending := event.Status == "pending"
		_ = s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", event.ID).Updates(map[string]any{
			"status": "recovered", "recovered_at": &now,
		}).Error
		if wasPending {
			s.appendMonitorAlertTimeline(event.ID, "recovered", "等待持续已取消", "告警条件已恢复，未达到持续时间，不发送恢复通知", "系统", nil)
		} else {
			s.appendMonitorAlertTimeline(event.ID, "recovered", "告警已自动恢复", "指标已不再满足告警条件", "系统", nil)
		}
		event.Status = "recovered"
		event.RecoveredAt = &now
		// 恢复通知只能对应一条已经发出过触发通知的告警。pending 事件在持续
		// 时间内恢复时并未对外告警，因此不能产生一条孤立的“恢复”消息。
		if shouldNotifyMonitorRecovery(rule, event) {
			if event.AggregateRuleID == 0 || event.AggregationKey == "" {
				s.dispatchMonitorNotification(rule, event, "recovered")
			} else {
				s.notifyMonitorAggregationRecovered(rule, event)
			}
		}
	}
}

func shouldNotifyMonitorRecovery(rule model.MonitorAlertRule, event model.MonitorAlertEvent) bool {
	return rule.NotifyEnabled && rule.NotifyRuleID > 0 && rule.NotifyRecoveryEnabled &&
		(event.LastNotifyAt != nil || event.NotifyCount > 0)
}

// A recovered event in an aggregation bucket only notifies when the final
// firing event in that bucket has recovered, preventing a recovery storm.
func (s *Service) notifyMonitorAggregationRecovered(rule model.MonitorAlertRule, event model.MonitorAlertEvent) {
	s.monitorNotifyMu.Lock()
	defer s.monitorNotifyMu.Unlock()
	var activeCount int64
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("aggregation_key = ? AND status IN ?", event.AggregationKey, []string{"pending", "firing", "claimed"}).Count(&activeCount).Error; err != nil || activeCount > 0 {
		return
	}
	s.DispatchNotifyRule(rule.NotifyRuleID, NotifyEvent{
		Scope: "monitor", Event: "recovered", TargetID: event.ID, TargetName: event.RuleName + "（聚合）", Status: "recovered",
		Summary: fmt.Sprintf("【聚合恢复】%s：同组告警已全部恢复", event.RuleName), Detail: event.LabelsJSON,
		StartedAt: &event.FirstTriggerAt, FinishedAt: event.RecoveredAt,
		Extra: map[string]string{"alertName": event.RuleName, "severity": event.Severity, "aggregationRule": event.AggregateRuleName, "labels": event.LabelsJSON},
	})
	s.appendMonitorAlertTimeline(event.ID, "aggregation_recovered", "已发送聚合恢复通知", "同一聚合分组内的告警均已恢复", "系统", map[string]any{"aggregationKey": event.AggregationKey})
}

func (s *Service) dispatchMonitorNotification(rule model.MonitorAlertRule, event model.MonitorAlertEvent, status string) {
	s.DispatchNotifyRule(rule.NotifyRuleID, NotifyEvent{
		Scope: "monitor", Event: status, TargetID: event.ID, TargetName: event.RuleName, Status: status,
		Summary: event.Summary, Detail: event.LabelsJSON, StartedAt: &event.FirstTriggerAt, FinishedAt: event.RecoveredAt,
		Extra: map[string]string{
			"alertName": event.RuleName, "severity": event.Severity, "instance": extractLabel(event.LabelsJSON, "instance"),
			"value": fmt.Sprintf("%.4f", event.CurrentValue), "threshold": fmt.Sprintf("%.4f", event.Threshold),
			"labels": event.LabelsJSON, "annotations": event.AnnotationsJSON, "datasourceName": event.DatasourceName,
		},
	})
}

func extractLabel(raw, key string) string {
	var labels map[string]string
	_ = json.Unmarshal([]byte(raw), &labels)
	return labels[key]
}

func (s *Service) ListMonitorAlertEvents(pageNum, pageSize int, keyword, status, severity string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorAlertEvent{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("rule_name LIKE ? OR metric LIKE ? OR summary LIKE ? OR labels_json LIKE ?", like, like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(severity) != "" {
		query = query.Where("severity = ?", normalizeSeverity(severity))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.MonitorAlertEvent
	if err := query.Order("last_trigger_at DESC, id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetMonitorAlertEventDetail(id uint) (map[string]any, error) {
	var event model.MonitorAlertEvent
	if err := s.db.First(&event, id).Error; err != nil {
		return nil, err
	}
	var timelines []model.MonitorAlertEventTimeline
	if err := s.db.Where("alert_event_id = ?", id).Order("created_at ASC, id ASC").Find(&timelines).Error; err != nil {
		return nil, err
	}
	if len(timelines) == 0 {
		timelines = append(timelines, model.MonitorAlertEventTimeline{
			AlertEventID: id, EventType: "firing", Title: "告警事件已创建", Detail: event.Summary, Operator: "系统", CreatedAt: event.FirstTriggerAt,
		})
		if event.ClaimedBy != "" {
			claimedAt := event.UpdatedAt
			if event.ClaimedAt != nil {
				claimedAt = *event.ClaimedAt
			}
			timelines = append(timelines, model.MonitorAlertEventTimeline{
				AlertEventID: id, EventType: "claimed", Title: "告警已认领", Detail: event.HandleNote, Operator: event.ClaimedBy, CreatedAt: claimedAt,
			})
		}
		if event.RecoveredAt != nil {
			title := "告警已自动恢复"
			if event.Status == "resolved" {
				title = "告警已人工关闭"
			}
			timelines = append(timelines, model.MonitorAlertEventTimeline{
				AlertEventID: id, EventType: event.Status, Title: title, Detail: event.ResolveNote, Operator: "系统", CreatedAt: *event.RecoveredAt,
			})
		}
	}
	var actions []model.MonitorAlertAction
	if err := s.db.Where("alert_event_id = ?", id).Order("id DESC").Find(&actions).Error; err != nil {
		return nil, err
	}
	var notifyLogs []model.NotifySendLog
	if err := s.db.Where("scope = ? AND target_id = ?", "monitor", id).Order("id DESC").Limit(20).Find(&notifyLogs).Error; err != nil {
		return nil, err
	}

	var rule model.MonitorAlertRule
	_ = s.db.First(&rule, event.RuleID).Error
	notificationState := map[string]any{
		"allowed": true, "reason": "当前允许发送", "notifyCount": event.NotifyCount,
		"maxNotifyCount": rule.MaxNotifyCount, "lastNotifyAt": event.LastNotifyAt,
	}
	if event.Status == "recovered" || event.Status == "resolved" {
		notificationState["allowed"] = false
		notificationState["reason"] = "告警已结束，无需继续通知"
	} else if event.Silenced {
		notificationState["allowed"] = false
		notificationState["reason"] = "命中屏蔽规则：" + firstNonEmpty(event.SilenceRuleName, "未命名规则")
	} else if rule.MaxNotifyCount > 0 && event.NotifyCount >= rule.MaxNotifyCount {
		notificationState["allowed"] = false
		notificationState["reason"] = "已达到最大发送次数"
	} else if event.LastNotifyAt != nil {
		nextNotifyAt := event.LastNotifyAt.Add(time.Duration(normalizeNotifyRepeatInterval(rule.NotifyRepeatIntervalSeconds)) * time.Second)
		notificationState["nextNotifyAt"] = nextNotifyAt
		if time.Now().Before(nextNotifyAt) {
			notificationState["allowed"] = false
			notificationState["reason"] = "等待重复通知间隔"
		}
	}
	if event.AggregateRuleID > 0 {
		notificationState["aggregationRule"] = event.AggregateRuleName
		notificationState["aggregationKey"] = event.AggregationKey
	}
	return map[string]any{
		"event": event, "timelines": timelines, "actions": actions,
		"notifyLogs": notifyLogs, "notificationState": notificationState,
	}, nil
}

func (s *Service) ClaimMonitorAlertEvent(payload MonitorAlertEventActionPayload) error {
	if payload.ID == 0 {
		return errors.New("告警事件 ID 不能为空")
	}
	now := time.Now()
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"status": "claimed", "claimed_by": strings.TrimSpace(payload.ClaimedBy), "claimed_at": &now, "handle_note": strings.TrimSpace(payload.HandleNote),
	}).Error; err != nil {
		return err
	}
	s.appendMonitorAlertTimeline(payload.ID, "claimed", "告警已认领", payload.HandleNote, payload.ClaimedBy, nil)
	return nil
}

func (s *Service) ResolveMonitorAlertEvent(payload MonitorAlertEventActionPayload) error {
	if payload.ID == 0 {
		return errors.New("告警事件 ID 不能为空")
	}
	now := time.Now()
	if err := s.db.Model(&model.MonitorAlertEvent{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"status": "resolved", "resolve_note": strings.TrimSpace(payload.HandleNote), "recovered_at": &now, "resolved_at": &now,
	}).Error; err != nil {
		return err
	}
	s.appendMonitorAlertTimeline(payload.ID, "resolved", "告警已人工关闭", payload.HandleNote, "操作人", nil)
	return nil
}

func (s *Service) BatchUpdateMonitorAlertEvents(payload MonitorAlertEventBatchPayload) error {
	ids, err := normalizeMonitorBatchIDs(payload.IDs)
	if err != nil {
		return err
	}
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	updates := map[string]any{}
	query := s.db.Model(&model.MonitorAlertEvent{}).Where("id IN ?", ids)
	switch action {
	case "claim":
		now := time.Now()
		updates["status"] = "claimed"
		updates["claimed_by"] = strings.TrimSpace(payload.ClaimedBy)
		updates["claimed_at"] = &now
		updates["handle_note"] = strings.TrimSpace(payload.HandleNote)
		query = query.Where("status IN ?", []string{"pending", "firing"})
	case "resolve":
		now := time.Now()
		updates["status"] = "resolved"
		updates["resolve_note"] = strings.TrimSpace(payload.HandleNote)
		updates["recovered_at"] = &now
		updates["resolved_at"] = &now
		query = query.Where("status IN ?", []string{"pending", "firing", "claimed", "silenced"})
	case "delete":
		return s.db.Where("id IN ?", ids).Delete(&model.MonitorAlertEvent{}).Error
	default:
		return errors.New("不支持的批量操作")
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if action == "claim" {
			return errors.New("所选事件没有可认领的等待持续或触发中告警")
		}
		return errors.New("所选事件没有可关闭的未结束告警")
	}
	for _, id := range ids {
		if action == "claim" {
			s.appendMonitorAlertTimeline(id, "claimed", "告警已批量认领", payload.HandleNote, payload.ClaimedBy, nil)
		} else if action == "resolve" {
			s.appendMonitorAlertTimeline(id, "resolved", "告警已批量关闭", payload.HandleNote, payload.ClaimedBy, nil)
		}
	}
	return nil
}

func (s *Service) GetMonitorOverview(startAt, endAt *time.Time) (map[string]any, error) {
	finishedStatuses := []string{"recovered", "resolved", "closed"}
	// Earlier event records can have only resolved_at (or, for imported legacy
	// records, only updated_at). Always use the first available end timestamp so
	// the overview and alert-event list describe the same history.
	finishedAt := "COALESCE(recovered_at, resolved_at, updated_at)"
	withinRange := func(query *gorm.DB, column string) *gorm.DB {
		if startAt != nil {
			query = query.Where(column+" >= ?", *startAt)
		}
		if endAt != nil {
			query = query.Where(column+" < ?", *endAt)
		}
		return query
	}
	var datasourceCount, healthyDatasourceCount, ruleCount, activeRuleCount, successfulRuleCount int64
	var firingCount, recoveredCount, rangeRecoveredCount int64
	var unclaimedCount, criticalCount, unhealthyDatasourceCount, evalFailedRuleCount int64
	var notificationFailedCount, notificationTotalCount, notificationSuccessCount, rangeTriggeredCount int64
	_ = s.db.Model(&model.MonitorDatasource{}).Count(&datasourceCount).Error
	_ = s.db.Model(&model.MonitorDatasource{}).Where("status = ? AND health_status IN ?", 1, []string{"healthy", "normal", "ok"}).Count(&healthyDatasourceCount).Error
	_ = s.db.Model(&model.MonitorAlertRule{}).Count(&ruleCount).Error
	_ = s.db.Model(&model.MonitorAlertRule{}).Where("status = ?", 1).Count(&activeRuleCount).Error
	_ = s.db.Model(&model.MonitorAlertRule{}).Where("status = ? AND (last_eval_status IN ? OR last_eval_status = '' OR last_eval_status IS NULL)", 1, []string{"success", "ok"}).Count(&successfulRuleCount).Error
	// Current risk always represents the platform now; the selected range is only for historical statistics.
	_ = s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ?", []string{"pending", "firing", "claimed"}).Count(&firingCount).Error
	_ = s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ? AND (claimed_by = '' OR claimed_by IS NULL)", []string{"pending", "firing"}).Count(&unclaimedCount).Error
	_ = s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ? AND severity IN ?", []string{"pending", "firing", "claimed"}, []string{"P0", "P1"}).Count(&criticalCount).Error
	_ = withinRange(s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ?", finishedStatuses), finishedAt).Count(&recoveredCount).Error
	_ = s.db.Model(&model.MonitorDatasource{}).Where("status = ? AND health_status = ?", 1, "unhealthy").Count(&unhealthyDatasourceCount).Error
	_ = s.db.Model(&model.MonitorAlertRule{}).Where("status = ? AND last_eval_status = ?", 1, "failed").Count(&evalFailedRuleCount).Error
	notifyQuery := s.db.Model(&model.NotifySendLog{}).Where("scope = ?", "monitor")
	if startAt == nil && endAt == nil {
		notifyQuery = notifyQuery.Where("created_at >= ?", time.Now().Add(-24*time.Hour))
	} else {
		notifyQuery = withinRange(notifyQuery, "created_at")
	}
	_ = notifyQuery.Count(&notificationTotalCount).Error
	_ = notifyQuery.Where("status = ?", "failed").Count(&notificationFailedCount).Error
	_ = notifyQuery.Where("status IN ?", []string{"success", "sent"}).Count(&notificationSuccessCount).Error
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	triggeredQuery := s.db.Model(&model.MonitorAlertEvent{})
	recoveredQuery := s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ?", finishedStatuses)
	if startAt == nil && endAt == nil {
		triggeredQuery = triggeredQuery.Where("created_at >= ?", dayStart)
		recoveredQuery = recoveredQuery.Where(finishedAt+" >= ?", dayStart)
	} else {
		triggeredQuery = withinRange(triggeredQuery, "created_at")
		recoveredQuery = withinRange(recoveredQuery, finishedAt)
	}
	_ = triggeredQuery.Count(&rangeTriggeredCount).Error
	_ = recoveredQuery.Count(&rangeRecoveredCount).Error
	severityRows := []map[string]any{}
	for _, severity := range []string{"P0", "P1", "P2", "P3"} {
		var count int64
		_ = s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ? AND severity = ?", []string{"pending", "firing", "claimed"}, severity).Count(&count).Error
		severityRows = append(severityRows, map[string]any{"severity": severity, "count": count})
	}
	var recent []model.MonitorAlertEvent
	_ = s.db.Where("status IN ?", []string{"pending", "firing", "claimed"}).Order("last_trigger_at DESC, id DESC").Limit(8).Find(&recent).Error
	var recentHandled []model.MonitorAlertEvent
	handledQuery := s.db.Where("claimed_at IS NOT NULL OR recovered_at IS NOT NULL")
	if startAt == nil && endAt == nil {
		handledQuery = handledQuery.Where("created_at >= ?", time.Now().Add(-30*24*time.Hour))
	} else {
		handledQuery = withinRange(handledQuery, "created_at")
	}
	_ = handledQuery.Find(&recentHandled).Error
	var totalAckSeconds, totalRecoverSeconds int64
	var ackSamples, recoverSamples int64
	for _, event := range recentHandled {
		if event.ClaimedAt != nil && event.ClaimedAt.After(event.FirstTriggerAt) {
			totalAckSeconds += int64(event.ClaimedAt.Sub(event.FirstTriggerAt).Seconds())
			ackSamples++
		}
		if event.RecoveredAt != nil && event.RecoveredAt.After(event.FirstTriggerAt) {
			totalRecoverSeconds += int64(event.RecoveredAt.Sub(event.FirstTriggerAt).Seconds())
			recoverSamples++
		}
	}
	average := func(total, count int64) int64 {
		if count == 0 {
			return 0
		}
		return total / count
	}
	trend := make([]map[string]any, 0)
	trendStart, trendEnd := dayStart, dayStart.AddDate(0, 0, 1)
	if startAt != nil {
		trendStart = time.Date(startAt.Year(), startAt.Month(), startAt.Day(), 0, 0, 0, 0, startAt.Location())
	}
	if endAt != nil {
		trendEnd = *endAt
	}
	for cursor := trendStart; cursor.Before(trendEnd); cursor = cursor.AddDate(0, 0, 1) {
		next := cursor.AddDate(0, 0, 1)
		var triggered, recovered int64
		_ = s.db.Model(&model.MonitorAlertEvent{}).Where("created_at >= ? AND created_at < ?", cursor, next).Count(&triggered).Error
		_ = s.db.Model(&model.MonitorAlertEvent{}).Where("status IN ? AND "+finishedAt+" >= ? AND "+finishedAt+" < ?", finishedStatuses, cursor, next).Count(&recovered).Error
		trend = append(trend, map[string]any{"date": cursor.Format("01-02"), "triggered": triggered, "recovered": recovered})
	}

	activities := make([]map[string]any, 0, 10)
	var recoveredEvents []model.MonitorAlertEvent
	_ = withinRange(s.db.Where("status IN ?", finishedStatuses), finishedAt).Order(finishedAt + " DESC").Limit(3).Find(&recoveredEvents).Error
	for _, item := range recoveredEvents {
		activities = append(activities, map[string]any{"type": "recovered", "title": item.RuleName, "detail": firstNonEmpty(item.Summary, "告警已恢复"), "time": monitorAlertFinishedAt(item)})
	}
	var unhealthySources []model.MonitorDatasource
	_ = s.db.Where("status = ? AND health_status = ?", 1, "unhealthy").Order("updated_at DESC").Limit(3).Find(&unhealthySources).Error
	for _, item := range unhealthySources {
		activities = append(activities, map[string]any{"type": "datasource", "title": item.Name, "detail": firstNonEmpty(item.LastError, "数据源健康检查异常"), "time": item.UpdatedAt})
	}
	var failedNotifications []model.NotifySendLog
	_ = notifyQuery.Where("status = ?", "failed").Order("created_at DESC").Limit(3).Find(&failedNotifications).Error
	for _, item := range failedNotifications {
		activities = append(activities, map[string]any{"type": "notification", "title": firstNonEmpty(item.RuleName, item.ChannelName), "detail": firstNonEmpty(item.ErrorText, item.Summary, "通知发送失败"), "time": item.CreatedAt})
	}
	var failedRules []model.MonitorAlertRule
	_ = s.db.Where("status = ? AND last_eval_status = ?", 1, "failed").Order("last_eval_at DESC").Limit(3).Find(&failedRules).Error
	for _, item := range failedRules {
		activities = append(activities, map[string]any{"type": "rule", "title": item.Name, "detail": firstNonEmpty(item.LastEvalMessage, "规则执行失败"), "time": item.LastEvalAt})
	}
	sort.SliceStable(activities, func(i, j int) bool {
		leftTime, leftOK := overviewActivityTime(activities[i]["time"])
		rightTime, rightOK := overviewActivityTime(activities[j]["time"])
		return leftOK && (!rightOK || leftTime.After(rightTime))
	})
	if len(activities) > 8 {
		activities = activities[:8]
	}
	return map[string]any{
		"datasourceCount": datasourceCount, "healthyDatasourceCount": healthyDatasourceCount,
		"ruleCount": ruleCount, "activeRuleCount": activeRuleCount, "successfulRuleCount": successfulRuleCount,
		"firingCount": firingCount, "recoveredCount": recoveredCount, "rangeRecoveredCount": rangeRecoveredCount, "severity": severityRows, "recentEvents": recent,
		"unclaimedCount": unclaimedCount, "criticalCount": criticalCount,
		"unhealthyDatasourceCount": unhealthyDatasourceCount, "evalFailedRuleCount": evalFailedRuleCount,
		"notificationFailedCount": notificationFailedCount, "notificationTotalCount": notificationTotalCount, "notificationSuccessCount": notificationSuccessCount, "rangeTriggeredCount": rangeTriggeredCount,
		"mttaSeconds": average(totalAckSeconds, ackSamples), "mttrSeconds": average(totalRecoverSeconds, recoverSamples),
		"trend": trend, "recentActivities": activities, "refreshedAt": time.Now(),
	}, nil
}

// GetMonitorCommandCenter is the read model for the operations command
// center. It uses the platform's already-managed asset inventory rather than
// fabricating geographic data or depending on an external map service.
func (s *Service) GetMonitorCommandCenter() (map[string]any, error) {
	overview, err := s.GetMonitorOverview(nil, nil)
	if err != nil {
		return nil, err
	}

	var hostCount, onlineHostCount, databaseCount, connectedDatabaseCount, clusterCount, serviceCount int64
	_ = s.db.Model(&model.AssetHost{}).Where("status = ?", 1).Count(&hostCount).Error
	_ = s.db.Model(&model.AssetHost{}).Where("status = ? AND alive_status = ?", 1, 1).Count(&onlineHostCount).Error
	_ = s.db.Model(&model.AssetDatabase{}).Where("status = ?", 1).Count(&databaseCount).Error
	_ = s.db.Model(&model.AssetDatabase{}).Where("status = ? AND connect_status = ?", 1, 1).Count(&connectedDatabaseCount).Error
	_ = s.db.Model(&model.K8sCluster{}).Count(&clusterCount).Error
	_ = s.db.Model(&model.AssetService{}).Where("status = ?", 1).Count(&serviceCount).Error

	type regionRow struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	regions := make([]regionRow, 0)
	_ = s.db.Model(&model.AssetHost{}).
		Select("COALESCE(NULLIF(region, ''), '未分配区域') AS name, COUNT(*) AS count").
		Where("status = ?", 1).
		Group("COALESCE(NULLIF(region, ''), '未分配区域')").
		Order("count DESC, name ASC").
		Limit(6).
		Scan(&regions).Error

	type ruleRow struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	topRules := make([]ruleRow, 0)
	_ = s.db.Model(&model.MonitorAlertEvent{}).
		Select("COALESCE(NULLIF(rule_name, ''), '未命名规则') AS name, COUNT(*) AS count").
		Where("status IN ?", []string{"pending", "firing", "claimed"}).
		Group("COALESCE(NULLIF(rule_name, ''), '未命名规则')").
		Order("count DESC, name ASC").
		Limit(5).
		Scan(&topRules).Error

	type hostRow struct {
		Name        string    `json:"name"`
		Region      string    `json:"region"`
		AliveStatus int       `json:"aliveStatus"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}
	hotHosts := make([]hostRow, 0)
	_ = s.db.Model(&model.AssetHost{}).
		Select("host_name AS name, COALESCE(NULLIF(region, ''), '未分配区域') AS region, alive_status, updated_at").
		Where("status = ?", 1).
		Order("alive_status ASC, updated_at DESC").
		Limit(5).
		Scan(&hotHosts).Error

	recentAlerts := make([]model.MonitorAlertEvent, 0)
	_ = s.db.Where("status IN ?", []string{"pending", "firing", "claimed"}).
		Order("last_trigger_at DESC, id DESC").
		Limit(6).
		Find(&recentAlerts).Error

	assetTotal := hostCount + databaseCount + clusterCount + serviceCount
	coverage := 0.0
	if hostCount > 0 {
		coverage = float64(onlineHostCount) * 100 / float64(hostCount)
	}
	return map[string]any{
		"overview": overview,
		"assetSummary": map[string]any{
			"total": assetTotal, "hosts": hostCount, "onlineHosts": onlineHostCount,
			"databases": databaseCount, "connectedDatabases": connectedDatabaseCount,
			"clusters": clusterCount, "services": serviceCount, "coverage": coverage,
		},
		"resourceComposition": []map[string]any{
			{"name": "物理/云主机", "count": hostCount, "tone": "cyan"},
			{"name": "Kubernetes 集群", "count": clusterCount, "tone": "blue"},
			{"name": "数据库", "count": databaseCount, "tone": "amber"},
			{"name": "服务", "count": serviceCount, "tone": "violet"},
		},
		"regions": regions, "topRules": topRules, "recentAlerts": recentAlerts,
		"hotHosts": hotHosts, "refreshedAt": time.Now(),
	}, nil
}

func monitorAlertFinishedAt(event model.MonitorAlertEvent) time.Time {
	if event.RecoveredAt != nil && !event.RecoveredAt.IsZero() {
		return *event.RecoveredAt
	}
	if event.ResolvedAt != nil && !event.ResolvedAt.IsZero() {
		return *event.ResolvedAt
	}
	return event.UpdatedAt
}

func overviewActivityTime(value any) (time.Time, bool) {
	switch item := value.(type) {
	case time.Time:
		return item, !item.IsZero()
	case *time.Time:
		if item != nil {
			return *item, !item.IsZero()
		}
	}
	return time.Time{}, false
}

func normalizeDashboardLayout(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "list":
		return "list"
	default:
		return "grid"
	}
}

func normalizePanelChartType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "table":
		return "table"
	case "line":
		return "line"
	case "bar":
		return "bar"
	case "gauge":
		return "gauge"
	default:
		return "stat"
	}
}

func normalizePanelSpan(value int) int {
	if value < 6 {
		return 6
	}
	if value > 24 {
		return 24
	}
	return value
}

func (s *Service) ListMonitorDashboards(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.MonitorDashboard{})
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
	var list []model.MonitorDashboard
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		var panelCount int64
		_ = s.db.Model(&model.MonitorDashboardPanel{}).Where("dashboard_id = ?", item.ID).Count(&panelCount).Error
		rows = append(rows, map[string]any{
			"id": item.ID, "name": item.Name, "layout": item.Layout, "status": item.Status,
			"description": item.Description, "panelCount": panelCount, "createTime": item.CreatedAt, "updateTime": item.UpdatedAt,
		})
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetMonitorDashboard(id uint) (map[string]any, error) {
	var dashboard model.MonitorDashboard
	if err := s.db.First(&dashboard, id).Error; err != nil {
		return nil, err
	}
	var panels []model.MonitorDashboardPanel
	if err := s.db.Where("dashboard_id = ?", id).Order("sort ASC, id ASC").Find(&panels).Error; err != nil {
		return nil, err
	}
	return map[string]any{"dashboard": dashboard, "panels": panels}, nil
}

func (s *Service) SaveMonitorDashboard(payload MonitorDashboardPayload) (*model.MonitorDashboard, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.New("dashboard name is required")
	}
	updates := map[string]any{
		"name":        Trimmed(payload.Name),
		"layout":      normalizeDashboardLayout(payload.Layout),
		"status":      normalizeMonitorStatus(payload.Status),
		"description": Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		if err := s.db.Model(&model.MonitorDashboard{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
		var item model.MonitorDashboard
		if err := s.db.First(&item, payload.ID).Error; err != nil {
			return nil, err
		}
		return &item, nil
	}
	item := model.MonitorDashboard{
		Name:        Trimmed(payload.Name),
		Layout:      normalizeDashboardLayout(payload.Layout),
		Status:      normalizeMonitorStatus(payload.Status),
		Description: Trimmed(payload.Description),
	}
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) DeleteMonitorDashboard(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dashboard_id = ?", id).Delete(&model.MonitorDashboardPanel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.MonitorDashboard{}, id).Error
	})
}

func (s *Service) SaveMonitorDashboardPanel(payload MonitorDashboardPanelPayload) error {
	if payload.DashboardID == 0 {
		return errors.New("dashboard is required")
	}
	if strings.TrimSpace(payload.Title) == "" {
		return errors.New("panel title is required")
	}
	if strings.TrimSpace(payload.PromQL) == "" {
		return errors.New("PromQL is required")
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return err
	}
	if isMonitorLogDatasource(ds.Type) {
		return errors.New("日志数据源不支持 PromQL 监控面板，请选择 Prometheus 或 VictoriaMetrics")
	}
	updates := map[string]any{
		"dashboard_id":    payload.DashboardID,
		"title":           Trimmed(payload.Title),
		"datasource_id":   ds.ID,
		"datasource_name": ds.Name,
		"prom_ql":         strings.TrimSpace(payload.PromQL),
		"unit":            strings.TrimSpace(payload.Unit),
		"chart_type":      normalizePanelChartType(payload.ChartType),
		"span":            normalizePanelSpan(payload.Span),
		"sort":            payload.Sort,
		"status":          normalizeMonitorStatus(payload.Status),
		"description":     Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.MonitorDashboardPanel{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.MonitorDashboardPanel{}).Create(updates).Error
}

func (s *Service) DeleteMonitorDashboardPanel(id uint) error {
	return s.db.Delete(&model.MonitorDashboardPanel{}, id).Error
}

func (s *Service) QueryMonitorDashboardPanel(payload MonitorDashboardPanelQueryPayload) (map[string]any, error) {
	var panel model.MonitorDashboardPanel
	if err := s.db.First(&panel, payload.ID).Error; err != nil {
		return nil, err
	}
	queryDatasourceID := panel.DatasourceID
	if payload.DatasourceID > 0 {
		queryDatasourceID = payload.DatasourceID
	}
	ds, err := s.GetMonitorDatasource(queryDatasourceID)
	if err != nil {
		return nil, err
	}
	if isMonitorLogDatasource(ds.Type) {
		var fallback model.MonitorDatasource
		if err := s.db.Where("status = ? AND type IN ?", 1, []string{"prometheus", "victoriametrics"}).Order("is_default DESC, id DESC").First(&fallback).Error; err != nil {
			return nil, errors.New("监控面板仅支持 Prometheus 或 VictoriaMetrics，请先配置指标数据源")
		}
		ds = &fallback
	}
	var result *PromQueryResult
	if payload.StartAt > 0 && payload.EndAt > payload.StartAt {
		result, err = s.prometheusRangeQuery(*ds, panel.PromQL, time.Unix(payload.StartAt, 0), time.Unix(payload.EndAt, 0), payload.StepSeconds)
	} else {
		result, err = s.prometheusQuery(*ds, panel.PromQL, time.Now())
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"panel": panel, "datasource": ds, "resultType": result.Data.ResultType, "result": result.Data.Result,
	}, nil
}
