package service

import (
	"errors"
	"log"
	"time"

	"ops-admin/backend/model"

	"gorm.io/gorm"
)

const (
	defaultHTTPProbeLogRetentionDays = 7
	maxHTTPProbeLogRetentionDays     = 365
	httpProbeLogCleanupBatchSize     = 500
)

func (s *Service) GetHTTPProbeLogRetention() (*model.OpsScheduleLogRetentionSetting, error) {
	var setting model.OpsScheduleLogRetentionSetting
	if err := s.db.First(&setting, 1).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (s *Service) SaveHTTPProbeLogRetention(days int) (*model.OpsScheduleLogRetentionSetting, error) {
	if days < 1 || days > maxHTTPProbeLogRetentionDays {
		return nil, errors.New("保留天数须在 1 到 365 天之间")
	}
	if err := s.db.Model(&model.OpsScheduleLogRetentionSetting{}).Where("id = ?", 1).
		Update("retention_days", days).Error; err != nil {
		return nil, err
	}
	setting, err := s.GetHTTPProbeLogRetention()
	if err != nil {
		return nil, err
	}
	go s.runHTTPProbeLogCleanup()
	return setting, nil
}

func httpProbeLogRetentionCutoff(now time.Time, days int) time.Time {
	if days < 1 {
		days = defaultHTTPProbeLogRetentionDays
	}
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

func expiredHTTPProbeLogQuery(db *gorm.DB, cutoff time.Time) *gorm.DB {
	return db.Where("task_type = ? AND created_at < ?", "http", cutoff)
}

func (s *Service) initHTTPProbeLogCleanup() {
	if s.opsScheduler == nil {
		return
	}
	if _, err := s.opsScheduler.cron.AddFunc("0 0 * * * *", s.runHTTPProbeLogCleanup); err != nil {
		log.Printf("schedule HTTP probe log cleanup failed: %v", err)
	}
	go s.runHTTPProbeLogCleanup()
}

func (s *Service) runHTTPProbeLogCleanup() {
	if _, err := s.cleanupHTTPProbeLogs(); err != nil {
		log.Printf("HTTP probe log cleanup failed: %v", err)
	}
}

func (s *Service) cleanupHTTPProbeLogs() (int64, error) {
	s.httpProbeLogCleanupMu.Lock()
	defer s.httpProbeLogCleanupMu.Unlock()

	setting, err := s.GetHTTPProbeLogRetention()
	if err != nil {
		return 0, err
	}
	cutoff := httpProbeLogRetentionCutoff(time.Now(), setting.RetentionDays)
	var deleted int64
	for {
		var ids []uint
		// Both queries retain the same type and age guards so script logs stay intact.
		if err := expiredHTTPProbeLogQuery(s.db.Model(&model.OpsScheduleTaskLog{}), cutoff).Order("created_at ASC, id ASC").Limit(httpProbeLogCleanupBatchSize).Pluck("id", &ids).Error; err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			break
		}
		result := expiredHTTPProbeLogQuery(s.db.Where("id IN ?", ids), cutoff).Delete(&model.OpsScheduleTaskLog{})
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < httpProbeLogCleanupBatchSize {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	now := time.Now()
	if err := s.db.Model(&model.OpsScheduleLogRetentionSetting{}).Where("id = ?", 1).Updates(map[string]any{
		"last_cleanup_at": now, "last_deleted_count": deleted,
	}).Error; err != nil {
		return deleted, err
	}
	return deleted, nil
}
