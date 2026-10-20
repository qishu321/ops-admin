package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"ops-admin/backend/model"
)

func syncCollectionTotal(snapshot SyncSnapshot) int {
	n := 0
	for _, item := range snapshot.Items {
		if item.CollectStatus != "" {
			n++
		}
	}
	return n
}
func syncCollectionCounts(snapshot SyncSnapshot) (total, success, failed, skipped int) {
	for _, item := range snapshot.Items {
		switch item.CollectStatus {
		case "pending":
			total++
		case "success":
			total++
			success++
		case "failed":
			total++
			failed++
		case "skipped":
			total++
			skipped++
		}
	}
	return
}
func (s *Service) probeAssetSyncHost(ctx context.Context, sourceID uint, item SyncPreviewItem) (status, message string) {
	if ctx.Err() != nil {
		return "failed", "采集任务已超时，可重试"
	}
	var link model.AssetSyncHostLink
	if s.db.Where("source_id = ? AND external_id = ? AND host_id = ?", sourceID, item.ID, item.HostID).First(&link).Error != nil {
		return "failed", "主机同步关联已变化，请重新预览"
	}
	var host model.AssetHost
	if s.db.First(&host, item.HostID).Error != nil {
		return "failed", "本地主机已不存在"
	}
	if host.Status != 1 {
		return "skipped", "主机已停用，未执行 SSH 采集"
	}
	if host.CredentialID == nil {
		return "failed", "主机未绑定登录凭据，请配置后重试采集"
	}
	if _, err := s.syncCredential(*host.CredentialID, false); err != nil {
		return "failed", err.Error()
	}
	result, err := s.SyncAssetHost(host.ID)
	if err != nil {
		return "failed", "主机信息采集失败，请检查网络、凭据与服务日志"
	}
	return syncCollectedHostResult(result)
}

func syncCollectedHostResult(result *model.AssetHost) (string, string) {
	if result == nil {
		return "failed", "未返回主机采集结果，请重试"
	}
	if result.AuthStatus != 1 {
		return "failed", "SSH 认证未通过，请检查登录用户名、凭据及网关"
	}
	if result.AliveStatus != 1 {
		return "failed", "主机连接不可达或采集超时，请检查网络及网关"
	}
	return "success", "连接与认证通过，主机信息已采集"
}

type syncCollectJob struct {
	index int
	item  SyncPreviewItem
}
type syncCollectResult struct {
	index           int
	status, message string
}

// Only the coordinator mutates/persists the snapshot. Network probes are bounded
// independently and never hold the inventory transaction or import mutex.
func collectSyncSnapshot(ctx context.Context, snapshot *SyncSnapshot, probe func(context.Context, SyncPreviewItem) (string, string), persist func(SyncSnapshot) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan syncCollectJob)
	results := make(chan syncCollectResult, 4)
	targets := []syncCollectJob{}
	for i, item := range snapshot.Items {
		if item.CollectStatus == "pending" {
			targets = append(targets, syncCollectJob{i, item})
		}
	}
	if err := persist(*snapshot); err != nil {
		return err
	}
	workerCount := len(targets)
	if workerCount > 4 {
		workerCount = 4
	}
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				result := syncCollectResult{index: job.index, status: "failed", message: "采集发生异常，可重试"}
				func() { defer func() { _ = recover() }(); result.status, result.message = probe(ctx, job.item) }()
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, job := range targets {
			select {
			case jobs <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	completed := 0
	lastFlush := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := persist(*snapshot); err != nil {
				return err
			}
		case result, ok := <-results:
			if !ok {
				return persist(*snapshot)
			}
			snapshot.Items[result.index].CollectStatus = result.status
			snapshot.Items[result.index].CollectMessage = result.message
			completed++
			if completed%4 == 0 || time.Since(lastFlush) > time.Second {
				if err := persist(*snapshot); err != nil {
					return err
				}
				lastFlush = time.Now()
			}
		}
	}
}

