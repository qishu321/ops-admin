package service

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestRefreshSessionPreservesTransientDatabaseErrors(t *testing.T) {
	for _, stage := range []string{"session", "admin", "update"} {
		t.Run(stage, func(t *testing.T) {
			unavailable := errors.New("temporary database outage")
			state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
				if strings.Contains(q, "sys_auth_session") && stage != "session" {
					return []string{"id", "admin_id", "last_activity_at", "expires_at"}, [][]driver.Value{{"test-session", int64(1), time.Now(), time.Now().Add(24 * time.Hour)}}, nil
				}
				if stage == "update" && strings.Contains(q, "sys_admin") {
					return []string{"id", "username", "status"}, [][]driver.Value{{int64(1), "test", int64(1)}}, nil
				}
				return nil, nil, unavailable
			}}
			db := systemDB(t, state)
			if stage == "update" {
				if err := db.Callback().Update().Before("gorm:update").Register("test:outage", func(tx *gorm.DB) { tx.AddError(unavailable) }); err != nil {
					t.Fatal(err)
				}
			}
			svc := &Service{db: db}
			_, _, err := svc.RefreshSession("test-refresh", time.Now())
			if !errors.Is(err, unavailable) {
				t.Fatalf("transient %s failure was turned into an authentication rejection: %v", stage, err)
			}
			if errors.Is(err, ErrRefreshSessionRejected) {
				t.Fatal("outage classified as an expired/revoked session")
			}
		})
	}
}

func TestRefreshSessionRenewsActiveButRejectsInvalidSessions(t *testing.T) {
	for _, mode := range []string{"active", "unknown", "expired", "revoked", "idle", "disabled", "missing-account"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now()
			expires := now.Add(6 * 24 * time.Hour)
			last := now
			var revoked driver.Value
			if mode == "expired" {
				expires = now.Add(-time.Second)
			}
			if mode == "revoked" {
				revoked = now
			}
			if mode == "idle" {
				last = now.Add(-7 * time.Hour)
			}
			state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
				if strings.Contains(q, "sys_auth_session") {
					if mode == "unknown" {
						return []string{"id"}, nil, nil
					}
					return []string{"id", "admin_id", "last_activity_at", "expires_at", "revoked_at", "created_at"}, [][]driver.Value{{"test-session", int64(1), last, expires, revoked, now.Add(-2 * time.Hour)}}, nil
				}
				if mode == "missing-account" {
					return []string{"id"}, nil, nil
				}
				status := int64(1)
				if mode == "disabled" {
					status = 0
				}
				return []string{"id", "username", "status"}, [][]driver.Value{{int64(1), "test", status}}, nil
			}}
			svc := &Service{db: systemDB(t, state)}
			data, refresh, err := svc.RefreshSession("test-refresh", time.Time{})
			if mode == "active" {
				if err != nil || refresh != "test-refresh" || data["token"] == "" || data["sessionExpiresAt"] != expires.UnixMilli() {
					t.Fatalf("active session failed to renew or absolute deadline moved: %v %v", data, err)
				}
				if expiry, ok := data["accessTokenExpiresAt"].(int64); !ok || expiry < now.Add(59*time.Minute).UnixMilli() {
					t.Fatal("access token did not receive a fresh one-hour lifetime")
				}
			} else if !errors.Is(err, ErrRefreshSessionRejected) || data != nil || refresh != "" {
				t.Fatalf("invalid session was not definitively rejected: %v", err)
			}
		})
	}
}
