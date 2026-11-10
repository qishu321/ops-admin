package service

import (
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
	"time"

	"ops-admin/backend/model"
)

// MonitorLogBoardPayload 是日志大屏的分析请求。Query 直接透传 LogsQL，
// 因此 kerbernetes.container_name:"notice" 这类过滤条件可以原样粘贴进来。
type MonitorLogBoardPayload struct {
	DatasourceID uint   `json:"datasourceId"`
	Query        string `json:"query"`
	StartAt      int64  `json:"startAt"`
	EndAt        int64  `json:"endAt"`
	Limit        int    `json:"limit"`
}

// logBoardRecentLimit 是「最近日志」面板的条数。该面板只展示错误级别，
// 因为值班排查真正关心的是 ERROR/FATAL，INFO 会稀释信号。
const logBoardRecentLimit = 50

type logBoardRecord struct {
	at        time.Time
	level     string
	pod       string
	namespace string
	container string
	message   string
	raw       string
	fields    map[string]string
}

// 日志正文里带 Spring Boot 的彩色控制台转义（\x1b[31;1mERROR\x1b[m），
// 不去掉会连带把 ERROR 这个词切碎，级别识别和过滤都会失效。
var (
	logBoardAnsiPattern   = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
	logBoardPrefixPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?)\s+(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\s+\d+\s+---\s+\[([^\]]*)\]\s+(\S+)\s*:\s?(.*)$`)
	logBoardPairPattern   = regexp.MustCompile(`(?:^|[,\s])([A-Za-z][A-Za-z0-9_.]{1,48})=`)
	logBoardDigitsPattern = regexp.MustCompile(`\d+`)
	logBoardHexPattern    = regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`)
	logBoardUUIDPattern   = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	logBoardIPPattern     = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	logBoardSpacePattern  = regexp.MustCompile(`[ \t]{2,}`)
)

// 面板上优先展示的业务字段：客户端错误日志里的平台、版本、异常类型等。
var logBoardPreferredFields = []string{
	"exceptionType", "businessModule", "platform", "appVersion", "logType", "category",
	"deviceModel", "operatingSystem", "uri", "remoteAddr", "message",
}

