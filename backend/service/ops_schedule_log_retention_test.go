package service

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"ops-admin/backend/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestHTTPProbeLogRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if got := httpProbeLogRetentionCutoff(now, 7); !got.Equal(now.Add(-168 * time.Hour)) {
		t.Fatalf("seven-day cutoff = %s", got)
	}
	if got := httpProbeLogRetentionCutoff(now, 0); !got.Equal(now.Add(-168 * time.Hour)) {
		t.Fatalf("default cutoff = %s", got)
	}
}

func TestHTTPProbeLogDeletionStaysScopedToExpiredProbes(t *testing.T) {
	sqlDB, err := sql.Open("mysql", "ops:ops@tcp(127.0.0.1:1)/ops")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	statement := expiredHTTPProbeLogQuery(db.Where("id IN ?", []uint{1, 2}), cutoff).
		Delete(&model.OpsScheduleTaskLog{}).Statement
	query := statement.SQL.String()
	if statement.Error != nil {
		t.Fatalf("build delete query: %v", statement.Error)
	}
	for _, required := range []string{"id IN", "task_type", "created_at <"} {
		if !strings.Contains(query, required) {
			t.Fatalf("delete query lacks %q: %s", required, query)
		}
	}
	for _, expected := range []any{"http", cutoff} {
		found := false
		for _, value := range statement.Vars {
			if value == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("delete query lacks argument %#v: %v", expected, statement.Vars)
		}
	}
}

func TestHTTPProbeLogRetentionRejectsInvalidDays(t *testing.T) {
	svc := &Service{}
	for _, days := range []int{0, -1, 366} {
		if _, err := svc.SaveHTTPProbeLogRetention(days); err == nil {
			t.Fatalf("retention %d days should fail", days)
		}
	}
}
