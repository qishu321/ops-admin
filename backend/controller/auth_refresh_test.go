package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/service"
)

func TestRefreshFailureOnlyDeletesCookieForRejectedSession(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		err := errors.New("database unavailable")
		want := http.StatusServiceUnavailable
		if invalid {
			err = service.ErrRefreshSessionRejected
			want = http.StatusUnauthorized
		}
		writeRefreshSessionError(c, err)
		if recorder.Code != want {
			t.Fatalf("invalid=%v: status %d, want %d", invalid, recorder.Code, want)
		}
		cookies := recorder.Header().Values("Set-Cookie")
		if invalid {
			if len(cookies) != 1 || !strings.Contains(cookies[0], "Max-Age=0") {
				t.Fatalf("rejected session cookie not removed: %v", cookies)
			}
		} else if len(cookies) != 0 || strings.Contains(recorder.Body.String(), "database") {
			t.Fatalf("outage removed cookie or leaked internals: %v %s", cookies, recorder.Body.String())
		}
	}
}
