package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"ops-admin/backend/httpx"
	"ops-admin/backend/internal/access"
)

func WebSocketAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.Request.Header.Set("Authorization", "Bearer "+c.Query("token")); Auth(db)(c) }
}

type resourceRoute struct {
	Kind, Action string
	Permissions  []string
	Admin        bool
}

// Unmapped operational routes fail closed for non-administrators. This also
// prevents an unintegrated execution/aggregate endpoint from bypassing scope.
func resourceRouteFor(method, path string) resourceRoute {
	path = strings.TrimPrefix(path, "/api/v1")
	read := method == http.MethodGet || method == http.MethodHead
	if strings.HasPrefix(path, "/asset/middleware/") {
		action := "read"
		switch {
		case strings.HasSuffix(path, "/save"), strings.HasSuffix(path, "/delete"), strings.HasSuffix(path, "/test"):
			return resourceRoute{Admin: true}
		case strings.HasSuffix(path, "/query"):
			action = "query"
		case strings.HasSuffix(path, "/write"):
			action = "write"
		case strings.HasSuffix(path, "/remove"):
			action = "delete"
		}
		return resourceRoute{Kind: "middleware", Action: action, Permissions: []string{"assets:middleware:" + map[string]string{"read": "list", "query": "query", "write": "write", "delete": "delete"}[action]}}
	}
	if strings.HasPrefix(path, "/asset/command-filter/") {
		return resourceRoute{Admin: true}
	}
	if path == "/profile" || path == "/admin/updatePersonal" || path == "/admin/updatePersonalPassword" {
		return resourceRoute{Action: "self"}
	}
	if strings.HasPrefix(path, "/system/") || strings.HasPrefix(path, "/systemConfig") || strings.HasPrefix(path, "/admin/") || strings.HasPrefix(path, "/role/") || strings.HasPrefix(path, "/menu/") || strings.HasPrefix(path, "/dept/") || strings.HasPrefix(path, "/post/") {
		return resourceRoute{Admin: true}
	}
	if strings.HasPrefix(path, "/domain/") {
		return resourceRoute{Action: "existing"}
	}
	if path == "/ops/environment/list" {
		return resourceRoute{Action: "self"}
	}
	if strings.HasPrefix(path, "/asset/hostGroup/") {
		if read {
			return resourceRoute{Kind: "host_group", Action: "read", Permissions: []string{"assets:hostgroup:list", "assets:host:list", "assets:terminal", "ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:file"}}
		}
		return resourceRoute{Admin: true}
	}
	if strings.HasPrefix(path, "/asset/host/") || strings.HasPrefix(path, "/asset/terminal/") {
		action, permission := "read", "assets:host:list"
		switch {
		case strings.HasSuffix(path, "/ws"):
			action = "terminal"
			permission = "assets:host:terminal"
		case strings.Contains(path, "/terminal/upload"):
			action = "upload"
			permission = "assets:host:upload"
		case strings.Contains(path, "/terminal/"):
			action = "download"
			permission = "assets:host:download"
		case strings.HasSuffix(path, "/delete"):
			action = "manage"
			permission = "assets:host:delete"
		case !read:
			return resourceRoute{Admin: true}
		}
		permissions := []string{permission}
		if action == "read" {
			permissions = append(permissions, "assets:terminal", "ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:file")
		}
		return resourceRoute{Kind: "host", Action: action, Permissions: permissions}
	}
	if strings.HasPrefix(path, "/asset/database/") {
		if !read {
			return resourceRoute{Admin: true}
		}
		return resourceRoute{Kind: "database", Action: "read", Permissions: []string{"assets:database:list", "assets:database:workbench", "assets:database:backup", "assets:database:import"}}
	}
	if strings.HasPrefix(path, "/dbms/") {
		if path == "/dbms/backup/plan/save" || path == "/dbms/backup/plan/delete" {
			return resourceRoute{Admin: true}
		}
		action, permission := "query", "assets:database:workbench"
		switch {
		case strings.Contains(path, "/backup/"):
			permission = "assets:database:backup"
			action = "backup"
			if strings.HasSuffix(path, "/import") {
				action = "restore"
				permission = "assets:database:backup:restore"
			}
			if strings.HasSuffix(path, "/download") {
				action = "export"
			}
		case strings.Contains(path, "/task/export") || strings.Contains(path, "/task/download"):
			action = "export"
			permission = "assets:database:export"
		case strings.Contains(path, "/import") || strings.HasSuffix(path, "/batch-sql"):
			action = "write"
			permission = "assets:database:import:execute"
		case strings.Contains(path, "/row/") || strings.HasSuffix(path, "/schema/create"):
			action = "write"
			permission = "assets:database:data:edit"
		case strings.HasSuffix(path, "/execute"):
			permission = "assets:database:sql:execute"
		}
		return resourceRoute{Kind: "database", Action: action, Permissions: []string{permission}}
	}
	if strings.HasPrefix(path, "/k8s/") {
		action := "read"
		permissions := []string{"assets:k8s:cluster", "assets:k8s:overview", "assets:k8s:node", "assets:k8s:namespace", "assets:k8s:workload", "assets:k8s:pod", "assets:k8s:advancednetwork", "assets:k8s:configstorage", "assets:k8s:scalingpolicy"}
		switch {
		case strings.HasSuffix(path, "/terminal/ws"):
			action = "terminal"
			permissions = []string{"assets:k8s:pod:terminal"}
		case strings.HasSuffix(path, "/pod/upload"):
			action = "upload"
			permissions = []string{"assets:k8s:pod:upload"}
		case strings.HasSuffix(path, "/pod/logs"):
			action = "logs"
			permissions = []string{"assets:k8s:pod"}
		case strings.HasSuffix(path, "/workload/scale"):
			action = "scale"
			permissions = []string{"assets:k8s:workload:scale"}
		case strings.HasSuffix(path, "/workload/restart"):
			action = "restart"
			permissions = []string{"assets:k8s:workload:restart"}
		case strings.Contains(path, "/cluster/") && !read:
			return resourceRoute{Admin: true}
		case !read:
			action = "write"
			permissions = []string{"assets:k8s:workload:yaml"}
			if strings.Contains(path, "/delete") {
				action = "delete"
				permissions = []string{"assets:k8s:pod:delete"}
			}
			if strings.Contains(path, "/images") {
				permissions = []string{"assets:k8s:workload:image"}
			}
			if strings.HasSuffix(path, "/workload/create") {
				permissions = []string{"assets:k8s:workload:create"}
			}
		}
		return resourceRoute{Kind: "k8s", Action: action, Permissions: permissions}
	}
	if strings.HasPrefix(path, "/monitor/logs/") {
		return resourceRoute{Kind: "datasource", Action: "query", Permissions: []string{"monitor:logs"}}
	}
	if path == "/monitor/datasource/options" || path == "/monitor/datasource/list" {
		return resourceRoute{Kind: "datasource", Action: "read", Permissions: []string{"monitor:datasource", "monitor:query", "monitor:logs", "monitor:traces"}}
	}
	if path == "/monitor/query/instant" || path == "/monitor/query/range" {
		return resourceRoute{Kind: "datasource", Action: "query", Permissions: []string{"monitor:query"}}
	}
	if strings.HasPrefix(path, "/asset/credential/") {
		if strings.HasSuffix(path, "/add") {
			return resourceRoute{Admin: true}
		}
		action := "manage"
		if read {
			action = "read"
		}
		permission := "assets:credential:list"
		if strings.HasSuffix(path, "/update") {
			permission = "assets:credential:edit"
		}
		if strings.HasSuffix(path, "/delete") {
			permission = "assets:credential:delete"
		}
		return resourceRoute{Kind: "credential", Action: action, Permissions: []string{permission}}
	}
	if strings.HasPrefix(path, "/ops/script/") && read {
		return resourceRoute{Action: "read", Permissions: []string{"ops:script:list", "ops:quickexec:script"}}
	}
	if strings.HasPrefix(path, "/ops/exec/") {
		if read {
			return resourceRoute{Kind: "host", Action: "read", Permissions: []string{"ops:quickexec:history", "ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:file"}}
		}

		action := "execute"
		permission := "ops:quickexec:command:execute"
		if strings.HasSuffix(path, "/script") {
			permission = "ops:quickexec:script:execute"
		}
		if strings.HasSuffix(path, "/file-dispatch") {
			action = "upload"
			permission = "ops:quickexec:file:execute"
		}
		if strings.HasSuffix(path, "/retry") {
			return resourceRoute{Kind: "host", Action: "execute", Permissions: []string{"ops:quickexec:command:execute", "ops:quickexec:script:execute"}}
		}
		return resourceRoute{Kind: "host", Action: action, Permissions: []string{permission}}
	}
	return resourceRoute{Admin: true}
}

