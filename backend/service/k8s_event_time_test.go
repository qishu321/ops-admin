package service

import "testing"

func TestFormatTimestampUsesBeijingTime(t *testing.T) {
	got := formatTimestamp("2026-09-15T07:04:00Z")
	const want = "2026-09-15 15:04"
	if got != want {
		t.Fatalf("formatTimestamp() = %q, want %q", got, want)
	}
}

func TestFirstNonEmptyEventTime(t *testing.T) {
	if got := firstNonEmptyEventTime("", "  ", "2026-09-15T07:04:00Z"); got != "2026-09-15T07:04:00Z" {
		t.Fatalf("firstNonEmptyEventTime() = %q", got)
	}
}
