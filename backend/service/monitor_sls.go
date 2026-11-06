package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sls "github.com/aliyun/aliyun-log-go-sdk"
	"net"
	"net/http"
	"net/url"
	"ops-admin/backend/model"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var slsResourceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,127}$`)
var slsCrossStore = regexp.MustCompile(`(?i)\b(from|join|union)\b`)

func validateMonitorSLS(endpoint string, cfg model.MonitorSLSConfig) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() != "" || !strings.HasSuffix(u.Hostname(), ".log.aliyuncs.com") {
		return errors.New("SLS Endpoint 请填写地域级 HTTPS 地址，例如 https://cn-hangzhou.log.aliyuncs.com")
	}
	if cfg.Region == "" || !slsResourceName.MatchString(cfg.Project) || cfg.CloudAccountID == 0 {
		return errors.New("请填写地域、Project 并选择阿里云账号")
	}
	expected := cfg.Region + ".log.aliyuncs.com"
	if u.Hostname() != expected && u.Hostname() != cfg.Region+"-intranet.log.aliyuncs.com" {
		return errors.New("Endpoint 与所选地域不匹配，请使用地域级地址，不包含 Project 前缀")
	}
	for _, name := range append(append([]string{}, cfg.Logstores...), cfg.DefaultLogstore) {
		if name != "" && !slsResourceName.MatchString(name) {
			return errors.New("Logstore 名称无效")
		}
	}
	if cfg.DefaultLogstore != "" && !monitorSLSStoreAllowed(cfg, cfg.DefaultLogstore) {
		return errors.New("默认 Logstore 不在允许范围内")
	}
	return nil
}
func monitorSLSStoreAllowed(cfg model.MonitorSLSConfig, store string) bool {
	if !slsResourceName.MatchString(store) {
		return false
	}
	if len(cfg.Logstores) == 0 {
		return true
	}
	for _, allowed := range cfg.Logstores {
		if store == allowed {
			return true
		}
	}
	return false
}
func (s *Service) monitorSLSClient(ds model.MonitorDatasource) (sls.ClientInterface, error) {
	if err := validateMonitorSLS(ds.URL, ds.SLS); err != nil {
		return nil, err
	}
	var account model.AssetCloudAccount
	if err := s.db.First(&account, ds.SLS.CloudAccountID).Error; err != nil {
		return nil, errors.New("SLS 云账号不存在或不可访问")
	}
	if account.Status != 1 || assetSyncProvider(account.Provider) != "aliyun" || account.AccessKey == "" || account.SecretKey == "" {
		return nil, errors.New("请选择已启用且 AK/SK 完整的阿里云账号")
	}
	client := sls.CreateNormalInterfaceV2(ds.URL, sls.NewStaticCredentialsProvider(account.AccessKey, account.SecretKey, ""))
	client.SetRegion(ds.SLS.Region)
	client.SetHTTPClient(&http.Client{Timeout: 6 * time.Second, Transport: slsContextTransport{ctx: s.db.Statement.Context, base: http.DefaultTransport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	client.SetRetryTimeout(6 * time.Second)
	return client, nil
}
func (s *Service) monitorSLSStores(ds model.MonitorDatasource) ([]map[string]any, error) {
	client, err := s.monitorSLSClient(ds)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	stores := ds.SLS.Logstores
	if len(stores) == 0 {
		stores, err = client.ListLogStore(ds.SLS.Project)
	} else {
		for _, store := range stores {
			if _, err = client.GetLogStore(ds.SLS.Project, store); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, monitorSLSError("读取 Logstore", err)
	}
	result := make([]map[string]any, 0, len(stores))
	for _, store := range stores {
		if monitorSLSStoreAllowed(ds.SLS, store) {
			result = append(result, map[string]any{"name": store})
		}
	}
	return result, nil
}

// SQL uses the selected Logstore implicitly. Explicit table references could bypass its allowlist.
func validateSLSQuery(query string) error {
	quoted := byte(0)
	var plain strings.Builder
	for i := 0; i < len(query); i++ {
		c := query[i]
		if quoted != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quoted {
				quoted = 0
			}
			plain.WriteByte(' ')
			continue
		}
		if c == '\'' || c == '"' {
			quoted = c
			plain.WriteByte(' ')
		} else {
			plain.WriteByte(c)
		}
	}
	if strings.Contains(query, "|") && slsCrossStore.MatchString(plain.String()) {
		return errors.New("SLS 分析仅支持当前 Logstore 的隐式查询，请勿使用 FROM、JOIN 或 UNION 引用其他日志库")
	}
	return nil
}
func (s *Service) queryMonitorSLS(ds model.MonitorDatasource, p MonitorLogQueryPayload) (map[string]any, error) {
	if ds.Status != 1 {
		return nil, errors.New("数据源已停用")
	}
	store := strings.TrimSpace(p.Index)
	if store == "" || store == "_all" {
		store = ds.SLS.DefaultLogstore
	}
	if !monitorSLSStoreAllowed(ds.SLS, store) {
		return nil, errors.New("请选择允许访问的 Logstore")
	}
	query := firstNonEmpty(strings.TrimSpace(p.Query), "*")
	if err := validateSLSQuery(query); err != nil {
		return nil, err
	}
	client, err := s.monitorSLSClient(ds)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return querySLSLogs(client, ds.SLS.Project, store, query, p)
}
func querySLSLogs(client sls.ClientInterface, project, store, query string, p MonitorLogQueryPayload) (map[string]any, error) {
	page, size := normalizeMonitorLogPagination(p.PageNum, p.PageSize)
	if size > 100 {
		size = 100
	} // SLS GetLogs caps raw search results at 100 per request.
	if page > 50 {
		return nil, errors.New("SLS 最多浏览前 50 页，请缩小时间范围或增加检索条件")
	}
	end := p.EndAt
	if end <= 0 {
		end = time.Now().UnixMilli()
	}
	start := p.StartAt
	if start <= 0 {
		start = end - 1800000
	}
	if start >= end {
		return nil, errors.New("开始时间必须早于结束时间")
	}
	from, to := start/1000, (end+999)/1000
	began := time.Now()
	var response *sls.GetLogsResponse
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		response, err = client.GetLogs(project, store, "", from, to, query, int64(size), int64((page-1)*size), true)
		if err != nil {
			return nil, monitorSLSError("查询日志", err)
		}
		if strings.EqualFold(response.Progress, "complete") {
			break
		}
	}
	complete := strings.EqualFold(response.Progress, "complete")
	items := make([]map[string]any, 0, len(response.Logs))
	columns := map[string]bool{}
	for i, row := range response.Logs {
		if response.HasSQL {
			item := map[string]any{}
			for key, value := range row {
				item[key] = value
				columns[key] = true
			}
			items = append(items, item)
			continue
		}
		source := map[string]any{}
		for k, v := range row {
			source[k] = v
		}
		seconds, _ := strconv.ParseInt(row["__time__"], 10, 64)
		message := firstNonEmpty(row["message"], row["content"], row["log"])
		if message == "" {
			raw, _ := json.Marshal(row)
			message = string(raw)
		}
		items = append(items, map[string]any{"id": fmt.Sprintf("%d-%d", page, i), "index": store, "timestamp": time.Unix(seconds, 0).UTC().Format(time.RFC3339), "message": message, "messageRaw": message, "level": monitorSLSLogLevel(row, message), "source": source, "namespace": row["namespace"], "container": row["container"], "pod": row["pod"]})
	}
	names := make([]string, 0, len(columns))
	for key := range columns {
		names = append(names, key)
	}
	sort.Strings(names)
	buckets := []map[string]any{}
	total := int64((page-1)*size + len(items))
	warning := ""
	hasMore := !response.HasSQL && len(items) == size
	totalExact := response.HasSQL && complete
	if !response.HasSQL && !p.SkipHistogram {
		hist, err := client.GetHistograms(project, store, "", from, to, query)
		if err != nil {
			complete = false
			warning = "日志已返回，" + monitorSLSError("统计日志分布", err).Error() + "；条数仅代表已加载日志"
		} else {
			complete = complete && hist.IsComplete()
			totalExact = complete
			total = hist.Count
			hasMore = int64(page*size) < total || (!complete && len(items) == size)
			for _, b := range hist.Histograms {
				buckets = append(buckets, map[string]any{"key": b.From * 1000, "doc_count": b.Count})
			}
		}
	}
	if !complete {
		warning = firstNonEmpty(warning, "SLS 查询结果尚未完整，请缩小时间范围或刷新重试")
	}
	return map[string]any{"items": items, "total": total, "histogram": buckets, "took": time.Since(began).Milliseconds(), "isSQL": response.HasSQL, "columns": names, "complete": complete, "warning": warning, "hasMore": hasMore, "pageNum": page, "pageSize": size, "totalExact": totalExact, "histogramSkipped": p.SkipHistogram}, nil
}
func (s *Service) monitorSLSFieldValues(ds model.MonitorDatasource, index, field, query string, start, end int64, limit int) ([]map[string]any, error) {
	if strings.Contains(query, "|") {
		return nil, errors.New("SQL 分析结果不支持字段筛选，请切换到日志检索")
	}
	result, err := s.queryMonitorSLS(ds, MonitorLogQueryPayload{Index: index, Query: query, StartAt: start, EndAt: end, PageSize: 200, SkipHistogram: true})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, row := range result["items"].([]map[string]any) {
		source, _ := row["source"].(map[string]any)
		if value := monitorSLSFieldValue(source, field); value != "" {
			counts[value]++
		}
	}
	values := []map[string]any{}
	for value, hits := range counts {
		values = append(values, map[string]any{"value": value, "hits": hits, "field": field})
	}
	sort.Slice(values, func(i, j int) bool { return values[i]["hits"].(int) > values[j]["hits"].(int) })
	limit = normalizeMonitorLogFieldValueLimit(limit)
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

// Scheduled checks verify metadata access without scanning logs or every allowed store.
func (s *Service) monitorSLSHealth(ds model.MonitorDatasource) error {
	client, err := s.monitorSLSClient(ds)
	if err != nil {
		return err
	}
	defer client.Close()
	store := ds.SLS.DefaultLogstore
	if store == "" && len(ds.SLS.Logstores) > 0 {
		store = ds.SLS.Logstores[0]
	}
	if store != "" {
		_, err = client.GetLogStore(ds.SLS.Project, store)
	} else {
		_, err = client.ListLogStore(ds.SLS.Project)
	}
	if err != nil {
		return monitorSLSError("测试连接", err)
	}
	return nil
}

// Bind SDK HTTP requests to the browser request, including cancellation.
type slsContextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t slsContextTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := t.ctx
	if ctx == nil {
		ctx = request.Context()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(request.Clone(ctx))
}

func monitorSLSError(operation string, err error) error {
	code, requestID := "", ""
	var apiErr *sls.Error
	if errors.As(err, &apiErr) {
		code = apiErr.Code
		requestID = apiErr.RequestID
	}
	hint := "请检查 Endpoint、Project、Logstore 和网络，重试后仍失败可凭 Request ID 排查"
	lower := strings.ToLower(code)
	message := ""
	if apiErr != nil {
		message = strings.ToLower(apiErr.Message)
	}
	switch {
	case errors.Is(err, context.Canceled):
		hint = "查询已取消"
	case errors.Is(err, context.DeadlineExceeded):
		hint = "查询超时，请缩小时间范围或简化 SQL 后重试"
	case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "denied"):
		hint = "访问被拒绝，请检查云账号 RAM 权限及 Project / Logstore 授权范围"
	case strings.Contains(lower, "signature") || strings.Contains(lower, "accesskey") || strings.Contains(lower, "securitytoken"):
		hint = "认证失败，请检查云账号 AK/SK、凭据状态、地域及服务器时钟"
	case strings.Contains(lower, "index") || strings.Contains(message, "index") && (strings.Contains(message, "not") || strings.Contains(message, "disable")):
		hint = "索引不可用，请在 SLS 为检索字段配置索引；SQL 分析字段还需开启统计"
	case strings.Contains(lower, "notexist") || strings.Contains(lower, "notfound"):
		hint = "资源不存在，请核对地域、Project、Logstore 名称"
	case strings.Contains(lower, "query") || strings.Contains(lower, "sql") || strings.Contains(lower, "parameter"):
		hint = "查询语法或参数无效，请检查 SLS 检索语法、SQL 和字段索引配置"
	case strings.Contains(lower, "quota") || strings.Contains(lower, "busy") || strings.Contains(lower, "throttl") || strings.Contains(lower, "exceed"):
		hint = "查询限流或资源繁忙，请降低刷新频率、缩小范围后重试"
	default:
		var networkErr net.Error
		if errors.As(err, &networkErr) && networkErr.Timeout() {
			hint = "连接或查询超时，请检查网络并缩小查询范围"
		}
	}
	details := ""
	if code != "" {
		details = " [" + code + "]"
	}
	if requestID != "" {
		details += " Request ID: " + requestID
	}
	// Do not forward provider messages or request bodies which may contain secrets or log data.
	return fmt.Errorf("SLS %s失败：%s%s", operation, hint, details)
}

// Index metadata is independent of whether a query happens to return any logs.
func (s *Service) monitorSLSIndexFields(ds model.MonitorDatasource, store string) ([]map[string]any, error) {
	if ds.Status != 1 {
		return nil, errors.New("数据源已停用")
	}
	if store == "" || store == "_all" {
		store = ds.SLS.DefaultLogstore
	}
	if !monitorSLSStoreAllowed(ds.SLS, store) {
		return nil, errors.New("请选择允许访问的 Logstore")
	}
	client, err := s.monitorSLSClient(ds)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	index, err := client.GetIndex(ds.SLS.Project, store)
	if err != nil {
		return nil, monitorSLSError("读取 Logstore 索引（需要 log:GetIndex 权限）", err)
	}
	return monitorSLSIndexMetadata(index), nil
}
func monitorSLSIndexMetadata(index *sls.Index) []map[string]any {
	fields := map[string]map[string]any{}
	if index == nil {
		return []map[string]any{}
	}
	add := func(name, typ, alias string, statistics, automatic bool) {
		fields[name] = map[string]any{"name": name, "queryName": name, "type": typ, "alias": alias, "indexed": true, "statistics": statistics, "automatic": automatic, "kind": "field"}
	}
	if index.Line != nil {
		for _, name := range index.Line.AutoTextKeys {
			add(name, "text", "", true, true)
		}
	}
	for name, key := range index.Keys {
		add(name, key.Type, key.Alias, key.DocValue, false)
		for child, value := range key.JsonKeys {
			if value != nil {
				add(name+"."+child, value.Type, value.Alias, value.DocValue, false)
			}
		}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]any, 0, len(fields)+1)
	if index.Line != nil {
		result = append(result, map[string]any{"name": "*", "kind": "fulltext", "type": "text", "indexed": true, "statistics": false, "automatic": index.Line.AutoKeyDetect})
	}
	for _, name := range names {
		result = append(result, fields[name])
	}
	return result
}
func monitorSLSFieldValue(source map[string]any, field string) string {
	if value, exists := source[field]; exists {
		return monitorSourceString(value)
	}

	var value any = source
	for _, part := range strings.Split(field, ".") {
		if text, ok := value.(string); ok {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			if decoder.Decode(&value) != nil {
				return ""
			}
		}
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = object[part]
	}
	return monitorSourceString(value)
}

var slsMessageLevel = regexp.MustCompile(`(?:^|[\s\[])(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)(?:[\s\]:]|$)`)
var slsKlogLevel = regexp.MustCompile(`^\[?([IWEF])\d{4}\s`)

func monitorSLSLogLevel(row map[string]string, message string) string {
	normalize := func(value string) string {
		value = strings.ToUpper(value)
		switch value {
		case "WARNING":
			return "WARN"
		case "ERR":
			return "ERROR"
		case "CRITICAL":
			return "FATAL"
		case "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL":
			return value
		}
		return ""
	}
	for _, key := range []string{"level", "severity", "log_level", "log.level"} {
		if level := normalize(row[key]); level != "" {
			return level
		}
	}
	var body map[string]any
	if json.Unmarshal([]byte(message), &body) == nil {
		for _, key := range []string{"level", "severity", "log_level"} {
			if level := normalize(monitorSourceString(body[key])); level != "" {
				return level
			}
		}
	}
	if parts := slsKlogLevel.FindStringSubmatch(message); len(parts) > 1 {
		return map[string]string{"I": "INFO", "W": "WARN", "E": "ERROR", "F": "FATAL"}[parts[1]]
	}
	if parts := slsMessageLevel.FindStringSubmatch(message); len(parts) > 1 {
		return parts[1]
	}
	return "-"
}
