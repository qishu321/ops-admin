package controller

import (
	"errors"
	"io"
	"net/http"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetLogBoardShareList(c *gin.Context) {
	data, err := ctl.service.ListLogBoardShares()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveLogBoardShare(c *gin.Context) {
	var payload service.LogBoardSharePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "分享配置格式无效")
		return
	}
	data, err := ctl.service.SaveLogBoardShare(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteLogBoardShare(c *gin.Context) {
	var payload struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "参数格式无效")
		return
	}
	if err := ctl.service.DeleteLogBoardShare(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, gin.H{"id": payload.ID})
}

func (ctl *Controller) UpdateLogBoardShareStatus(c *gin.Context) {
	var payload struct {
		ID     uint `json:"id"`
		Status int  `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "参数格式无效")
		return
	}
	data, err := ctl.service.SetLogBoardShareStatus(payload.ID, payload.Status)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RegenerateLogBoardShareToken(c *gin.Context) {
	var payload struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "参数格式无效")
		return
	}
	data, err := ctl.service.RegenerateLogBoardShareToken(payload.ID)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

// GetPublicLogBoardMeta 是免登录入口：只返回大屏标题、默认查询和可切换的数据源清单。
func (ctl *Controller) GetPublicLogBoardMeta(c *gin.Context) {
	data, err := ctl.service.GetPublicLogBoardMeta(c.Param("token"))
	if err != nil {
		writeLogBoardShareError(c, err)
		return
	}
	httpx.Success(c, data)
}

// GetPublicLogBoardData 执行公开大屏的一次分析。数据源、LogsQL 和时间范围
// 由查看者在页面上选择，服务端只做范围与频率限制。
func (ctl *Controller) GetPublicLogBoardData(c *gin.Context) {
	var payload service.PublicLogBoardQuery
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		httpx.Failed(c, 400, "参数格式无效")
		return
	}
	data, err := ctl.service.GetPublicLogBoardData(c.Param("token"), payload, c.ClientIP())
	if err != nil {
		writeLogBoardShareError(c, err)
		return
	}
	httpx.Success(c, data)
}

func writeLogBoardShareError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrLogBoardShareNotFound):
		httpx.Failed(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrLogBoardShareDisabled), errors.Is(err, service.ErrLogBoardShareExpired):
		httpx.Failed(c, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrLogBoardShareTooBusy):
		httpx.Failed(c, http.StatusTooManyRequests, err.Error())
	default:
		httpx.Failed(c, 500, err.Error())
	}
}
