package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/service"
)

func (ctl *Controller) K8sDeploy(c *gin.Context) {
	var data any
	var err error
	integer := func(key string, fallback int) int {
		n, e := strconv.Atoi(c.Query(key))
		if e != nil || n < 0 {
			return fallback
		}
		return n
	}
	action := strings.TrimPrefix(c.FullPath(), "/api/v1/k8s/deploy/")
	if c.Request.Method == http.MethodGet {
		switch action {
		case "capabilities":
			data = ctl.service.K8sDeployCapabilities()
		case "list":
			data, err = ctl.service.ListK8sDeployPlans(integer("pageNum", 1), integer("pageSize", 10), c.Query("keyword"), c.Query("status"))
		case "host-options":
			data, err = ctl.service.K8sDeployHostOptions(c.Query("keyword"), c.Query("environment"), uint(integer("groupId", 0)), integer("pageNum", 1), integer("pageSize", 10))
		case "info":
			data, err = ctl.service.K8sDeployDetail(uint(integer("id", 0)))
		case "logs":
			data, err = ctl.service.K8sDeployLogs(uint(integer("id", 0)), uint(integer("runId", 0)), uint(integer("after", 0)))
		}
	} else {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
		if action == "save" {
			var payload service.K8sDeployPayload
			if e := c.ShouldBindJSON(&payload); e != nil {
				httpx.Failed(c, 400, "部署配置格式不正确或内容过大")
				return
			}
			data, err = ctl.service.SaveK8sDeployPlan(payload, c.GetString("username"))
		} else {
			var payload struct {
				ID       uint `json:"id"`
				Revision int  `json:"revision"`
			}
			if e := c.ShouldBindJSON(&payload); e != nil || payload.ID == 0 {
				httpx.Failed(c, 400, "请选择有效的部署计划")
				return
			}
			switch action {
			case "precheck":
				data, err = ctl.service.StartK8sDeployRun(payload.ID, payload.Revision, "precheck", c.GetString("username"))
			case "start":
				data, err = ctl.service.StartK8sDeployRun(payload.ID, payload.Revision, "deploy", c.GetString("username"))
			case "stop":
				err = ctl.service.StopK8sDeploy(payload.ID)
				data = true
			case "delete":
				err = ctl.service.DeleteK8sDeploy(payload.ID)
				data = true
			case "register-cluster":
				data, err = ctl.service.RegisterK8sDeployCluster(payload.ID)
			}
		}
	}
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
