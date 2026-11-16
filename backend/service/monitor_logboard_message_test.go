package service

import (
	"strings"
	"testing"
)

// 详情弹窗要看到完整堆栈：客户端错误日志实测中位数 3.2KB，
// 之前统一截到 600 字符，导致弹窗里尾部 stackTrace 全丢。
func TestLogBoardRecordKeepsFullMessage(t *testing.T) {
	body := "客户端错误日志上报: uri=/api/v1/client-logs/errors, message=" +
		strings.Repeat("NullReferenceException; ", 260) + "END_OF_STACK"
	source := map[string]any{
		"_time": "2026-11-10 15:55:27.851",
		"_msg":  "2026-11-10 15:55:27.851 ERROR 1 --- [nio-4434-exec-1] CONSOLE_ONLY : " + body,
	}
	record := parseLogBoardRecord(source)
	if !strings.Contains(record.message, "END_OF_STACK") {
		t.Fatalf("full body should be kept, got %d chars", len(record.message))
	}
	if len(record.message) <= 600 {
		t.Fatalf("body over 600 chars must not be clipped, got %d", len(record.message))
	}
	if record.messageTruncated {
		t.Fatal("body under the 20K cap should not be flagged as truncated")
	}
	// 面板摘要仍是单行，不受完整正文影响。
	if summary := firstLine(record.message); strings.ContainsAny(summary, "\n") || summary == "" {
		t.Fatalf("firstLine summary broken: %q", summary)
	}
}

func TestLogBoardRecordCapsHugeMessage(t *testing.T) {
	source := map[string]any{
		"_time": "2026-11-10 15:55:27.851",
		"_msg":  "2026-11-10 15:55:27.851 ERROR 1 --- [nio-1-exec-1] CONSOLE_ONLY : " + strings.Repeat("x", logBoardMessageMaxLen+500),
	}
	record := parseLogBoardRecord(source)
	if !record.messageTruncated {
		t.Fatal("huge body must be flagged as truncated")
	}
	if got := len([]rune(record.message)); got > logBoardMessageMaxLen+1 {
		t.Fatalf("capped body too long: %d runes", got)
	}
}

func TestLogBoardRecentCarriesTruncationFlag(t *testing.T) {
	record := recentTestRecord("ERROR", "正文")
	record.messageTruncated = true
	recent, _ := logBoardRecentEntries([]logBoardRecord{record})
	if len(recent) != 1 {
		t.Fatalf("expected one row, got %d", len(recent))
	}
	if flag, _ := recent[0]["messageTruncated"].(bool); !flag {
		t.Fatal("recent rows must expose messageTruncated so the dialog can warn")
	}
}
