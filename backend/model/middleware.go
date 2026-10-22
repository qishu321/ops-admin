package model

import "time"

type MiddlewareDatasource struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"size:128;not null"`
	Type         string    `json:"type" gorm:"size:24;index;not null"`
	Endpoints    []string  `json:"endpoints" gorm:"serializer:json;type:text"`
	Version      string    `json:"version" gorm:"size:16"`
	CredentialID uint      `json:"credentialId"`
	AuthType     string    `json:"authType" gorm:"size:24"`
	TLS          bool      `json:"tls"`
	Description  string    `json:"description" gorm:"size:500"`
	CreatedAt    time.Time `json:"createTime"`
	UpdatedAt    time.Time `json:"updateTime"`
}

func (MiddlewareDatasource) TableName() string { return "asset_middleware" }
