package httpx

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"ops-admin/backend/internal/access"
	"strings"
)

func Success(c *gin.Context, data any) {
	if p := access.From(c.Request.Context()); p != nil && !p.Super {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/v1/asset/") || strings.HasPrefix(path, "/api/v1/k8s/cluster/") || strings.HasPrefix(path, "/api/v1/monitor/datasource/") {
			encoded, err := json.Marshal(data)
			if err != nil {
				Failed(c, 500, "无法返回资源信息")
				return
			}
			var safe any
			if json.Unmarshal(encoded, &safe) != nil {
				Failed(c, 500, "无法返回资源信息")
				return
			}
			redactResourceSecrets(safe)
			data = safe
		}
	}
	c.JSON(200, gin.H{
		"code":    200,
		"message": "success",
		"data":    data,
	})
}

func Failed(c *gin.Context, code int, message string) {
	if strings.Contains(message, access.ErrDenied.Error()) {
		code = 403
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
		"data":    nil,
	})
}

func redactResourceSecrets(value any) {
	switch item := value.(type) {
	case []any:
		for _, v := range item {
			redactResourceSecrets(v)
		}
	case map[string]any:
		for key, v := range item {
			switch strings.ToLower(key) {
			case "password", "privatekey", "passphrase", "secretkey", "accesskey", "token", "kubeconfig", "credential":
				delete(item, key)
			default:
				redactResourceSecrets(v)
			}
		}
	}
}
