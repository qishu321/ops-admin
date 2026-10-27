package middleware

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"time"

	"ops-admin/backend/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func OperationLog(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestSummary := auditRequestSummary(c)
		c.Next()

		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1") {
			return
		}
		if c.Request.Method == "GET" || c.Request.URL.Path == "/api/v1/login" {
			return
		}

		userID, _ := c.Get("userID")
		username, _ := c.Get("username")
		statusCode := c.Writer.Status()
		logItem := model.OperationLog{
			Method:         c.Request.Method,
			IP:             c.ClientIP(),
			URL:            c.Request.URL.Path,
			Description:    actionDescription(c.Request.URL.Path, c.Request.Method),
			RiskLevel:      riskLevel(c.Request.URL.Path, c.Request.Method),
			StatusCode:     statusCode,
			Success:        statusCode >= 200 && statusCode < 400,
			DurationMs:     time.Since(start).Milliseconds(),
			RequestSummary: requestSummary,
			CreatedAt:      time.Now(),
		}
		if id, ok := userID.(uint); ok {
			logItem.AdminID = id
		}
		if name, ok := username.(string); ok {
			logItem.Username = name
		}
		db.Create(&logItem)
	}
}

func auditRequestSummary(c *gin.Context) string {
	parts := make([]string, 0, 2)
	if query := strings.TrimSpace(c.Request.URL.RawQuery); query != "" {
		parts = append(parts, "query: "+maskSensitive(query))
	}
	if c.Request.Body != nil && c.Request.Method != "GET" {
		if c.Request.URL.Path == "/api/v1/domain/public/certificates/upload" {
			parts = append(parts, "body: SSL Certificate 与 Private Key 敏感正文，已跳过记录")
			return strings.Join(parts, "\n")
		}
		contentType := strings.ToLower(c.GetHeader("Content-Type"))
		if strings.Contains(contentType, "multipart/form-data") {
			parts = append(parts, "body: multipart/form-data 文件上传，已跳过正文记录")
			return strings.Join(parts, "\n")
		}
		if c.Request.ContentLength > 4096 || c.Request.ContentLength < 0 {
			parts = append(parts, "body: 请求体较大，已跳过正文记录")
			return strings.Join(parts, "\n")
		}
		data, _ := io.ReadAll(io.LimitReader(c.Request.Body, 4096))
		c.Request.Body = io.NopCloser(bytes.NewBuffer(data))
		body := strings.TrimSpace(string(data))
		if body != "" {
			parts = append(parts, "body: "+maskSensitive(compactText(body, 1200)))
		}
	}
	return strings.Join(parts, "\n")
}

func maskSensitive(value string) string {
	result := value
	sensitiveKeys := []string{
		"password", "passwd", "secret", "secretKey", "accessKey", "token", "kubeConfig", "kubeconfig",
		"client-key-data", "clientCertificateData", "privateKey", "privateKeyPem", "keyData", "credential",
	}
	for _, key := range sensitiveKeys {
		jsonPattern := regexp.MustCompile(`(?i)("` + regexp.QuoteMeta(key) + `"\s*:\s*")([^"]*)(")`)
		queryPattern := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(key) + `=)([^&\s]*)`)
		result = jsonPattern.ReplaceAllString(result, `${1}***${3}`)
		result = queryPattern.ReplaceAllString(result, `${1}***`)
	}
	return result
}

func compactText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...(已截断)"
}

func riskLevel(path string, method string) string {
	if method == "DELETE" || strings.Contains(path, "/clean") || strings.Contains(path, "/batch/delete") {
		return "high"
	}
	highRiskPaths := []string{
		"/k8s/deploy/start", "/k8s/deploy/stop",
		"/database/sql", "/database/export", "/database/import", "/k8s/yaml", "/k8s/workload/images",
		"/ops/quick", "/ops/job", "/ops/schedule", "/ops/application/pipeline/run",
	}
	for _, item := range highRiskPaths {
		if strings.Contains(path, item) {
			return "high"
		}
	}
	if method == "POST" || method == "PUT" {
		return "medium"
	}
	return "normal"
}

func actionDescription(path string, method string) string {
	switch {
	case strings.Contains(path, "/asset/host"):
		return "主机资产管理"
	case strings.Contains(path, "/asset/database"):
		return "数据库资产管理"
	case strings.Contains(path, "/asset/gateway"):
		return "网关资产管理"
	case strings.Contains(path, "/database"):
		return "数据库工作台"
	case strings.Contains(path, "/k8s"):
		return "K8s 资源管理"
	case strings.Contains(path, "/ops/quick"):
		return "快速执行"
	case strings.Contains(path, "/ops/job"):
		return "作业编排"
	case strings.Contains(path, "/ops/schedule"):
		return "定时任务"
	case strings.Contains(path, "/ops/application"):
		return "应用发布"
	case strings.Contains(path, "/notify"):
		return "消息通知"
	case strings.Contains(path, "/monitor"):
		return "监控中心"
	case strings.Contains(path, "/domain"):
		return "域名管理"
	case strings.Contains(path, "/admin"):
		return "用户管理"
	case strings.Contains(path, "/role"):
		return "角色管理"
	case strings.Contains(path, "/menu"):
		return "菜单管理"
	case strings.Contains(path, "/dept"):
		return "部门管理"
	case strings.Contains(path, "/post"):
		return "岗位管理"
	case strings.Contains(path, "/sysLoginInfo"):
		return "登录日志管理"
	case strings.Contains(path, "/sysOperationLog"):
		return "操作日志管理"
	default:
		return method + " " + path
	}
}
