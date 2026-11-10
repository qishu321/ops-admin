package service

import (
	"strings"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func TestLogBoardShareTokenRejectsUnexpectedFormats(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef"
	if !logBoardShareTokenPattern.MatchString(valid) {
		t.Fatal("generated token format must be accepted")
	}
	for _, value := range []string{"", "short", "../../etc/passwd", "0123456789ABCDEF0123456789ABCDEF", "0123456789abcdef0123456789abcdefg"} {
		if logBoardShareTokenPattern.MatchString(value) {
			t.Fatalf("unexpected token accepted: %q", value)
		}
	}
}

func TestLogBoardShareViewReportsUnavailableReason(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	disabled := logBoardShareView(model.LogBoardShare{Status: 0}, "ds", now)
	if disabled.Active || disabled.Unavailable != "已停用" {
		t.Fatalf("disabled share view = %+v", disabled)
	}

	expired := logBoardShareView(model.LogBoardShare{Status: 1, ExpiresAt: &past}, "ds", now)
	if expired.Active || expired.Unavailable != "已过期" {
		t.Fatalf("expired share view = %+v", expired)
	}

	active := logBoardShareView(model.LogBoardShare{Status: 1, ExpiresAt: &future}, "ds", now)
	if !active.Active || active.Unavailable != "" {
		t.Fatalf("active share view = %+v", active)
	}
}

func TestLogBoardShareCacheHonoursTTL(t *testing.T) {
	store := &logBoardShareCacheStore{entries: map[string]logBoardShareCacheEntry{}}
	store.set("k", map[string]any{"total": 1}, 20*time.Millisecond)
	if _, ok := store.get("k"); !ok {
		t.Fatal("fresh cache entry must be reusable")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := store.get("k"); ok {
		t.Fatal("expired cache entry must not be served")
	}
}

func TestLogBoardShareCacheTTLIsBounded(t *testing.T) {
	if got := logBoardShareCacheTTL(600); got != 8*time.Second {
		t.Fatalf("slow refresh should cap at 8s, got %s", got)
	}
	if got := logBoardShareCacheTTL(6); got != 6*time.Second {
		t.Fatalf("fast refresh should follow the share setting, got %s", got)
	}
	if got := logBoardShareCacheTTL(1); got != 3*time.Second {
		t.Fatalf("very fast refresh must still be floored at 3s, got %s", got)
	}
}

func TestLogBoardShareCacheKeySeparatesViewerQueries(t *testing.T) {
	base := logBoardShareCacheKey("tok", 1, `kubernetes.container_name:"notice"`, 86400)
	if same := logBoardShareCacheKey("tok", 2, `kubernetes.container_name:"notice"`, 86400); same == base {
		t.Fatal("different datasource must not share a cache entry")
	}
	if same := logBoardShareCacheKey("tok", 1, "ERROR", 86400); same == base {
		t.Fatal("different LogsQL must not share a cache entry")
	}
	if same := logBoardShareCacheKey("tok", 1, `kubernetes.container_name:"notice"`, 3600); same == base {
		t.Fatal("different time range must not share a cache entry")
	}
	if !strings.HasPrefix(base, "tok|1|86400|") {
		t.Fatalf("cache key layout changed: %s", base)
	}
}

func TestLogBoardShareRateLimiterCapsBurst(t *testing.T) {
	limiter := &logBoardShareRateLimiterStore{hits: map[string][]time.Time{}}
	key := logBoardShareRateKey("token", "10.0.0.1")
	for index := 0; index < logBoardShareRateBurst; index++ {
		if !limiter.allow(key) {
			t.Fatalf("request %d should still be allowed", index)
		}
	}
	if limiter.allow(key) {
		t.Fatal("limiter must reject requests above the burst window")
	}
	if !limiter.allow(logBoardShareRateKey("token", "10.0.0.2")) {
		t.Fatal("another viewer must not be blocked by someone else's traffic")
	}
}

func TestStripLogBoardShareDetailRemovesRawBodies(t *testing.T) {
	analysis := map[string]any{
		"recent": []map[string]any{
			{"summary": "NullReferenceException", "message": "at Game.Battle.Start() userId=123"},
		},
		"pods":       []map[string]any{{"value": "notice-0"}},
		"containers": []map[string]any{{"value": "notice"}},
		"total":      int64(9),
	}
	stripLogBoardShareDetail(analysis)
	recent := analysis["recent"].([]map[string]any)
	if _, ok := recent[0]["message"]; ok {
		t.Fatal("raw log body must be removed when detail is not allowed")
	}
	if recent[0]["summary"] != "NullReferenceException" {
		t.Fatal("summary must stay so the board is still readable")
	}
	if _, ok := analysis["pods"]; ok {
		t.Fatal("pod inventory should be hidden from public summary view")
	}
	if analysis["total"] != int64(9) {
		t.Fatal("aggregated counters must be preserved")
	}
}

func TestLogBoardShareStatusNormalization(t *testing.T) {
	if normalizeLogBoardShareStatus(0) != 0 || normalizeLogBoardShareStatus(5) != 1 {
		t.Fatal("status must collapse to enabled or disabled")
	}
}
