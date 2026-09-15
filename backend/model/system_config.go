package model

import "time"

type SystemConfig struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	SiteName             string    `json:"siteName" gorm:"size:100;not null"`
	SiteSlogan           string    `json:"siteSlogan" gorm:"size:255"`
	LogoType             string    `json:"logoType" gorm:"size:32;not null;default:'text'"`
	LogoValue            string    `json:"logoValue" gorm:"size:255"`
	LoginTitle           string    `json:"loginTitle" gorm:"size:100"`
	LoginSubtitle        string    `json:"loginSubtitle" gorm:"size:255"`
	UseLoginBackground   bool      `json:"useLoginBackground" gorm:"default:false"`
	LoginBackground      string    `json:"loginBackground" gorm:"size:255"`
	PrimaryColor         string    `json:"primaryColor" gorm:"size:20;default:'#5b6cf9'"`
	SidebarTheme         string    `json:"sidebarTheme" gorm:"size:20;default:'dark'"`
	RememberLoginEnabled bool      `json:"rememberLoginEnabled" gorm:"not null;default:true"`
	CreatedAt            time.Time `json:"createTime"`
	UpdatedAt            time.Time `json:"updateTime"`
}

func (SystemConfig) TableName() string {
	return "sys_system_config"
}