func (s *Service) AnalyzeMonitorLogBoard(payload MonitorLogBoardPayload) (map[string]any, error) {
	if payload.DatasourceID == 0 {
		return nil, errors.New("请选择日志数据源")
	}
	if err := s.requireResource("datasource", payload.DatasourceID, "", "query"); err != nil {
		return nil, err
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return nil, err
	}
	if normalizeMonitorDatasourceType(ds.Type) != "victorialogs" {
		return nil, errors.New("日志大屏当前仅支持 VictoriaLogs 数据源")
	}
	endAt := payload.EndAt
	if endAt <= 0 {
		endAt = time.Now().UnixMilli()
	}
	startAt := payload.StartAt
	if startAt <= 0 || startAt >= endAt {
		startAt = endAt - time.Hour.Milliseconds()
	}
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		query = "*"
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = 5000
	}
	if limit > 20000 {
		limit = 20000
	}
	startedAt := time.Now()

	histogram, total := s.victoriaLogsHistogram(*ds, query, startAt, endAt)
	rawRows, err := s.victoriaLogsRawRows(*ds, query, startAt, endAt, limit)
	if err != nil {
		return nil, err
	}
	records := make([]logBoardRecord, 0, len(rawRows))
	for _, row := range rawRows {
		records = append(records, parseLogBoardRecord(row))
	}

	step := monitorLogHistogramStepSeconds(startAt, endAt)
	levelCounts := map[string]int{}
	podCounts := map[string]int{}
	namespaceCounts := map[string]int{}
	containerCounts := map[string]int{}
	errorCounts := map[string]int{}
	errorSamples := map[string]string{}
	errorFields := map[string]string{}
	patternCounts := map[string]int{}
	patternSamples := map[string]string{}
	fieldValues := map[string]map[string]int{}
	fieldCoverage := map[string]int{}
	bucketError := map[int64]int{}
	bucketWarn := map[int64]int{}
	bucketTotal := map[int64]int{}
	pods, namespaces := map[string]struct{}{}, map[string]struct{}{}

	for _, record := range records {
		level := strings.ToUpper(record.level)
		if level == "" {
			level = "OTHER"
		}
		levelCounts[level]++
		if record.pod != "" {
			podCounts[record.pod]++
			pods[record.pod] = struct{}{}
		}
		if record.namespace != "" {
			namespaceCounts[record.namespace]++
			namespaces[record.namespace] = struct{}{}
		}
		if record.container != "" {
			containerCounts[record.container]++
		}
		bucket := int64(0)
		if !record.at.IsZero() {
			bucket = record.at.UnixMilli() / (step * 1000) * step * 1000
			bucketTotal[bucket]++
		}
		if level == "ERROR" || level == "FATAL" {
			bucketError[bucket]++
			exceptionType := strings.TrimSpace(record.fields["exceptionType"])
			key := strings.TrimSpace(firstNonEmpty(exceptionType, firstLine(record.message)))
			if key != "" {
				errorCounts[key]++
				// 记录这个错误类型是从哪个字段来的：前端下钻时只有字段值才能
				// 用 "字段=取值" 精确过滤，纯正文只能退化成全文短语。
				if exceptionType != "" {
					errorFields[key] = "exceptionType"
				}
				if errorSamples[key] == "" {
					errorSamples[key] = firstLine(record.message)
				}
			}
		} else if level == "WARN" {
			bucketWarn[bucket]++
		}
		if pattern := logBoardPattern(record); pattern != "" {
			patternCounts[pattern]++
			if patternSamples[pattern] == "" {
				patternSamples[pattern] = firstLine(record.message)
			}
		}
		for key, value := range record.fields {
			value = strings.TrimSpace(value)
			if value == "" || len(value) > 80 {
				continue
			}
			if fieldValues[key] == nil {
				fieldValues[key] = map[string]int{}
			}
			fieldValues[key][value]++
			fieldCoverage[key]++
		}
	}

	buckets := make([]map[string]any, 0, len(histogram))
	for _, item := range histogram {
		key, _ := item["key"].(int64)
		buckets = append(buckets, map[string]any{
			"ts":      key,
			"total":   item["doc_count"],
			"error":   bucketError[key],
			"warning": bucketWarn[key],
		})
	}
	// /select/logsql/hits 偶发超时会返回空结果，此时用样本自己补一条时间线，
	// 面板宁可展示估算曲线，也不要整块空白。
	if len(buckets) == 0 && len(bucketTotal) > 0 {
		keys := make([]int64, 0, len(bucketTotal))
		for key := range bucketTotal {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, key := range keys {
			buckets = append(buckets, map[string]any{
				"ts": key, "total": bucketTotal[key],
				"error": bucketError[key], "warning": bucketWarn[key],
			})
		}
	}
	if total == 0 && len(records) > 0 {
		total = int64(len(records))
	}

	recent, recentErrorTotal := logBoardRecentEntries(records)

	errorRate := 0.0
	if len(records) > 0 {
		errorRate = float64(levelCounts["ERROR"]+levelCounts["FATAL"]) * 100 / float64(len(records))
	}
	sampled := len(records)
	return map[string]any{
		"datasource": map[string]any{"id": ds.ID, "name": ds.Name, "type": ds.Type},
		"query":      query,
		"startAt":    startAt,
		"endAt":      endAt,
		"step":       step,
		"total":      total,
		"sampled":    sampled,
		"truncated":  total > int64(limit),
		"limit":      limit,
		"tookMs":     time.Since(startedAt).Milliseconds(),
		"errorRate":  errorRate,
		"levels":     countEntries(levelCounts, 8),
		"namespaces": countEntries(namespaceCounts, 12),
		"pods":       countEntries(podCounts, 12),
		"containers": countEntries(containerCounts, 12),
		"errorTypes": logBoardErrorTypes(errorCounts, errorSamples, errorFields, 10),
		"patterns":   logBoardTopEntries(patternCounts, patternSamples, 12, "pattern", "sample"),
		"facets":     logBoardFacets(fieldValues, fieldCoverage, sampled, 8),
		"buckets":    buckets,
		"recent":     recent,
		// recent 只含 ERROR/FATAL；Sample 内共多少条错误由 recentTotal 告知，
		// 前端据此说明「共 N 条，展示最新 50 条」。
		"recentLimit": logBoardRecentLimit,
		"recentTotal": recentErrorTotal,
		"recentLevel": "ERROR",
		"podCount":    len(pods),
		"nsCount":     len(namespaces),
	}, nil
}

