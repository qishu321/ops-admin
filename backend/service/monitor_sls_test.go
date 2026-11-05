package service

import (
	"context"
	"errors"
	sls "github.com/aliyun/aliyun-log-go-sdk"
	"net/http"
	"ops-admin/backend/model"
	"strings"
	"testing"
)

type slsQueryStub struct {
	sls.ClientInterface
	calls      int
	sql        bool
	incomplete bool
	histCalls  int
	histFail   bool
}

func (c *slsQueryStub) GetLogs(project, store, topic string, from, to int64, q string, lines, offset int64, reverse bool) (*sls.GetLogsResponse, error) {
	if from != 100 || to != 201 || offset != 0 || !reverse {
		return nil, errors.New("incorrect time/pagination mapping")
	}
	if lines > 100 {
		return nil, errors.New("SLS page too large")
	}
	c.calls++
	progress := "Complete"
	if c.incomplete || c.calls == 1 {
		progress = "Incomplete"
	}
	logs := []map[string]string{{"__time__": "150", "content": "ERROR test", "level": "ERROR"}}
	if c.sql {
		logs = []map[string]string{{"count": "42"}}
	}
	return &sls.GetLogsResponse{Progress: progress, HasSQL: c.sql, Logs: logs}, nil
}
func (c *slsQueryStub) GetHistograms(project, store, topic string, from, to int64, q string) (*sls.GetHistogramsResponse, error) {
	c.histCalls++
	if c.histFail {
		return nil, errors.New("denied")
	}
	return &sls.GetHistogramsResponse{Progress: "Complete", Count: 5, Histograms: []sls.SingleHistogram{{From: 100, To: 200, Count: 5}}}, nil
}
func TestSLSQueryMapping(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		sql, incomplete, histFail bool
	}{{"logs", false, false, false}, {"sql", true, false, false}, {"partial", false, true, false}, {"histogram denied", false, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			c := &slsQueryStub{sql: tc.sql, incomplete: tc.incomplete, histFail: tc.histFail}
			r, err := querySLSLogs(c, "project", "store", "*", MonitorLogQueryPayload{StartAt: 100001, EndAt: 200001, PageSize: 200})
			if err != nil {
				t.Fatal(err)
			}
			if c.calls != 2 {
				t.Fatalf("retry calls=%d", c.calls)
			}
			if r["isSQL"] != tc.sql {
				t.Fatal("wrong result mode")
			}
			if tc.incomplete && r["complete"] != false {
				t.Fatal("partial result marked complete")
			}
			if tc.histFail && r["warning"] == "" {
				t.Fatal("hidden histogram failure")
			}
			if tc.sql && len(r["columns"].([]string)) != 1 {
				t.Fatal("SQL columns missing")
			}
		})
	}
}
func TestSLSConfigBoundary(t *testing.T) {
	cfg := model.MonitorSLSConfig{Region: "cn-hangzhou", Project: "demo-project", CloudAccountID: 1, DefaultLogstore: "app", Logstores: []string{"app"}}
	if err := validateMonitorSLS("https://cn-hangzhou.log.aliyuncs.com", cfg); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"https://evil.example", "https://demo.cn-hangzhou.log.aliyuncs.com", "http://cn-hangzhou.log.aliyuncs.com", "https://cn-shanghai.log.aliyuncs.com"} {
		if validateMonitorSLS(endpoint, cfg) == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	if monitorSLSStoreAllowed(cfg, "other") {
		t.Fatal("allowlist bypass")
	}
	for _, q := range []string{"* | select * from other", "* | select * FROM `other`", "* | select * join other"} {
		if validateSLSQuery(q) == nil {
			t.Fatalf("accepted cross-store SQL: %s", q)
		}
	}
	if err := validateSLSQuery(`* | select count(*) as count`); err != nil {
		t.Fatal(err)
	}
	if normalizeLogShortcutDatasourceType("sls") != "sls" {
		t.Fatal("shortcut type lost")
	}
}

