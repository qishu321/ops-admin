package model

import "time"

type CommandFilter struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	MatchType   string    `json:"matchType" gorm:"size:16;not null"`
	Pattern     string    `json:"pattern" gorm:"type:text;not null"`
	Scopes      []string  `json:"scopes" gorm:"serializer:json;type:text"`
	Enabled     bool      `json:"enabled" gorm:"not null;default:false"`
	Description string    `json:"description" gorm:"size:500"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (CommandFilter) TableName() string { return "asset_command_filter" }