// logBoardRecentEntries 挑出「最近日志」面板要展示的记录。面板只收
// ERROR / FATAL：值班排查真正要动作的是错误，混着 INFO 会把列表冲掉。
// records 已按 _time 倒序，所以顺序取满 logBoardRecentLimit 条即「最新 50 条错误日志」。
// 第二个返回值是取样窗口内的错误总数，前端用它说明「共 N 条，仅展示前 50 条」。
func logBoardRecentEntries(records []logBoardRecord) ([]map[string]any, int) {
	recent := make([]map[string]any, 0, logBoardRecentLimit)
	errorTotal := 0
	for _, record := range records {
		level := strings.ToUpper(record.level)
		if level != "ERROR" && level != "FATAL" {
			continue
		}
		errorTotal++
		if len(recent) >= logBoardRecentLimit {
			continue
		}
		summary := firstLine(record.message)
		if exceptionType := record.fields["exceptionType"]; exceptionType != "" {
			if detail := firstLine(record.fields["message"]); detail != "" {
				summary = exceptionType + " · " + detail
			} else {
				summary = exceptionType + " · " + summary
			}
		}
		recent = append(recent, map[string]any{
			"time":          record.at.Local().Format("2006-01-02 15:04:05.000"),
			"summary":       summary,
			"level":         level,
			"namespace":     record.namespace,
			"pod":           record.pod,
			"container":     record.container,
			"message":       record.message,
			"exceptionType": record.fields["exceptionType"],
			"platform":      record.fields["platform"],
			"appVersion":    record.fields["appVersion"],
			"deviceModel":   record.fields["deviceModel"],
		})
	}
	return recent, errorTotal
}

// victoriaLogsRawRows 直接取 VictoriaLogs 的原始记录，保留 _msg / _time 和全部
// kubernetes.* 字段，供日志大屏在本地做解析与聚合。
func (s *Service) victoriaLogsRawRows(ds model.MonitorDatasource, query string, startAt, endAt int64, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 5000
	}
	fullQuery := query
	// 用户写的是过滤条件（最常见的用法）时补排序取最新；已经带聚合管道就原样执行。
	if !strings.Contains(query, "|") {
		fullQuery = fmt.Sprintf("%s | sort by (_time desc) | limit %d", query, limit)
	}
	form := url.Values{}
	form.Set("query", fullQuery)
	form.Set("start", time.UnixMilli(startAt).UTC().Format(time.RFC3339Nano))
	form.Set("end", time.UnixMilli(endAt).UTC().Format(time.RFC3339Nano))
	form.Set("limit", strconv.Itoa(limit))
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/select/logsql/query", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyMonitorDatasourceAuth(request, ds)
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaLogs 查询失败，状态码 %d: %s", response.StatusCode, truncateForMessage(string(body)))
	}
	rows := make([]map[string]any, 0, 256)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var source map[string]any
		if err := json.Unmarshal([]byte(line), &source); err != nil {
			continue
		}
		rows = append(rows, source)
	}
	return rows, nil
}

