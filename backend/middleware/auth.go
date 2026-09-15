package middleware

import (
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/auth"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Auth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			httpx.Failed(c, 401, "请先登录")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(header, "Bearer ")
		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			httpx.Failed(c, 401, auth.TokenErrorMessage(err))
			c.Abort()
			return
		}
		if claims.SessionID == "" {
			httpx.Failed(c, 401, "登录会话无效，请重新登录")
			c.Abort()
			return
		}

		var session model.AuthSession
		if err := db.Where("id = ? AND admin_id = ?", claims.SessionID, claims.UserID).First(&session).Error; err != nil {
			httpx.Failed(c, 401, "登录会话无效，请重新登录")
			c.Abort()
			return
		}
		now := time.Now()
		lastActivityAt := session.LastActivityAt
		if rawActivity := c.GetHeader("X-User-Activity"); rawActivity != "" {
			if milliseconds, parseErr := strconv.ParseInt(rawActivity, 10, 64); parseErr == nil {
				reportedAt := time.UnixMilli(milliseconds)
				if reportedAt.After(lastActivityAt) && !reportedAt.After(now.Add(time.Minute)) {
					lastActivityAt = reportedAt
				}
			}
		}
		if session.RevokedAt != nil || auth.SessionExpired(now, lastActivityAt, session.ExpiresAt, session.RememberLogin) {
			httpx.Failed(c, 401, "登录已过期，请重新登录")
			c.Abort()
			return
		}

		// The browser reports actual keyboard/pointer activity. Background API
		// polling therefore does not accidentally keep an unattended session alive.
		if lastActivityAt.After(session.LastActivityAt) {
			_ = db.Model(&model.AuthSession{}).Where("id = ?", session.ID).Update("last_activity_at", lastActivityAt).Error
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}
