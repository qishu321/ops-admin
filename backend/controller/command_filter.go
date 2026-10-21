package controller

import (
	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"
	"ops-admin/backend/service"
)

func (ctl *Controller) ListCommandFilters(c *gin.Context) {
	data, err := ctl.service.ListCommandFilters()
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) SaveCommandFilter(c *gin.Context) {
	var rule model.CommandFilter
	if c.ShouldBindJSON(&rule) != nil {
		httpx.Failed(c, 400, "规则参数不正确")
		return
	}
	if err := ctl.service.SaveCommandFilter(rule); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
func (ctl *Controller) DeleteCommandFilter(c *gin.Context) {
	var p service.IDPayload
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "请选择规则")
		return
	}
	if err := ctl.service.DeleteCommandFilter(p.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
