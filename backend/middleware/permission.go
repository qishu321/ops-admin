package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"ops-admin/backend/httpx"
	"ops-admin/backend/internal/access"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequirePermission enforces sensitive permissions server-side. The frontend
// directive is only a usability aid and must never be the security boundary.
func RequirePermission(db *gorm.DB, permission string) gin.HandlerFunc {
	return RequireAnyPermission(db, permission)
}

// RequireCreateOrEditPermission selects the permission from the payload ID so
// a role that may create objects cannot use the shared save endpoint to edit
// existing ones (and vice versa).
func RequireCreateOrEditPermission(db *gorm.DB, createPermission, editPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			httpx.Failed(c, 400, "invalid request payload")
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(data))
		var payload struct {
			ID uint `json:"id"`
		}
		permission := createPermission
		if json.Unmarshal(data, &payload) == nil && payload.ID > 0 {
			permission = editPermission
		}
		RequirePermission(db, permission)(c)
	}
}

// RequireAnyPermission accepts any one of the supplied permissions. This is
// useful for shared read endpoints (for example DNS account options) that are
// consumed by more than one independently-authorized page.
func RequireAnyPermission(db *gorm.DB, permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := access.From(c.Request.Context()); p != nil {
			for _, permission := range permissions {
				if p.Has(permission) {
					c.Next()
					return
				}
			}
			httpx.Failed(c, 403, "没有执行该操作的权限")
			c.Abort()
			return
		}
		values := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			if permission = strings.TrimSpace(permission); permission != "" {
				values = append(values, permission)
			}
		}
		if len(values) == 0 {
			httpx.Failed(c, 403, "没有执行该操作的权限")
			c.Abort()
			return
		}
		var count int64
		err := db.Table("sys_admin_role ar").
			Joins("JOIN sys_role_menu rm ON rm.role_id=ar.role_id").
			Joins("JOIN sys_menu m ON m.id=rm.menu_id").
			Where("ar.admin_id = ? AND m.value IN ? AND m.menu_status = ?", c.GetUint("userID"), values, 1).
			Count(&count).Error
		if err != nil || count == 0 {
			httpx.Failed(c, 403, "没有执行该操作的权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