func parseLogBoardRecord(source map[string]any) logBoardRecord {
	record := logBoardRecord{fields: map[string]string{}}
	rawMessage := monitorLogFieldValue(source, "_msg")
	record.raw = rawMessage
	clean := strings.TrimSpace(logBoardAnsiPattern.ReplaceAllString(rawMessage, ""))
	record.at = parseLogBoardTime(firstNonEmpty(
		monitorLogFieldValue(source, "_time"),
		monitorLogFieldValue(source, "timestamp_end"),
		monitorLogFieldValue(source, "@timestamp"),
	))
	record.namespace = firstNonEmpty(
		monitorLogFieldValue(source, "kubernetes.pod_namespace"),
		monitorLogFieldValue(source, "namespace"),
	)
	record.pod = firstNonEmpty(
		monitorLogFieldValue(source, "kubernetes.pod_name"),
		monitorLogFieldValue(source, "pod"),
	)
	record.container = firstNonEmpty(
		monitorLogFieldValue(source, "kubernetes.container_name"),
		monitorLogFieldValue(source, "container"),
	)

	content := clean
	if match := logBoardPrefixPattern.FindStringSubmatch(clean); match != nil {
		record.level = strings.ToUpper(match[2])
		record.fields["logger"] = strings.TrimSpace(match[4])
		record.fields["thread"] = strings.TrimSpace(match[3])
		content = strings.TrimSpace(match[5])
	}
	if record.level == "" {
		record.level = detectLogBoardLevel(clean)
	}
	if strings.HasPrefix(content, "{") {
		var payload map[string]any
		if json.Unmarshal([]byte(content), &payload) == nil {
			for key, value := range payload {
				switch typed := value.(type) {
				case string:
					record.fields[key] = typed
				case float64:
					record.fields[key] = strconv.FormatFloat(typed, 'f', -1, 64)
				}
			}
		}
	}
	for key, value := range parseLogBoardPairs(content) {
		if _, exists := record.fields[key]; !exists {
			record.fields[key] = value
		}
	}
	// firstBusinessStackLine 是「类路径.方法名 ()」，面板上真正想知道的是哪个业务类炸了，
	// 把方法名去掉后基数会落到可统计的范围。
	if stackLine := strings.TrimSpace(record.fields["firstBusinessStackLine"]); stackLine != "" {
		if field := strings.Fields(stackLine); len(field) > 0 {
			qualified := field[0]
			if index := strings.LastIndex(qualified, "."); index > 0 {
				record.fields["businessModule"] = qualified[:index]
			}
		}
	}
	record.message = logBoardSpacePattern.ReplaceAllString(content, " ")
	if len(record.message) > 600 {
		record.message = record.message[:600] + "…"
	}
	return record
}

// parseLogBoardPairs 解析 `key=value, key=value` 形式的正文。值里可能带逗号
// （userAgent 里就有），所以按「下一个 key= 的起点」切，而不是简单按逗号切。
func parseLogBoardPairs(content string) map[string]string {
	matches := logBoardPairPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make(map[string]string, len(matches))
	for index, match := range matches {
		key := content[match[2]:match[3]]
		valueStart := match[1]
		valueEnd := len(content)
		if index+1 < len(matches) {
			valueEnd = matches[index+1][0]
		}
		value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(content[valueStart:valueEnd]), ","))
		if value == "" {
			continue
		}
		if len(value) > 400 {
			value = value[:400] + "…"
		}
		result[key] = value
	}
	return result
}

func parseLogBoardTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05.000"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func detectLogBoardLevel(content string) string {
	head := content
	if len(head) > 160 {
		head = head[:160]
	}
	upper := strings.ToUpper(head)
	for _, level := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE"} {
		if strings.Contains(upper, level) {
			return level
		}
	}
	return ""
}

