package service

import (
	"strings"
	"testing"
	"time"
)

func recentTestRecord(level, message string) logBoardRecord {
	return logBoardRecord{
		at:      time.Date(2026, 11, 10, 15, 0, 0, 0, time.Local),
		level:   level,
		message: message,
		fields:  map[string]string{},
	}
}

func TestLogBoardRecentKeepsOnlyErrors(t *testing.T) {
	records := []logBoardRecord{
		recentTestRecord("ERROR", "第一个错误"),
		recentTestRecord("INFO", "正常日志"),
		recentTestRecord("WARN", "告警日志"),
		recentTestRecord("FATAL", "致命错误"),
		recentTestRecord("ERROR", "第三个错误"),
	}
	recent, total := logBoardRecentEntries(records)
	if len(recent) != 3 || total != 3 {
		t.Fatalf("expected 3 error rows out of 5 records, got %d rows / total %d", len(recent), total)
	}
	for _, item := range recent {
		level, _ := item["level"].(string)
		if level != "ERROR" && level != "FATAL" {
			t.Fatalf("recent panel must not contain %q", level)
		}
	}
	// 顺序必须保持调用方给的倒序，不能因为过滤而重排。
	if summary, _ := recent[0]["summary"].(string); summary != "第一个错误" {
		t.Fatalf("recent order changed, first summary = %q", summary)
	}
}

func TestLogBoardRecentCapsAtFiftyAndCountsAllErrors(t *testing.T) {
	records := make([]logBoardRecord, 0, 70)
	for i := 0; i < 70; i++ {
		level := "ERROR"
		if i%7 == 0 {
			level = "WARN"
		}
		records = append(records, recentTestRecord(level, "boom"))
	}
	recent, total := logBoardRecentEntries(records)
	if len(recent) != logBoardRecentLimit {
		t.Fatalf("expected %d rows, got %d", logBoardRecentLimit, len(recent))
	}
	if logBoardRecentLimit != 50 {
		t.Fatalf("recent panel should show the latest 50 errors, got limit %d", logBoardRecentLimit)
	}
	// 70 条里每 7 条一个 WARN，共 10 条非错误。
	if total != 60 {
		t.Fatalf("error total should count every error in the sample, got %d", total)
	}
}

func TestLogBoardRecentSummaryPrefersExceptionType(t *testing.T) {
	record := recentTestRecord("ERROR", "原始首行")
	record.fields["exceptionType"] = "System.NullReferenceException"
	record.fields["message"] = "对象引用未设置为对象的实例"
	recent, _ := logBoardRecentEntries([]logBoardRecord{record})
	summary, _ := recent[0]["summary"].(string)
	if !strings.HasPrefix(summary, "System.NullReferenceException · ") {
		t.Fatalf("summary should lead with the exception type, got %q", summary)
	}
}

func TestLogBoardRecentHandlesEmptySample(t *testing.T) {
	recent, total := logBoardRecentEntries(nil)
	if len(recent) != 0 || total != 0 {
		t.Fatalf("empty sample should produce an empty list, got %d / %d", len(recent), total)
	}
}
