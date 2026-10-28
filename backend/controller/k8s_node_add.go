package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/service"
)

func (ctl *Controller) K8sNodeAdd(c *gin.Context) {
	integer := func(key string, fallback int) int {
		n, e := strconv.Atoi(c.Query(key))
		if e != nil || n < 0 {
			return fallback
		}
		return n
	}
	clusterID := uint(integer("clusterId", 0))
	id64, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	id := uint(id64)
	action := strings.TrimPrefix(c.Request.URL.Path, "/api/v1/k8s/node-add/")
	var data any
	var err error
	if c.Request.Method == http.MethodGet {
		switch action {
		case "context":
			data, err = ctl.service.K8sNodeAddContext(clusterID)
		case "hosts":
			data, err = ctl.service.K8sNodeAddHosts(clusterID, c.Query("keyword"), c.Query("environment"), uint(integer("groupId", 0)), integer("pageNum", 1), integer("pageSize", 5))
		case "tasks":
			data, err = ctl.service.K8sNodeAddTasks(clusterID)
		default:
			data, err = ctl.service.K8sNodeAddTask(clusterID, id)
		}
	} else {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var p service.K8sNodeAddPayload
		if e := c.ShouldBindJSON(&p); e != nil {
			httpx.Failed(c, 400, "节点配置格式错误或内容过大")
			return
		}
		switch action {
		case "prechecks":
			data, err = ctl.service.StartK8sNodeAddPrecheck(p, c.GetString("username"))
		case "tasks":
			data, err = ctl.service.StartK8sNodeAdd(p, c.GetString("username"))
		default:
			data, err = ctl.service.StopK8sNodeAdd(p.ClusterID, id)
		}
	}
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
