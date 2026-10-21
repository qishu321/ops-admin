package httpx

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/internal/access"
)

func TestResourceInventoryDoesNotExposeConnectionSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest("GET", "/api/v1/asset/host/list", nil).WithContext(access.Context(context.Background(), &access.Policy{UserID: 2}))
	Success(c, map[string]any{"list": []any{map[string]any{"id": 1, "hostName": "allowed-host", "credential": map[string]any{"password": "ssh-secret"}, "gateway": map[string]any{"host": "gateway.internal", "credential": map[string]any{"privateKey": "private-key"}}, "cloudAccount": map[string]any{"name": "cloud", "secretKey": "cloud-secret"}, "kubeConfig": "kube-secret"}}})
	body := r.Body.String()
	for _, secret := range []string{"ssh-secret", "private-key", "cloud-secret", "kube-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("inventory leaked %s", secret)
		}
	}
	if !strings.Contains(body, "gateway.internal") || !strings.Contains(body, "allowed-host") {
		t.Fatal("safe inventory metadata must remain usable")
	}
	var result any
	if json.Unmarshal(r.Body.Bytes(), &result) != nil {
		t.Fatal("redacted inventory must be valid JSON")
	}
}
