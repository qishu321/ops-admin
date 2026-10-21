package model

import "time"

type IntegrationNavigationGroup struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name" gorm:"size:128;not null;index"`
	Description   string    `json:"description" gorm:"size:500"`
	IsPublic      bool      `json:"isPublic" gorm:"not null;default:false;index"`
	PublicToken   string    `json:"publicToken" gorm:"size:64;index"`
	PublicOrigins []string  `json:"publicOrigins" gorm:"serializer:json;type:text"`
	Status        int       `json:"status" gorm:"not null;default:1;index"`
	Sort          int       `json:"sort" gorm:"not null;default:0;index"`
	CreatedAt     time.Time `json:"createTime"`
	UpdatedAt     time.Time `json:"updateTime"`
}

func (IntegrationNavigationGroup) TableName() string {
	return "integration_navigation_group"
}

type IntegrationNavigation struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	GroupID     uint      `json:"groupId" gorm:"not null;index"`
	Name        string    `json:"name" gorm:"size:128;not null;index"`
	Description string    `json:"description" gorm:"size:500"`
	URL         string    `json:"url" gorm:"size:2048;not null"`
	IconURL     string    `json:"iconUrl" gorm:"size:2048"`
	OpenMode    string    `json:"openMode" gorm:"size:16;not null;default:new"`
	Status      int       `json:"status" gorm:"not null;default:1;index"`
	Sort        int       `json:"sort" gorm:"not null;default:0;index"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (IntegrationNavigation) TableName() string {
	return "integration_navigation"
}