// logBoardPattern 归并同类日志：业务字段齐全时用「异常类型 + 异常正文」做模板，
// 否则退回「首行去掉数字/ID/IP」。单纯的整行归一化会把所有客户端上报并成一条，
// 面板上就看不出是哪类错误在涨。
func logBoardPattern(record logBoardRecord) string {
	line := ""
	if exceptionType := strings.TrimSpace(record.fields["exceptionType"]); exceptionType != "" {
		line = exceptionType
		if message := strings.TrimSpace(record.fields["message"]); message != "" {
			line += " · " + firstLine(message)
		}
	}
	if line == "" {
		line = firstLine(record.message)
	}
	if line == "" {
		return ""
	}
	line = logBoardUUIDPattern.ReplaceAllString(line, "<uuid>")
	line = logBoardIPPattern.ReplaceAllString(line, "<ip>")
	line = logBoardHexPattern.ReplaceAllString(line, "<id>")
	line = logBoardDigitsPattern.ReplaceAllString(line, "<n>")
	line = logBoardSpacePattern.ReplaceAllString(strings.TrimSpace(line), " ")
	if len(line) > 150 {
		line = line[:150]
	}
	return line
}

func firstLine(value string) string {
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		value = value[:index]
	}
	value = strings.TrimSpace(value)
	if len(value) > 220 {
		value = value[:220]
	}
	return value
}

func truncateForMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 300 {
		return value[:300] + "…"
	}
	return value
}

func countEntries(counts map[string]int, limit int) []map[string]any {
	type entry struct {
		key   string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, entry{key: key, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].key < entries[j].key
		}
		return entries[i].count > entries[j].count
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	result := make([]map[string]any, 0, len(entries))
	for _, item := range entries {
		result = append(result, map[string]any{"value": item.key, "count": item.count})
	}
	return result
}

func logBoardTopEntries(counts map[string]int, samples map[string]string, limit int, keyName, sampleName string) []map[string]any {
	entries := countEntries(counts, limit)
	for _, item := range entries {
		key, _ := item["value"].(string)
		item[keyName] = key
		item[sampleName] = samples[key]
	}
	return entries
}

func logBoardErrorTypes(counts map[string]int, samples, fields map[string]string, limit int) []map[string]any {
	entries := countEntries(counts, limit)
	for _, item := range entries {
		name, _ := item["value"].(string)
		item["name"] = name
		item["sample"] = samples[name]
		item["field"] = fields[name]
	}
	return entries
}

// logBoardFacets 动态挑出适合做面板的字段：优先业务字段，其次按覆盖度，
// 只保留基数适中（2~40）的字段，避免把 fingerprint 这类高基数字段塞满面板。
func logBoardFacets(values map[string]map[string]int, coverage map[string]int, sampled, limit int) []map[string]any {
	type candidate struct {
		key      string
		priority int
		card     int
		hit      int
	}
	candidates := make([]candidate, 0, len(values))
	for key, buckets := range values {
		if len(buckets) < 2 || len(buckets) > 40 {
			continue
		}
		priority := len(logBoardPreferredFields) + 1
		for index, name := range logBoardPreferredFields {
			if name == key {
				priority = index
				break
			}
		}
		// 栈信息、设备唯一号这类字段做分布没有意义，直接跳过。
		if key == "stackTrace" || key == "message" || strings.Contains(key, "UniqueIdentifier") {
			continue
		}
		candidates = append(candidates, candidate{key: key, priority: priority, card: len(buckets), hit: coverage[key]})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].hit != candidates[j].hit {
			return candidates[i].hit > candidates[j].hit
		}
		return candidates[i].key < candidates[j].key
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	result := make([]map[string]any, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, map[string]any{
			"key":      item.key,
			"values":   countEntries(values[item.key], 8),
			"coverage": item.hit,
			"sampled":  sampled,
		})
	}
	return result
}
