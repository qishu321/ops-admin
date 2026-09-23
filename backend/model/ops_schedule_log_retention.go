package model

import "time"

// OpsScheduleLogRetentionSetting controls automatic cleanup of HTTP probe logs.
// Script task logs are intentionally outside this policy.
type OpsScheduleLogRetentionSetting struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	RetentionDays    int        `json:"retentionDays" gorm:"not null;default:7"`
	LastCleanupAt    *time.Time `json:"lastCleanupAt"`
	LastDeletedCount int64      `json:"lastDeletedCount" gorm:"not null;default:0"`
	CreatedAt        time.Time  `json:"createTime"`
	UpdatedAt        time.Time  `json:"updateTime"`
}

func (OpsScheduleLogRetentionSetting) TableName() string {
	return "ops_schedule_log_retention_setting"
}
