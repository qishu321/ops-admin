package auth

import (
	"testing"
	"time"
)

func TestGenerateTokenUsesConfiguredAccessTTL(t *testing.T) {
	t.Setenv("OPS_ADMIN_JWT_SECRET", "test-only-secret-with-sufficient-length")
	before := time.Now()
	token, expiresAt, err := GenerateToken(7, "admin", "session-123")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if claims.UserID != 7 || claims.Username != "admin" || claims.SessionID != "session-123" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if expiresAt.Before(before.Add(AccessTokenTTL-time.Second)) || expiresAt.After(before.Add(AccessTokenTTL+time.Second)) {
		t.Fatalf("expiry %v does not match access TTL %v", expiresAt, AccessTokenTTL)
	}
}

func TestOpaqueTokenIsRandomAndOnlyHashIsPersistable(t *testing.T) {
	first, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken() error = %v", err)
	}
	second, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken() error = %v", err)
	}
	if first == second || len(first) != 64 {
		t.Fatalf("opaque tokens are not sufficiently random")
	}
	if hash := HashOpaqueToken(first); hash == first || len(hash) != 64 {
		t.Fatalf("unexpected token hash %q", hash)
	}
}

func TestGenerateTokenUntilDoesNotOutliveSession(t *testing.T) {
	t.Setenv("OPS_ADMIN_JWT_SECRET", "test-only-secret-with-sufficient-length")
	deadline := time.Now().Add(10 * time.Minute)
	token, expiresAt, err := GenerateTokenUntil(7, "admin", "session-123", deadline)
	if err != nil {
		t.Fatalf("GenerateTokenUntil() error = %v", err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if expiresAt.After(deadline) || claims.ExpiresAt.Time.After(deadline) {
		t.Fatalf("access token expiry %v exceeds session deadline %v", expiresAt, deadline)
	}
}

func TestRememberedSessionSkipsIdleTimeoutButKeepsFixedDeadline(t *testing.T) {
	loginAt := time.Date(2026, 9, 14, 9, 0, 0, 0, time.Local)
	deadline := loginAt.Add(SessionMaxTTL)

	if !SessionExpired(loginAt.Add(SessionIdleTTL), loginAt, deadline, false) {
		t.Fatal("ordinary session should expire after the idle timeout")
	}
	if SessionExpired(loginAt.Add(6*24*time.Hour), loginAt, deadline, true) {
		t.Fatal("remembered session should not expire because it was idle")
	}
	if !SessionExpired(deadline, loginAt, deadline, true) {
		t.Fatal("remembered session must expire at the original fixed deadline")
	}
}