func TestSLSSkipHistogram(t *testing.T) {
	c := &slsQueryStub{}
	r, err := querySLSLogs(c, "project", "app", "*", MonitorLogQueryPayload{StartAt: 100001, EndAt: 200001, PageSize: 200, SkipHistogram: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.histCalls != 0 || r["histogramSkipped"] != true || r["totalExact"] != false {
		t.Fatalf("unexpected stats: %v calls %d", r, c.histCalls)
	}
}
func TestSLSErrorHintsDoNotExposeProviderMessages(t *testing.T) {
	for _, tc := range []struct{ code, message, want string }{
		{"Unauthorized", "secret-provider-message", "RAM"},
		{"SignatureNotMatch", "secret-provider-message", "AK/SK"},
		{"InvalidQuery", "index does not exist secret-provider-message", "索引"},
		{"InvalidQuery", "syntax secret-provider-message", "语法"},
		{"IndexNotExist", "secret-provider-message", "索引"},
		{"ProjectNotExist", "secret-provider-message", "资源不存在"},
		{"QuotaExceed", "secret-provider-message", "限流"},
	} {
		err := monitorSLSError("查询", &sls.Error{Code: tc.code, Message: tc.message, RequestID: "test-request"})
		if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "test-request") || strings.Contains(err.Error(), "secret-provider-message") {
			t.Fatalf("unsafe/incorrect error: %v", err)
		}
	}
}

type slsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f slsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func TestSLSRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := slsContextTransport{ctx: ctx, base: slsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	request, _ := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	_, err := transport.RoundTrip(request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}

func TestSLSIndexMetadataIncludesUnseenFieldsAndJSONChildren(t *testing.T) {
	index := &sls.Index{Line: &sls.IndexLine{AutoKeyDetect: true, AutoTextKeys: []string{"auto_field", "level"}}, Keys: map[string]sls.IndexKey{
		"level":      {Type: "text", DocValue: false},
		"actionType": {Type: "long", DocValue: true},
		"details":    {Type: "json", JsonKeys: map[string]*sls.JsonKey{"code": {Type: "long", DocValue: true}, "empty": nil}},
	}}
	fields := monitorSLSIndexMetadata(index)
	byName := map[string]map[string]any{}
	for _, field := range fields {
		byName[field["name"].(string)] = field
	}
	if byName["*"]["kind"] != "fulltext" || byName["actionType"]["type"] != "long" || byName["details.code"]["statistics"] != true || byName["auto_field"]["automatic"] != true {
		t.Fatalf("metadata lost: %v", fields)
	}
	if byName["level"]["statistics"] != false {
		t.Fatal("explicit index must override autodetection")
	}
	if len(monitorSLSIndexMetadata(nil)) != 0 {
		t.Fatal("nil index")
	}
	if got := monitorSLSFieldValue(map[string]any{"details": `{"code":9007199254740993}`}, "details.code"); got != "9007199254740993" {
		t.Fatalf("lost long precision: %s", got)
	}
}

func TestSLSLevelDoesNotGuessTraceFromTraceID(t *testing.T) {
	for _, tc := range []struct{ message, explicit, want string }{
		{`I1103 10:28:10.232579 1 file.go:974] "traceID"="abc"`, "", "INFO"},
		{`request traceID=abc completed`, "", "-"},
		{`2026-11-03 10:28:10 INFO service started`, "", "INFO"},
		{`{"level":"warn","traceID":"abc"}`, "", "WARN"},
		{`traceID=abc`, "ERROR", "ERROR"},
	} {
		if got := monitorSLSLogLevel(map[string]string{"level": tc.explicit}, tc.message); got != tc.want {
			t.Fatalf("%s: %s != %s", tc.message, got, tc.want)
		}
	}
	if value := monitorSLSFieldValue(map[string]any{"_container_name_": "mgr"}, "__tag__:_container_name_"); value != "" {
		t.Fatalf("tag must not alias ordinary field: %s", value)
	}
}
