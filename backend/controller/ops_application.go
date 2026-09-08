package controller

import (
	"strconv"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetOpsApplicationList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsApplications(pageNum, pageSize, c.Query("keyword"), c.Query("repoType"), c.Query("status"), c.Query("serviceType"), c.Query("env"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsApplicationOptions(c *gin.Context) {
	data, err := ctl.service.ListOpsApplicationOptions()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsApplicationInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsApplication(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsApplicationBindings(c *gin.Context) {
	data, err := ctl.service.ListOpsApplicationEnvironmentBindings(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveOpsApplication(c *gin.Context) {
	var payload service.OpsApplicationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid application payload")
		return
	}
	if err := ctl.service.SaveOpsApplication(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsApplication(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsApplication(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetOpsAppBuildTaskList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsAppBuildTasks(
		pageNum,
		pageSize,
		uint(mustAtoi(c.Query("appId"))),
		c.Query("keyword"),
		c.Query("env"),
		c.Query("status"),
	)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppBuildTaskInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsAppBuildTask(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveOpsAppBuildTask(c *gin.Context) {
	var payload service.OpsAppBuildTaskPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid build task payload")
		return
	}
	if err := ctl.service.SaveOpsAppBuildTask(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsAppBuildTaskStatus(c *gin.Context) {
	var payload service.OpsAppBuildTaskStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid build task status payload")
		return
	}
	if err := ctl.service.UpdateOpsAppBuildTaskStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsAppBuildTask(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsAppBuildTask(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RunOpsAppBuildTask(c *gin.Context) {
	var payload service.OpsAppBuildRunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid build run payload")
		return
	}
	data, err := ctl.service.RunOpsAppBuildTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RunOpsAppRelease(c *gin.Context) {
	var payload service.OpsAppReleasePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid release payload")
		return
	}
	data, err := ctl.service.RunOpsAppRelease(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RetryOpsAppRelease(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid release retry payload")
		return
	}
	data, err := ctl.service.RetryOpsAppRelease(payload.ID)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppReleaseList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsAppReleases(pageNum, pageSize, uint(mustAtoi(c.Query("appId"))), c.Query("keyword"), c.Query("status"), c.Query("env"), c.Query("startTime"), c.Query("endTime"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppReleaseInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsAppRelease(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppArtifactList(c *gin.Context) {
	data, err := ctl.service.ListOpsAppArtifacts(uint(mustAtoi(c.Query("appId"))), c.Query("env"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsImageRegistryList(c *gin.Context) {
	data, err := ctl.service.ListOpsImageRegistries(c.Query("enabledOnly") == "1")
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveOpsImageRegistry(c *gin.Context) {
	var payload service.OpsImageRegistryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid image registry payload")
		return
	}
	if err := ctl.service.SaveOpsImageRegistry(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsImageRegistry(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid image registry delete payload")
		return
	}
	if err := ctl.service.DeleteOpsImageRegistry(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetOpsAppPipelineTemplateList(c *gin.Context) {
	data, err := ctl.service.ListOpsAppPipelineTemplates(c.Query("category"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveOpsAppPipelineTemplate(c *gin.Context) {
	var payload service.OpsAppPipelineTemplatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline template payload")
		return
	}
	if err := ctl.service.SaveOpsAppPipelineTemplate(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) NormalizeOpsAppPipelineTemplateDefinition(c *gin.Context) {
	var payload struct {
		Definition string `json:"definition"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline template definition")
		return
	}
	data, err := ctl.service.NormalizeOpsAppPipelineTemplateDefinition(payload.Definition)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppPipelineList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsAppPipelines(
		pageNum,
		pageSize,
		uint(mustAtoi(c.Query("appId"))),
		c.Query("keyword"),
		c.Query("env"),
		c.Query("status"),
		c.Query("techStack"),
	)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppPipelineInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsAppPipeline(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SaveOpsAppPipeline(c *gin.Context) {
	var payload service.OpsAppPipelinePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline payload")
		return
	}
	if err := ctl.service.SaveOpsAppPipeline(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsAppPipelineStatus(c *gin.Context) {
	var payload service.OpsAppPipelineStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline status payload")
		return
	}
	if err := ctl.service.UpdateOpsAppPipelineStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsAppPipeline(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsAppPipeline(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) CopyOpsAppPipeline(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid copy payload")
		return
	}
	data, err := ctl.service.CopyOpsAppPipeline(payload.ID)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RunOpsAppPipeline(c *gin.Context) {
	var payload service.OpsAppPipelineRunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline run payload")
		return
	}
	data, err := ctl.service.RunOpsAppPipeline(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppPipelineRunList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsAppPipelineRuns(
		pageNum,
		pageSize,
		uint(mustAtoi(c.Query("pipelineId"))),
		uint(mustAtoi(c.Query("appId"))),
		c.Query("keyword"),
		c.Query("status"),
		c.Query("env"),
	)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsAppPipelineRunInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsAppPipelineRun(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ApproveOpsAppPipelineRun(c *gin.Context) {
	var payload service.OpsAppPipelineApprovalPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline approval payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.ApproveOpsAppPipelineRun(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RollbackOpsAppPipelineRun(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid pipeline rollback payload")
		return
	}
	data, err := ctl.service.RollbackOpsAppPipelineRun(payload.ID, c.GetString("username"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}