func (s *Service) runAssetSyncCollection(src model.AssetSyncSource, runID, attempt uint, snapshot SyncSnapshot) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	persist := func(current SyncSnapshot) error {
		total, success, failed, skipped := syncCollectionCounts(current)
		data, err := json.Marshal(current)
		if err != nil {
			return err
		}
		return s.db.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status = ? AND collect_attempt = ?", runID, "collecting", attempt).Updates(map[string]any{"snapshot": string(data), "collect_total": total, "collect_success": success, "collect_failed": failed, "collect_skipped": skipped, "message": fmt.Sprintf("资产已入库；采集进度 %d/%d，成功 %d，失败 %d，跳过 %d", success+failed+skipped, total, success, failed, skipped)})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				var count int64
				if err := tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status = ? AND collect_attempt = ?", runID, "collecting", attempt).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return errors.New("采集任务已结束或已被替代")
				}
			}
			return tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = ?", src.ID, runID).Update("busy_until", time.Now().Add(5*time.Minute)).Error
		})
	}
	err := collectSyncSnapshot(ctx, &snapshot, func(ctx context.Context, item SyncPreviewItem) (string, string) {
		return s.probeAssetSyncHost(ctx, src.ID, item)
	}, persist)
	if err != nil {
		for i := range snapshot.Items {
			if snapshot.Items[i].CollectStatus == "pending" {
				snapshot.Items[i].CollectStatus = "failed"
				snapshot.Items[i].CollectMessage = "采集中断或超时，资产已保留，可重试采集"
			}
		}
	}
	updates := syncApplyResult(snapshot)
	total, success, failed, skipped := syncCollectionCounts(snapshot)
	updates["inventory_status"] = "success"
	updates["collect_total"] = total
	updates["collect_success"] = success
	updates["collect_failed"] = failed
	updates["collect_skipped"] = skipped
	updates["collect_status"] = "success"
	updates["_collect_attempt"] = attempt
	updates["message"] = fmt.Sprintf("资产已入库；采集成功 %d 台，失败 %d 台，跳过 %d 台", success, failed, skipped)
	if failed > 0 || skipped > 0 || err != nil {
		updates["status"] = "partial"
		updates["collect_status"] = "partial"
		updates["message"] = updates["message"].(string) + "，可直接重试采集"
	}
	return updates
}

func prepareSyncCollectionRetry(snapshot *SyncSnapshot) int {
	targets := 0
	for i := range snapshot.Items {
		item := &snapshot.Items[i]
		if item.HostID > 0 && (item.CollectStatus == "failed" || item.CollectStatus == "skipped" || item.CollectStatus == "pending") {
			item.CollectStatus = "pending"
			item.CollectMessage = "等待重新检测并采集"
			targets++
		}
	}
	return targets
}
func (s *Service) RetryAssetSyncCollection(runID uint) (*model.AssetSyncRun, error) {
	s.expireSyncLeases()
	var run model.AssetSyncRun
	var src model.AssetSyncSource
	var snapshot SyncSnapshot
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&run, runID).Error; err != nil {
			return err
		}
		if err := tx.First(&src, run.SourceID).Error; err != nil {
			return err
		}
		if run.InventoryStatus != "success" || (run.Status != "success" && run.Status != "partial") {
			return errors.New("资产尚未完成入库或任务正在执行，不能重试采集")
		}
		if err := json.Unmarshal([]byte(run.Snapshot), &snapshot); err != nil {
			return err
		}
		if prepareSyncCollectionRetry(&snapshot) == 0 {
			return errors.New("该任务没有待重试的采集项")
		}
		data, _ := json.Marshal(snapshot)
		res := tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status IN ? AND collect_attempt = ?", runID, []string{"success", "partial"}, run.CollectAttempt).Updates(map[string]any{"status": "collecting", "collect_status": "running", "collect_attempt": run.CollectAttempt + 1, "snapshot": string(data), "finished_at": nil, "message": "资产已入库，正在重试信息采集"})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("该任务已在采集，请刷新")
		}
		res = tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = 0", src.ID).Updates(map[string]any{"active_run_id": runID, "busy_until": time.Now().Add(5 * time.Minute)})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("该同步源有任务正在执行，请稍后重试")
		}
		run.CollectAttempt++
		run.Status = "collecting"
		return nil
	})
	if err != nil {
		return nil, err
	}
	go func() {
		updates := map[string]any{"status": "partial", "inventory_status": "success", "collect_status": "partial", "message": "采集执行中断，资产已保留，可重试", "_collect_attempt": run.CollectAttempt}
		defer func() { _ = recover(); s.finishSyncRun(run.ID, src.ID, updates) }()
		updates = s.runAssetSyncCollection(src, run.ID, run.CollectAttempt, snapshot)
	}()
	return &run, nil
}
