package controller

import (
	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"
	"ops-admin/backend/service"
	"strings"
)

func middlewareResponse(c *gin.Context, data any, err error) {
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) ListMiddleware(c *gin.Context) {
	v, e := ctl.service.ListMiddleware(c.Query("type"))
	middlewareResponse(c, v, e)
}
func (ctl *Controller) SaveMiddleware(c *gin.Context) {
	var d model.MiddlewareDatasource
	if c.ShouldBindJSON(&d) != nil {
		httpx.Failed(c, 400, "参数不正确")
		return
	}
	e := ctl.service.SaveMiddleware(d)
	middlewareResponse(c, true, e)
}
func (ctl *Controller) DeleteMiddleware(c *gin.Context) {
	var d service.IDPayload
	if c.ShouldBindJSON(&d) != nil {
		httpx.Failed(c, 400, "参数不正确")
		return
	}
	middlewareResponse(c, true, ctl.service.DeleteMiddleware(d.ID))
}
func (ctl *Controller) TestMiddleware(c *gin.Context) {
	var d model.MiddlewareDatasource
	if c.ShouldBindJSON(&d) != nil {
		httpx.Failed(c, 400, "参数不正确")
		return
	}
	v, e := ctl.service.TestMiddleware(d)
	middlewareResponse(c, v, e)
}
func (ctl *Controller) OperateMiddleware(c *gin.Context) {
	var d service.MiddlewareOperation
	if c.ShouldBindJSON(&d) != nil {
		httpx.Failed(c, 400, "参数不正确")
		return
	}
	action := "query"
	if strings.HasSuffix(c.Request.URL.Path, "/write") {
		action = "write"
	}
	if strings.HasSuffix(c.Request.URL.Path, "/remove") {
		action = "delete"
	}
	v, e := ctl.service.OperateMiddleware(d, action)
	middlewareResponse(c, v, e)
}
