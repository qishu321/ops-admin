package controller

import (
	"strconv"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetOpsScheduleTaskList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsScheduleTasks(pageNum, pageSize, c.Query("keyword"), c.Query("taskType"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScheduleTaskInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsScheduleTask(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateOpsScheduleTask(c *gin.Context) {
	var payload service.OpsScheduleTaskPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid schedule task payload")
		return
	}
	if err := ctl.service.CreateOpsScheduleTask(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsScheduleTask(c *gin.Context) {
	var payload service.OpsScheduleTaskPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid schedule task payload")
		return
	}
	if err := ctl.service.UpdateOpsScheduleTask(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsScheduleTask(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsScheduleTask(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchDeleteOpsScheduleTask(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.BatchDeleteOpsScheduleTasks(payload.IDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsScheduleTaskStatus(c *gin.Context) {
	var payload service.OpsScheduleTaskStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdateOpsScheduleTaskStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RunOpsScheduleTask(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid run payload")
		return
	}
	if err := ctl.service.RunOpsScheduleTask(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) PreviewOpsScheduleTaskNotification(c *gin.Context) {
	var payload service.OpsScheduleNotifyPreviewPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid notification preview payload")
		return
	}
	data, err := ctl.service.PreviewOpsScheduleTaskNotification(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScheduleLogList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsScheduleTaskLogs(pageNum, pageSize, c.Query("keyword"), c.Query("taskType"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScheduleLogInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsScheduleTaskLog(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScheduleTemplateList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsScheduleTemplates(pageNum, pageSize, c.Query("keyword"), c.Query("taskType"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScheduleTemplateInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsScheduleTemplate(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateOpsScheduleTemplate(c *gin.Context) {
	var payload service.OpsScheduleTemplatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid schedule template payload")
		return
	}
	if err := ctl.service.CreateOpsScheduleTemplate(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsScheduleTemplate(c *gin.Context) {
	var payload service.OpsScheduleTemplatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid schedule template payload")
		return
	}
	if err := ctl.service.UpdateOpsScheduleTemplate(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsScheduleTemplate(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsScheduleTemplate(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}
