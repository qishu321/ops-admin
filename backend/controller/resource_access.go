package controller

import (
	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"ops-admin/backend/service"
)

// Scoped binds a controller to immutable request authorization without copying
// runtime mutexes or storing user state on the shared controller/service.
func (ctl *Controller) Scoped(handler func(*Controller, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := access.From(c.Request.Context())
		requestController := &Controller{service: ctl.service.WithAccess(c.Request.Context(), p)}
		handler(requestController, c)
	}
}
func (ctl *Controller) ResourceAccessData(c *gin.Context) {
	data, err := ctl.service.ResourceAccessData()
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) ResourceAccessOptions(c *gin.Context) {
	data, err := ctl.service.ResourceAccessOptions()
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) SaveUserGroup(c *gin.Context) {
	var p service.UserGroupPayload
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "用户组参数不正确")
		return
	}
	if err := ctl.service.SaveUserGroup(p); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
func (ctl *Controller) DeleteUserGroup(c *gin.Context) {
	var p service.IDPayload
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "用户组参数不正确")
		return
	}
	if err := ctl.service.DeleteUserGroup(p.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
func (ctl *Controller) SaveResourceGrant(c *gin.Context) {
	var p model.ResourceGrant
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "授权参数不正确")
		return
	}
	if err := ctl.service.SaveResourceGrant(p); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
func (ctl *Controller) DeleteResourceGrant(c *gin.Context) {
	var p service.IDPayload
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "授权参数不正确")
		return
	}
	if err := ctl.service.DeleteResourceGrant(p.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) SaveAccessPolicy(c *gin.Context) {
	var p model.ResourceGrant
	if c.ShouldBindJSON(&p) != nil {
		httpx.Failed(c, 400, "策略参数不正确")
		return
	}
	if err := ctl.service.SaveAccessPolicy(p); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
func (ctl *Controller) EffectiveAccess(c *gin.Context) {
	data, err := ctl.service.EffectiveAccess(uint(mustAtoi(c.Query("userId"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveGroupBindings(c *gin.Context) {
	var payload service.GroupBindingsPayload
	if c.ShouldBindJSON(&payload) != nil {
		httpx.Failed(c, 400, "绑定参数不正确")
		return
	}
	if err := ctl.service.SaveGroupBindings(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
