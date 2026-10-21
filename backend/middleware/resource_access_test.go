package middleware

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func TestResourceRequestGateRequiresFeatureAndResourceForEveryTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, body, permission string
		grants                               []model.ResourceGrant
		status                               int
	}{
		{"authorized SSH", "GET", "/api/v1/asset/terminal/ws?hostId=1", "", "assets:host:terminal", []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"read", "terminal"}}}, 204},
		{"SSH wrong host", "GET", "/api/v1/asset/terminal/ws?hostId=2", "", "assets:host:terminal", []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"terminal"}}}, 403},
		{"SSH missing feature", "GET", "/api/v1/asset/terminal/ws?hostId=1", "", "assets:host:list", []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"terminal"}}}, 403},
		{"batch contains unauthorized host", "DELETE", "/api/v1/asset/host/batch/delete", `{"ids":[1,2]}`, "assets:host:delete", []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"manage"}}}, 403},
		{"namespace terminal", "GET", "/api/v1/k8s/pod/terminal/ws?clusterId=2&namespace=dev", "", "assets:k8s:pod:terminal", []model.ResourceGrant{{ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"terminal"}}}, 204},
		{"namespace grant can discover its cluster", "GET", "/api/v1/k8s/cluster/list", "", "assets:k8s:cluster", []model.ResourceGrant{{ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"read"}}}, 204},
		{"resource grant cannot edit cluster connection", "PUT", "/api/v1/k8s/cluster/update", `{"id":2}`, "assets:k8s:cluster:edit", []model.ResourceGrant{{ResourceType: "k8s", ResourceID: 2, Actions: []string{"manage"}}}, 403},
		{"other namespace terminal", "GET", "/api/v1/k8s/pod/terminal/ws?clusterId=2&namespace=prod", "", "assets:k8s:pod:terminal", []model.ResourceGrant{{ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"terminal"}}}, 403},
		{"exact log index", "POST", "/api/v1/monitor/logs/query", `{"datasourceId":3,"index":"dev-logs"}`, "monitor:logs", []model.ResourceGrant{{ResourceType: "datasource", ResourceID: 3, Scope: "dev-logs", Actions: []string{"query"}}}, 204},
		{"all log indices denied", "POST", "/api/v1/monitor/logs/query", `{"datasourceId":3,"index":"_all"}`, "monitor:logs", []model.ResourceGrant{{ResourceType: "datasource", ResourceID: 3, Scope: "dev-logs", Actions: []string{"query"}}}, 403},
		{"index discovery", "GET", "/api/v1/monitor/logs/indices?datasourceId=3", "", "monitor:logs", []model.ResourceGrant{{ResourceType: "datasource", ResourceID: 3, Scope: "dev-logs", Actions: []string{"query"}}}, 204},
		{"SQL wrong connection", "POST", "/api/v1/dbms/sql/execute", `{"databaseId":5}`, "assets:database:sql:execute", []model.ResourceGrant{{ResourceType: "database", ResourceID: 4, Actions: []string{"query"}}}, 403},
		{"cannot elevate own role", "PUT", "/api/v1/role/assignPermissions", `{"id":1}`, "system:role:assign", nil, 403},
		{"cannot bypass via unintegrated route", "POST", "/api/v1/ops/application/pipeline/run", `{"id":1}`, "ops:app:pipeline:run", nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			engine := gin.New()
			p := &access.Policy{Permissions: []string{tc.permission}, Grants: tc.grants, Actions: map[string]string{}}
			engine.Use(func(c *gin.Context) { authorizeResourceRequest(c, p) })
			engine.Any("/*path", func(c *gin.Context) {
				if tc.body != "" {
					body, _ := io.ReadAll(c.Request.Body)
					if string(body) != tc.body {
						t.Errorf("authorization changed payload: %s", body)
					}
				}
				c.Status(204)
			})
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestResourceGateDoesNotTrustContentTypeForJSONHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	p := &access.Policy{Actions: map[string]string{}, Permissions: []string{"assets:host:delete"}, Grants: []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"manage"}}}}
	engine.Use(func(c *gin.Context) { authorizeResourceRequest(c, p) })
	engine.DELETE("/api/v1/asset/host/batch/delete", func(c *gin.Context) { t.Error("unauthorized batch reached handler"); c.Status(204) })
	req := httptest.NewRequest("DELETE", "/api/v1/asset/host/batch/delete", bytes.NewBufferString(`{"ids":[1,2]}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("Content-Type bypass: %d %s", rec.Code, rec.Body.String())
	}
}
