package middleware

import (
	"net/http"
	"strings"

	"ops-admin/backend/httpx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GlobalReadOnly is a server-side safety boundary for the platform-provided
// global read-only role. It intentionally does not rely on hidden UI buttons.
func GlobalReadOnly(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var row struct {
			IsReadOnly bool `gorm:"column:is_read_only"`
		}
		err := db.Table("sys_admin_role ar").
			Select("r.is_read_only").
			Joins("JOIN sys_role r ON r.id = ar.role_id").
			Where("ar.admin_id = ?", c.GetUint("userID")).
			Order("ar.id asc").
			Limit(1).
			Scan(&row).Error
		if err != nil {
			httpx.Failed(c, http.StatusForbidden, "无法确认当前角色权限")
			c.Abort()
			return
		}
		if row.IsReadOnly && !globalReadOnlyRequestAllowed(c.Request.Method, c.Request.URL.Path) {
			httpx.Failed(c, http.StatusForbidden, "全局只读角色不能执行变更、运行任务或访问敏感配置")
			c.Abort()
			return
		}
		c.Next()
	}
}

func globalReadOnlyRequestAllowed(method, path string) bool {
	if method == http.MethodPost {
		switch path {
		case "/api/v1/monitor/query/instant", "/api/v1/monitor/query/range", "/api/v1/monitor/logs/query",
			"/api/v1/monitor/traces/query", "/api/v1/monitor/dashboard/panel/query",
			"/api/v1/domain/internal/query":
			return true
		default:
			return false
		}
	}
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	// These endpoints expose credentials, privileged administration data, or a
	// remote execution surface, so they stay outside the global-read template.
	for _, prefix := range []string{
		"/api/v1/admin/", "/api/v1/role/", "/api/v1/menu/", "/api/v1/dept/", "/api/v1/post/",
		"/api/v1/systemConfig", "/api/v1/system/", "/api/v1/asset/credential", "/api/v1/asset/cloudAccount",
		"/api/v1/asset/gateway", "/api/v1/asset/database", "/api/v1/dbms/", "/api/v1/ops/script/info",
		"/api/v1/ops/script/versions", "/api/v1/domain/public/accounts", "/api/v1/integration/navigation",
		"/api/v1/integration/ai/", "/api/v1/integration/finops/account",
	} {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	switch path {
	case "/api/v1/asset/service/diagnosis/run", "/api/v1/asset/host/template",
		"/api/v1/domain/public/certificates/download-private":
		return false
	}
	return true
}
