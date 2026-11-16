package model

import "time"

// LogBoardShare 是一条日志大屏的免登录分享。分享的是一块「可交互的大屏」：
// 打开链接的人可以自己切换 VictoriaLogs 数据源、改 LogsQL、换时间范围，
// 因此换数据源时不需要重新生成链接。查询时长、取样条数由服务端固定，
// 避免链接被当成无限查询接口。
type LogBoardShare struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Token       string `gorm:"size:64;uniqueIndex" json:"token"`
	Name        string `gorm:"size:120" json:"name"`
	Description string `gorm:"size:255" json:"description"`
	// DatasourceID / Query / RangeSeconds 是打开链接时的默认值，查看者可以修改。
	DatasourceID   uint   `gorm:"index" json:"datasourceId"`
	Query          string `gorm:"type:text" json:"query"`
	RangeSeconds   int    `gorm:"default:86400" json:"rangeSeconds"`
	RefreshSeconds int    `gorm:"default:60" json:"refreshSeconds"`
	// AllowDetail 控制公开页面能否查看单条日志的完整正文与字段（含堆栈）。
	AllowDetail  bool       `json:"allowDetail"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	Status       int        `gorm:"index;default:1" json:"status"`
	ViewCount    int64      `json:"viewCount"`
	LastViewedAt *time.Time `json:"lastViewedAt"`
	CreatedBy    string     `gorm:"size:64" json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}
