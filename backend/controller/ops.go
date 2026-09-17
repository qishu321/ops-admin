package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetOpsScriptList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsScripts(pageNum, pageSize, c.Query("keyword"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScriptOptions(c *gin.Context) {
	data, err := ctl.service.ListOpsScriptOptions()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScriptInfo(c *gin.Context) {
	data, err := ctl.service.GetOpsScript(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateOpsScript(c *gin.Context) {
	var payload service.OpsScriptPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid script payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.CreateOpsScript(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsScript(c *gin.Context) {
	var payload service.OpsScriptPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid script payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.UpdateOpsScript(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteOpsScript(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOpsScript(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateOpsScriptStatus(c *gin.Context) {
	var payload service.OpsScriptStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdateOpsScriptStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) ExecuteOpsCommand(c *gin.Context) {
	var payload service.OpsExecCommandPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid command execution payload")
		return
	}
	payload.Operator = c.GetString("username")
	payload.SourceIP = c.ClientIP()
	data, err := ctl.service.ExecuteOpsCommand(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ExecuteOpsScript(c *gin.Context) {
	var payload service.OpsExecScriptPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid script execution payload")
		return
	}
	payload.Operator = c.GetString("username")
	payload.SourceIP = c.ClientIP()
	data, err := ctl.service.ExecuteOpsScript(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ExecuteOpsFileDispatch(c *gin.Context) {
	payload, upload, err := parseOpsFileDispatchPayload(c)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
			httpx.Failed(c, http.StatusRequestEntityTooLarge, "本地上传文件大小不能超过 500 MB")
		} else {
			httpx.Failed(c, 400, err.Error())
		}
		return
	}
	payload.Operator = c.GetString("username")
	payload.SourceIP = c.ClientIP()
	data, runErr := ctl.service.ExecuteOpsFileDispatch(payload, upload)
	if runErr != nil {
		upload.Cleanup()
		httpx.Failed(c, 400, runErr.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsScriptVersions(c *gin.Context) {
	data, err := ctl.service.ListOpsScriptVersions(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RollbackOpsScript(c *gin.Context) {
	var payload struct {
		ID      uint `json:"id"`
		Version int  `json:"version"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid rollback payload")
		return
	}
	if err := ctl.service.RollbackOpsScript(payload.ID, payload.Version, c.GetString("username")); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RetryOpsExecTask(c *gin.Context) {
	var payload service.OpsExecRetryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid retry payload")
		return
	}
	payload.Operator = c.GetString("username")
	payload.SourceIP = c.ClientIP()
	data, err := ctl.service.RetryFailedOpsExecTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsExecTaskList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOpsExecTasks(pageNum, pageSize, c.Query("keyword"), c.Query("taskType"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetOpsExecTaskDetail(c *gin.Context) {
	data, err := ctl.service.GetOpsExecTaskDetail(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func parseOpsFileDispatchPayload(c *gin.Context) (service.OpsFileDispatchPayload, *service.OpsFileDispatchUpload, error) {
	// Keep a small envelope allowance for multipart fields and boundaries, while
	// enforcing the exact 500 MiB content limit below from the file metadata.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.OpsFileDispatchMaxUploadBytes+2*1024*1024)
	payload := service.OpsFileDispatchPayload{
		Title:          strings.TrimSpace(c.PostForm("title")),
		SourceType:     strings.TrimSpace(c.PostForm("sourceType")),
		SourceHostID:   uint(mustAtoi(c.PostForm("sourceHostId"))),
		SourcePath:     strings.TrimSpace(c.PostForm("sourcePath")),
		TargetDir:      strings.TrimSpace(c.PostForm("targetDir")),
		TargetFileName: strings.TrimSpace(c.PostForm("targetFileName")),
		TargetPath:     strings.TrimSpace(c.PostForm("targetPath")),
		Concurrency:    mustAtoi(c.DefaultPostForm("concurrency", "5")),
		TimeoutSeconds: mustAtoi(c.DefaultPostForm("timeoutSeconds", "10")),
		Overwrite:      strings.EqualFold(c.DefaultPostForm("overwrite", "false"), "true"),
		RiskConfirmed:  strings.EqualFold(c.DefaultPostForm("riskConfirmed", "false"), "true"),
	}

	hostIDs, err := parseUintSliceField(c.PostForm("hostIds"))
	if err != nil {
		return payload, nil, err
	}
	groupIDs, err := parseUintSliceField(c.PostForm("groupIds"))
	if err != nil {
		return payload, nil, err
	}
	payload.HostIDs = hostIDs
	payload.GroupIDs = groupIDs

	file, err := c.FormFile("file")
	if err != nil {
		if payload.SourceType == "upload" {
			return payload, nil, err
		}
		return payload, nil, nil
	}
	if file.Size > service.OpsFileDispatchMaxUploadBytes {
		return payload, nil, errors.New("本地上传文件大小不能超过 500 MB")
	}
	reader, err := file.Open()
	if err != nil {
		return payload, nil, err
	}
	defer reader.Close()
	directory := filepath.Join("uploads", "temp", "file-dispatch")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return payload, nil, err
	}
	target, err := os.CreateTemp(directory, "upload-*")
	if err != nil {
		return payload, nil, err
	}
	tempPath := target.Name()
	cleanup := func(err error) (service.OpsFileDispatchPayload, *service.OpsFileDispatchUpload, error) {
		_ = target.Close()
		_ = os.Remove(tempPath)
		return payload, nil, err
	}
	size, copyErr := io.Copy(target, io.LimitReader(reader, service.OpsFileDispatchMaxUploadBytes+1))
	if copyErr != nil {
		return cleanup(copyErr)
	}
	if closeErr := target.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return payload, nil, closeErr
	}
	if size > service.OpsFileDispatchMaxUploadBytes {
		_ = os.Remove(tempPath)
		return payload, nil, errors.New("本地上传文件大小不能超过 500 MB")
	}
	return payload, &service.OpsFileDispatchUpload{FileName: filepath.Base(file.Filename), TempPath: tempPath, Size: size}, nil
}

func parseUintSliceField(raw string) ([]uint, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	var list []uint
	if strings.HasPrefix(value, "[") {
		var numbers []uint
		if err := json.Unmarshal([]byte(value), &numbers); err != nil {
			return nil, err
		}
		return numbers, nil
	}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		list = append(list, uint(number))
	}
	return list, nil
}
