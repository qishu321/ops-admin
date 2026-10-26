package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/service"
)

func writeRefreshSessionError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrRefreshSessionRejected) {
		clearRefreshCookie(c)
		httpx.Failed(c, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	// A failed database lookup/update is not proof that the session expired.
	// Keep its cookie so the browser can retry once the service recovers.
	httpx.Failed(c, http.StatusServiceUnavailable, "登录续期暂时不可用，请稍后重试")
}
