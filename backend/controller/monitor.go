package controller

import (
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetMonitorOverview(c *gin.Context) {
	startAt, err := parseMonitorOverviewDate(c.Query("startDate"), false)
	if err != nil {
		httpx.Failed(c, 400, "startDate 格式无效，应为 YYYY-MM-DD")
		return
	}
	endAt, err := parseMonitorOverviewDate(c.Query("endDate"), true)
	if err != nil {
		httpx.Failed(c, 400, "endDate 格式无效，应为 YYYY-MM-DD")
		return
	}
	if startAt != nil && endAt != nil && !startAt.Before(*endAt) {
		httpx.Failed(c, 400, "开始日期必须早于结束日期")
		return
	}
	data, err := ctl.service.GetMonitorOverview(startAt, endAt)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

// GetMonitorCommandCenter returns the compact data set required by the
// monitoring command center. It intentionally aggregates native platform
// assets and monitoring data in one request so the screen has a stable first
// paint and does not fan out into a large number of browser requests.
func (ctl *Controller) GetMonitorCommandCenter(c *gin.Context) {
	data, err := ctl.service.GetMonitorCommandCenter()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func parseMonitorOverviewDate(value string, endOfDay bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return &parsed, nil
}

func (ctl *Controller) GetMonitorDatasourceList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorDatasources(pageNum, pageSize, c.Query("keyword"), c.Query("type"), c.Query("status"), c.Query("env"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorDatasourceOptions(c *gin.Context) {
	data, err := ctl.service.ListMonitorDatasourceOptions()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorDatasourceInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorDatasource(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorDatasource(c *gin.Context) {
	var payload service.MonitorDatasourcePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid datasource payload")
		return
	}
	if err := ctl.service.SaveMonitorDatasource(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMonitorDatasource(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorDatasource(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) TestMonitorDatasource(c *gin.Context) {
	var payload service.MonitorDatasourcePayload
	_ = c.ShouldBindJSON(&payload)
	if err := ctl.service.TestMonitorDatasource(uint(mustAtoi(c.Query("id"))), payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) QueryMonitorPrometheus(c *gin.Context) {
	var payload struct {
		DatasourceID uint   `json:"datasourceId"`
		Query        string `json:"query"`
		Time         int64  `json:"time"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid query payload")
		return
	}
	ts := time.Now()
	if payload.Time > 0 {
		ts = time.Unix(payload.Time, 0)
	}
	data, err := ctl.service.PrometheusInstantQuery(payload.DatasourceID, payload.Query, ts)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) QueryMonitorPrometheusRange(c *gin.Context) {
	var payload service.MonitorRangeQueryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid range query payload")
		return
	}
	data, err := ctl.service.MonitorRangeQuery(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) QueryMonitorLogs(c *gin.Context) {
	var payload service.MonitorLogQueryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid log query payload")
		return
	}
	data, err := ctl.service.QueryMonitorLogs(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorJaegerServices(c *gin.Context) {
	data, err := ctl.service.ListMonitorJaegerServices(uint(mustAtoi(c.Query("datasourceId"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorJaegerOperations(c *gin.Context) {
	data, err := ctl.service.ListMonitorJaegerOperations(uint(mustAtoi(c.Query("datasourceId"))), c.Query("service"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) QueryMonitorTraces(c *gin.Context) {
	var payload service.MonitorTraceQueryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid trace query payload")
		return
	}
	data, err := ctl.service.QueryMonitorTraces(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorTrace(c *gin.Context) {
	data, err := ctl.service.GetMonitorTrace(uint(mustAtoi(c.Query("datasourceId"))), c.Query("traceId"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorElasticsearchIndices(c *gin.Context) {
	data, err := ctl.service.ListMonitorElasticsearchIndices(uint(mustAtoi(c.Query("datasourceId"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorVictoriaLogsStreams(c *gin.Context) {
	data, err := ctl.service.ListMonitorVictoriaLogsStreams(
		uint(mustAtoi(c.Query("datasourceId"))), c.Query("field"), c.Query("query"),
		monitorQueryInt64(c.Query("startAt")), monitorQueryInt64(c.Query("endAt")), mustAtoi(c.Query("limit")),
	)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorLogFields(c *gin.Context) {
	data, err := ctl.service.ListMonitorLogFields(
		uint(mustAtoi(c.Query("datasourceId"))), c.Query("index"), c.Query("query"),
		monitorQueryInt64(c.Query("startAt")), monitorQueryInt64(c.Query("endAt")),
	)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorLogFieldValues(c *gin.Context) {
	data, err := ctl.service.ListMonitorLogFieldValues(
		uint(mustAtoi(c.Query("datasourceId"))), c.Query("index"), c.Query("field"), c.Query("query"),
		monitorQueryInt64(c.Query("startAt")), monitorQueryInt64(c.Query("endAt")), mustAtoi(c.Query("limit")),
	)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func monitorQueryInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func (ctl *Controller) GetMonitorLogShortcuts(c *gin.Context) {
	data, err := ctl.service.ListMonitorLogShortcutsByType(c.GetString("username"), c.Query("datasourceType"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorLogShortcut(c *gin.Context) {
	var payload service.MonitorLogShortcutPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid log shortcut payload")
		return
	}
	if err := ctl.service.SaveMonitorLogShortcutByType(c.GetString("username"), payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMonitorLogShortcut(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid log shortcut payload")
		return
	}
	if err := ctl.service.DeleteMonitorLogShortcut(c.GetString("username"), payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMonitorQueryHistoryList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorQueryHistories(pageNum, pageSize, c.Query("keyword"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorAlertTemplateList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorAlertTemplates(pageNum, pageSize, c.Query("keyword"), c.Query("category"), c.Query("datasourceType"), c.Query("source"), uint(mustAtoi(c.Query("groupId"))))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorAlertTemplateGroups(c *gin.Context) {
	data, err := ctl.service.ListMonitorAlertTemplateGroups()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) SaveMonitorAlertTemplateGroup(c *gin.Context) {
	var payload service.MonitorAlertTemplateGroupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid template group payload")
		return
	}
	data, err := ctl.service.SaveMonitorAlertTemplateGroup(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) DeleteMonitorAlertTemplateGroup(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorAlertTemplateGroup(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMonitorAlertTemplateInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorAlertTemplate(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorAlertTemplate(c *gin.Context) {
	var payload service.MonitorAlertTemplatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid alert template payload")
		return
	}
	data, err := ctl.service.SaveMonitorAlertTemplate(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteMonitorAlertTemplate(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorAlertTemplate(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) ParsePrometheusAlertTemplates(c *gin.Context) {
	var payload struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "请粘贴 Prometheus Rule YAML")
		return
	}
	content := strings.TrimSpace(payload.Content)
	if content == "" {
		httpx.Failed(c, 400, "请粘贴 Prometheus Rule YAML")
		return
	}
	if len([]byte(content)) > 2*1024*1024 {
		httpx.Failed(c, 400, "YAML 内容不能超过 2 MB")
		return
	}
	data, err := service.ParsePrometheusAlertTemplates([]byte(content))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ImportPrometheusAlertTemplates(c *gin.Context) {
	var payload service.MonitorAlertTemplateImportPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "无效的 Prometheus 模板导入参数")
		return
	}
	data, err := ctl.service.ImportPrometheusAlertTemplates(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ExportPrometheusAlertTemplates(c *gin.Context) {
	var payload service.MonitorAlertTemplateExportPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid alert template export payload")
		return
	}
	content, err := ctl.service.ExportPrometheusAlertTemplates(payload.IDs)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	c.Header("Content-Type", "application/x-yaml; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=ops-admin-alert-templates.yaml")
	c.Data(200, "application/x-yaml; charset=utf-8", content)
}

func (ctl *Controller) GetMonitorAlertRuleList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorAlertRules(pageNum, pageSize, c.Query("keyword"), c.Query("status"), c.Query("severity"), c.Query("env"), c.Query("alertType"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorAlertRuleInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorAlertRule(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorAlertRule(c *gin.Context) {
	var payload service.MonitorAlertRulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid alert rule payload")
		return
	}
	if err := ctl.service.SaveMonitorAlertRule(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMonitorAlertRule(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorAlertRule(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateMonitorAlertRuleStatus(c *gin.Context) {
	var payload struct {
		ID     uint `json:"id"`
		Status int  `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdateMonitorAlertRuleStatus(payload.ID, payload.Status); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchUpdateMonitorAlertRules(c *gin.Context) {
	var payload service.MonitorAlertRuleBatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch alert rule payload")
		return
	}
	if err := ctl.service.BatchUpdateMonitorAlertRules(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RunMonitorAlertRule(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid run payload")
		return
	}
	if err := ctl.service.RunMonitorAlertRule(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) PreviewMonitorAlertRule(c *gin.Context) {
	var payload service.MonitorAlertRulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid alert rule preview payload")
		return
	}
	data, err := ctl.service.PreviewMonitorAlertRule(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorSilenceRuleList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorSilenceRules(pageNum, pageSize, c.Query("keyword"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorSilenceRuleInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorSilenceRule(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorSilenceRule(c *gin.Context) {
	var payload service.MonitorSilenceRulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid silence rule payload")
		return
	}
	if err := ctl.service.SaveMonitorSilenceRule(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) PreviewMonitorSilenceRule(c *gin.Context) {
	var payload service.MonitorSilenceRulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid silence rule preview payload")
		return
	}
	data, err := ctl.service.PreviewMonitorSilenceRule(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteMonitorSilenceRule(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorSilenceRule(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchUpdateMonitorSilenceRules(c *gin.Context) {
	var payload service.MonitorRuleBatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid silence rule batch payload")
		return
	}
	if err := ctl.service.BatchUpdateMonitorSilenceRules(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMonitorAggregationRuleList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorAggregationRules(pageNum, pageSize, c.Query("keyword"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorAggregationRuleInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorAggregationRule(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorAggregationRule(c *gin.Context) {
	var payload service.MonitorAggregationRulePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid aggregation rule payload")
		return
	}
	if err := ctl.service.SaveMonitorAggregationRule(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMonitorAggregationRule(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorAggregationRule(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchUpdateMonitorAggregationRules(c *gin.Context) {
	var payload service.MonitorRuleBatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid aggregation rule batch payload")
		return
	}
	if err := ctl.service.BatchUpdateMonitorAggregationRules(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMonitorAlertEventList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorAlertEvents(pageNum, pageSize, c.Query("keyword"), c.Query("status"), c.Query("severity"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorAlertEventDetail(c *gin.Context) {
	data, err := ctl.service.GetMonitorAlertEventDetail(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ClaimMonitorAlertEvent(c *gin.Context) {
	var payload service.MonitorAlertEventActionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid claim payload")
		return
	}
	if err := ctl.service.ClaimMonitorAlertEvent(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) ResolveMonitorAlertEvent(c *gin.Context) {
	var payload service.MonitorAlertEventActionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid resolve payload")
		return
	}
	if err := ctl.service.ResolveMonitorAlertEvent(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchUpdateMonitorAlertEvents(c *gin.Context) {
	var payload service.MonitorAlertEventBatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid alert event batch payload")
		return
	}
	if err := ctl.service.BatchUpdateMonitorAlertEvents(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMonitorDashboardList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListMonitorDashboards(pageNum, pageSize, c.Query("keyword"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorDashboardInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorDashboard(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveMonitorDashboard(c *gin.Context) {
	var payload service.MonitorDashboardPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid dashboard payload")
		return
	}
	data, err := ctl.service.SaveMonitorDashboard(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteMonitorDashboard(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorDashboard(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) SaveMonitorDashboardPanel(c *gin.Context) {
	var payload service.MonitorDashboardPanelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid panel payload")
		return
	}
	if err := ctl.service.SaveMonitorDashboardPanel(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMonitorDashboardPanel(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMonitorDashboardPanel(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) QueryMonitorDashboardPanel(c *gin.Context) {
	var payload service.MonitorDashboardPanelQueryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid query payload")
		return
	}
	data, err := ctl.service.QueryMonitorDashboardPanel(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RunMonitorInspection(c *gin.Context) {
	var payload service.MonitorInspectionRunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid inspection payload")
		return
	}
	data, err := ctl.service.RunMonitorInspection(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetLatestMonitorInspectionRun(c *gin.Context) {
	data, err := ctl.service.GetLatestMonitorInspectionRun(uint(mustAtoi(c.Query("dashboardId"))))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetMonitorInspectionRunInfo(c *gin.Context) {
	data, err := ctl.service.GetMonitorInspectionRun(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}
