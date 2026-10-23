package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

// Retry context never leaves the server, and includes the original resolved
// variables, which may be secrets. Encrypt the entire snapshot at rest.
type opsRetryContext struct {
	Script    *model.OpsScript  `json:"script,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	FilePath  string            `json:"filePath,omitempty"`
	Overwrite bool              `json:"overwrite"`
}

var opsRetryMu sync.Mutex

func encodeOpsRetryContext(value opsRetryContext) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return util.EncryptSecret(string(data))
}

func decodeOpsRetryContext(value string) (opsRetryContext, error) {
	var result opsRetryContext
	if value == "" {
		return result, errors.New("该历史任务未保留重试副本，请重新提交原任务")
	}
	plain, err := util.DecryptSecret(value)
	if err != nil {
		return result, errors.New("无法读取任务重试副本")
	}
	err = json.Unmarshal([]byte(plain), &result)
	return result, err
}

func opsRetryDirectory() string { return filepath.Join("data", "ops-retry") }

func retainOpsRetryFile(source string) (string, time.Time, error) {
	return retainOpsRetryFileIn(opsRetryDirectory(), source, time.Now())
}

func retainOpsRetryFileIn(directory, source string, now time.Time) (string, time.Time, error) {
	expires := now.Add(24 * time.Hour)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", expires, err
	}
	reader, err := os.Open(source)
	if err != nil {
		return "", expires, err
	}
	defer reader.Close()
	file, err := os.CreateTemp(directory, "dispatch-*")
	if err != nil {
		return "", expires, err
	}
	name, err := filepath.Abs(file.Name())
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", expires, err
	}
	_, copyErr := io.Copy(file, io.LimitReader(reader, OpsFileDispatchMaxUploadBytes+1))
	info, statErr := file.Stat()
	closeErr := file.Close()
	if copyErr != nil || statErr != nil || closeErr != nil || info.Size() > OpsFileDispatchMaxUploadBytes {
		os.Remove(name)
		return "", expires, errors.New("保存文件重试副本失败或文件超过500 MB")
	}
	return name, expires, nil
}

func cleanupOpsRetryFiles(directory string, now time.Time, protected ...string) {
	keep := map[string]bool{}
	for _, name := range protected {
		absolute, _ := filepath.Abs(name)
		keep[absolute] = true
	}
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "dispatch-") {
			continue
		}
		absolute, _ := filepath.Abs(filepath.Join(directory, entry.Name()))
		if keep[absolute] {
			continue
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
}

func (s *Service) initOpsRetryCleanup() {
	cleanup := func() {
		var tasks []model.OpsExecTask
		if err := s.db.Where("task_type = ? AND status = ?", "file", "running").Find(&tasks).Error; err != nil {
			return
		}
		protected := []string{}
		for _, task := range tasks {
			if task.RetryContext == "" {
				continue
			}
			snapshot, err := decodeOpsRetryContext(task.RetryContext)
			if err != nil {
				return
			}
			protected = append(protected, snapshot.FilePath)
		}
		cleanupOpsRetryFiles(opsRetryDirectory(), time.Now(), protected...)
	}
	cleanup()
	if s.opsScheduler != nil {
		if _, err := s.opsScheduler.cron.AddFunc("0 15 * * * *", cleanup); err != nil {
			log.Printf("schedule execution retry cleanup: %v", err)
		}
	}
}

func selectOpsRetryTargets(rows []model.OpsExecTargetResult, resultID uint) ([]uint, error) {
	ids := []uint{}
	seen := map[uint]bool{}
	for _, row := range rows {
		if resultID != 0 && row.ID != resultID {
			continue
		}
		if row.Status != "failed" && row.Status != "timeout" {
			continue
		}
		if row.HostID != 0 && !seen[row.HostID] {
			ids = append(ids, row.HostID)
			seen[row.HostID] = true
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("请选择该任务中失败或超时的主机；成功及执行中的主机不能重试")
	}
	return ids, nil
}

func excludeOpsRetriedTargets(ids []uint, previous []model.OpsExecTargetResult) []uint {
	blocked := map[uint]bool{}
	for _, result := range previous {
		blocked[result.HostID] = true
	}
	remaining := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !blocked[id] {
			remaining = append(remaining, id)
		}
	}
	return remaining
}

func (s *Service) RetryFailedOpsExecTask(payload OpsExecRetryPayload) (map[string]any, error) {
	// Serialise creation so repeated clicks cannot launch overlapping retries.
	opsRetryMu.Lock()
	defer opsRetryMu.Unlock()
	var original model.OpsExecTask
	if err := s.db.First(&original, payload.TaskID).Error; err != nil {
		return nil, err
	}
	if original.TaskType != "command" && original.TaskType != "script" && original.TaskType != "file" {
		return nil, errors.New("不支持重试该任务类型")
	}
	if p := s.resourcePolicy(); p != nil && !p.Has("ops:quickexec:"+original.TaskType+":execute") {
		return nil, access.ErrDenied
	}
	if original.Status == "running" || original.Status == "pending" {
		return nil, errors.New("任务仍在执行中，暂不能重试")
	}
	var rows []model.OpsExecTargetResult
	if err := s.db.Where("task_id = ?", original.ID).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids, err := selectOpsRetryTargets(rows, payload.ResultID)
	if err != nil {
		return nil, err
	}
	var previous []model.OpsExecTargetResult
	if err := s.db.Table("ops_exec_target_result r").Select("r.*").Joins("JOIN ops_exec_task t ON t.id = r.task_id").Where("t.retry_of_task_id = ? AND r.host_id IN ? AND r.status IN ?", original.ID, ids, []string{"success", "running", "pending"}).Find(&previous).Error; err != nil {
		return nil, err
	}
	ids = excludeOpsRetriedTargets(ids, previous)
	if len(ids) == 0 {
		return nil, errors.New("所选主机已有执行中或成功的重试任务，请查看重试任务结果")
	}
	if original.TaskType == "file" {
		s = s.withResourceOperation("host", "upload")
	}
	hosts, err := s.resolveOpsTargetHosts(ids, nil)
	if err != nil {
		return nil, err
	}
	if len(hosts) != len(ids) {
		return nil, errors.New("部分重试主机已删除或不可访问，请检查主机管理")
	}

	task := original
	task.ID = 0
	task.CreatedAt = time.Time{}
	task.UpdatedAt = time.Time{}
	task.StartedAt, task.FinishedAt = nil, nil
	task.Title = original.Title + "（失败重试）"
	task.HostCount, task.SuccessCount, task.FailedCount = len(hosts), 0, 0
	task.Status, task.Summary = "running", "失败目标重试执行中"
	task.Source, task.RetryOfTaskID = "retry", original.ID
	task.Operator, task.SourceIP = payload.Operator, payload.SourceIP
	task.TargetSnapshot = opsTargetSnapshot(hosts)

	switch original.TaskType {
	case "command":
		command := strings.TrimSpace(original.CommandText + " " + original.Parameters)
		if err := s.CheckCommandFilters("job", command); err != nil {
			return nil, err
		}
		return s.runOpsTaskAsync(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
			return s.execCommandOnHost(host, command, task.TimeoutSeconds)
		})
	case "script":
		snapshot, err := decodeOpsRetryContext(original.RetryContext)
		if err != nil {
			return nil, err
		}
		if snapshot.Script == nil {
			return nil, errors.New("脚本执行快照不存在，请重新提交任务")
		}
		// Check current permission/state, execute the originally approved content.
		current, err := s.GetOpsScript(original.ScriptID)
		if err != nil {
			return nil, err
		}
		if current.Status != 1 {
			return nil, errors.New("当前脚本已禁用，无法重试")
		}
		if err := s.CheckCommandFilters("job", jobFilterContent(snapshot.Script.Content, task.Parameters, snapshot.Variables)); err != nil {
			return nil, err
		}
		return s.runOpsTaskAsync(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
			return s.execScriptOnHost(host, *snapshot.Script, task.Parameters, snapshot.Variables, task.TimeoutSeconds)
		})
	case "file":
		snapshot, err := decodeOpsRetryContext(original.RetryContext)
		if err != nil {
			return nil, err
		}
		if original.FileRetryUntil == nil || !time.Now().Before(*original.FileRetryUntil) {
			return nil, errors.New("文件重试副本已过期（保留24小时），请重新上传文件")
		}
		// Each retry owns a separate copy; simultaneous targets never depend on
		// another retry's cleanup or on a changed remote source file.
		name, expires, err := retainOpsRetryFile(snapshot.FilePath)
		if err != nil {
			return nil, fmt.Errorf("文件重试副本不可用，请重新上传文件: %w", err)
		}
		snapshot.FilePath = name
		task.RetryContext, err = encodeOpsRetryContext(snapshot)
		if err != nil {
			os.Remove(name)
			return nil, err
		}
		task.FileRetryUntil = &expires
		data, err := s.runOpsTaskAsync(task, hosts, func(host model.AssetHost) model.OpsExecTargetResult {
			return s.dispatchFileFromPath(host, task.FileName, task.TargetPath, name, snapshot.Overwrite, task.TimeoutSeconds)
		})
		if err != nil {
			os.Remove(name)
		}
		return data, err
	}
	return nil, errors.New("不支持重试该任务")
}
