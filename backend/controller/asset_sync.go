package controller

import (
	"github.com/gin-gonic/gin"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"
	"ops-admin/backend/service"
)

func syncResponse(c *gin.Context, data any, err error) {
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
func (ctl *Controller) AssetSyncData(c *gin.Context) {
	data, err := ctl.service.AssetSyncData()
	syncResponse(c, data, err)
}

func (ctl *Controller) AssetSyncHostOptions(c *gin.Context) {
	data, err := ctl.service.AssetSyncHostOptions(c.Query("keyword"))
	syncResponse(c, data, err)
}
func (ctl *Controller) SaveAssetSyncSource(c *gin.Context) {
	var p model.AssetSyncSource
	if err := c.ShouldBindJSON(&p); err != nil {
		httpx.Failed(c, 400, "同步源参数无效")
		return
	}
	data, err := ctl.service.SaveAssetSyncSource(p)
	syncResponse(c, data, err)
}
func (ctl *Controller) DeleteAssetSyncSource(c *gin.Context) {
	var p struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || p.ID == 0 {
		httpx.Failed(c, 400, "请选择同步源")
		return
	}
	syncResponse(c, nil, ctl.service.DeleteAssetSyncSource(p.ID))
}
func (ctl *Controller) TestAssetSyncSource(c *gin.Context) {
	data, err := ctl.service.TestAssetSyncSource(c.Request.Context(), uint(mustAtoi(c.Query("id"))))
	syncResponse(c, data, err)
}
func (ctl *Controller) PreviewAssetSync(c *gin.Context) {
	var p struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || p.ID == 0 {
		httpx.Failed(c, 400, "请选择同步源")
		return
	}
	data, err := ctl.service.StartAssetSyncPreview(p.ID)
	syncResponse(c, data, err)
}
func (ctl *Controller) ApplyAssetSync(c *gin.Context) {
	var p service.SyncApplyPayload
	if err := c.ShouldBindJSON(&p); err != nil || p.RunID == 0 {
		httpx.Failed(c, 400, "请选择同步预览")
		return
	}
	data, err := ctl.service.ApplyAssetSyncPreview(p)
	syncResponse(c, data, err)
}
func (ctl *Controller) RetryAssetSyncCollection(c *gin.Context) {
	var p struct {
		RunID uint `json:"runId"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || p.RunID == 0 {
		httpx.Failed(c, 400, "请选择同步任务")
		return
	}
	data, err := ctl.service.RetryAssetSyncCollection(p.RunID)
	syncResponse(c, data, err)
}
func (ctl *Controller) ListAssetSyncRuns(c *gin.Context) {
	data, err := ctl.service.ListAssetSyncRuns(uint(mustAtoi(c.Query("sourceId"))))
	syncResponse(c, data, err)
}
func (ctl *Controller) GetAssetSyncRun(c *gin.Context) {
	data, err := ctl.service.GetAssetSyncRun(uint(mustAtoi(c.Query("id"))))
	syncResponse(c, data, err)
}