func ResourceAuthorization(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := access.Load(db, c.GetUint("userID"))
		if err != nil {
			httpx.Failed(c, 403, "无法确认当前用户的访问权限")
			c.Abort()
			return
		}
		authorizeResourceRequest(c, p)
	}
}
func authorizeResourceRequest(c *gin.Context, p *access.Policy) {
	rule := resourceRouteFor(c.Request.Method, c.Request.URL.Path)
	if rule.Kind != "" {
		p.Actions[rule.Kind] = rule.Action
	}
	c.Request = c.Request.WithContext(access.Context(c.Request.Context(), p))
	deny := func() { httpx.Failed(c, 403, "没有访问该资源或执行该操作的权限"); c.Abort() }
	if p.Super {
		c.Next()
		return
	}
	if rule.Admin {
		deny()
		return
	}
	if len(rule.Permissions) > 0 {
		found := false
		for _, permission := range rule.Permissions {
			found = found || p.Has(permission)
		}
		if !found {
			deny()
			return
		}
	}
	// Validate explicit targets before handlers can mutate/delete. Lists are
	// filtered in SQL before count/pagination; compound operations validate
	// every resolved target again at their execution boundary.
	values := map[string]any{}
	for key, items := range c.Request.URL.Query() {
		if len(items) > 0 {
			values[key] = items[0]
		}
	}
	if c.Request.Body != nil && c.Request.Method != http.MethodGet && !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 8<<20+1))
		if readErr != nil || len(body) > 8<<20 {
			httpx.Failed(c, 400, "请求内容过大或无法读取")
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var posted map[string]any
		if json.Unmarshal(body, &posted) == nil {
			for key, value := range posted {
				values[key] = value
			}
		}
	}
	idKeys := map[string][]string{"middleware": {"id"}, "host": {"hostId", "hostIds"}, "k8s": {"clusterId"}, "database": {"databaseId", "targetDatabaseId"}, "datasource": {"datasourceId"}, "credential": {"id"}}
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/asset/host/") {
		idKeys["host"] = append(idKeys["host"], "id", "ids")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/asset/database/") {
		idKeys["database"] = append(idKeys["database"], "id")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/k8s/cluster/") && strings.HasSuffix(c.Request.URL.Path, "/info") {
		idKeys["k8s"] = append(idKeys["k8s"], "id")
	}
	scope, _ := values["namespace"].(string)
	if rule.Kind == "datasource" && strings.Contains(c.Request.URL.Path, "/monitor/logs/") {
		scope, _ = values["index"].(string)
	}
	if rule.Kind == "database" {
		for _, id := range resourceIDs(values["sourceDatabaseId"]) {
			if !p.Allows("database", id, "", "export") {
				deny()
				return
			}
		}
	}
	for _, key := range idKeys[rule.Kind] {
		for _, id := range resourceIDs(values[key]) {
			if rule.Action == "read" {
				if !p.Visible(rule.Kind, id) {
					deny()
					return
				}
			} else if strings.HasSuffix(c.Request.URL.Path, "/monitor/logs/indices") && p.Visible("datasource", id) {
				// The service filters every returned index with the same policy.
			} else if !p.Allows(rule.Kind, id, scope, rule.Action) {
				deny()
				return
			}
		}
	}
	// Raw connection configuration is never included in ordinary inventory.
	if strings.HasSuffix(c.Request.URL.Path, "/k8s/cluster/info") {
		id := firstResourceID(values["id"])
		if !p.Allows("k8s", id, "", "manage") {
			deny()
			return
		}
	}
	c.Next()
}
func firstResourceID(value any) uint {
	ids := resourceIDs(value)
	if len(ids) > 0 {
		return ids[0]
	}
	return 0
}
func resourceIDs(value any) []uint {
	if values, ok := value.([]any); ok {
		ids := []uint{}
		for _, v := range values {
			ids = append(ids, resourceIDs(v)...)
		}
		return ids
	}
	var n uint64
	switch v := value.(type) {
	case string:
		n, _ = strconv.ParseUint(v, 10, 64)
	case float64:
		if v > 0 && v == float64(uint64(v)) {
			n = uint64(v)
		}
	}
	if n == 0 {
		return nil
	}
	return []uint{uint(n)}
}
