package service

import (
	"strings"
	"testing"
	"time"
)

func TestLogBoardKeywordRegexEscapesMetaCharacters(t *testing.T) {
	cases := map[string]string{
		"请求礼包":                  `(?i)请求礼包`,
		"[ConfirmPanelContext]": `(?i)\\[ConfirmPanelContext\\]`,
		"v1.0":                  `(?i)v1\\.0`,
		"a+b":                   `(?i)a\\+b`,
		"心跳 heartbeat":          `(?i)心跳 heartbeat`,
	}
	for input, want := range cases {
		if got := logBoardKeywordRegex(input); got != want {
			t.Fatalf("keyword %q escaped to %q, want %q", input, got, want)
		}
	}
}

func TestBuildLogBoardKeywordQueryUsesAnd(t *testing.T) {
	got := buildLogBoardKeywordQuery(`kubernetes.container_name:"notice"`, []string{"请求礼包", "购买"})
	want := `(kubernetes.container_name:"notice") AND _msg:~"(?i)请求礼包" AND _msg:~"(?i)购买"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestBuildLogBoardKeywordQueryDropsWildcardBase(t *testing.T) {
	got := buildLogBoardKeywordQuery("*", []string{"error"})
	want := `_msg:~"(?i)error"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestNormalizeLogBoardKeywords(t *testing.T) {
	got := normalizeLogBoardKeywords([]string{"  请求礼包  购买 ", `"心跳"`, "购买"})
	want := []string{"请求礼包", "购买", "心跳"}
	if len(got) != len(want) {
		t.Fatalf("keywords = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keywords = %v, want %v", got, want)
		}
	}
}

func TestNormalizeLogBoardKeywordsCapsCountAndLength(t *testing.T) {
	input := []string{"a b c d e f g h"}
	got := normalizeLogBoardKeywords(input)
	if len(got) != logBoardRecentSearchMaxKeyword {
		t.Fatalf("expected at most %d keywords, got %d (%v)", logBoardRecentSearchMaxKeyword, len(got), got)
	}
	long := strings.Repeat("错", logBoardRecentKeywordMaxLen+20)
	trimmed := normalizeLogBoardKeywords([]string{long})
	if len([]rune(trimmed[0])) != logBoardRecentKeywordMaxLen {
		t.Fatalf("keyword should be truncated to %d runes, got %d", logBoardRecentKeywordMaxLen, len([]rune(trimmed[0])))
	}
}

func TestNormalizeLogBoardKeywordsIgnoresEmptyInput(t *testing.T) {
	if got := normalizeLogBoardKeywords([]string{"   ", "", `""`}); len(got) != 0 {
		t.Fatalf("blank keywords should be dropped, got %v", got)
	}
}

func TestLogBoardRecentSearchWindowIsHalfDay(t *testing.T) {
	if logBoardRecentSearchWindow != 12*time.Hour {
		t.Fatalf("search upper bound must stay at 12h, got %s", logBoardRecentSearchWindow)
	}
}

// 窗口跟随页面时间范围：小范围原样用，大范围收口到 12 小时。
func TestLogBoardRecentWindowFollowsRange(t *testing.T) {
	cases := []struct {
		rangeSeconds int
		want         time.Duration
	}{
		{0, 12 * time.Hour},                // 没传：按上限
		{-60, 12 * time.Hour},              // 脏数据：按上限
		{900, 15 * time.Minute},            // 15 分钟
		{3600, time.Hour},                  // 1 小时
		{21600, 6 * time.Hour},             // 6 小时
		{43200, 12 * time.Hour},            // 正好 12 小时
		{86400, 12 * time.Hour},            // 24 小时 → 12 小时
		{604800, 12 * time.Hour},           // 7 天 → 12 小时
		{5, logBoardRecentSearchMinWindow}, // 过小：保底 1 分钟
	}
	for _, item := range cases {
		if got := logBoardRecentWindow(item.rangeSeconds); got != item.want {
			t.Fatalf("range %ds should search %s, got %s", item.rangeSeconds, item.want, got)
		}
	}
}
