package model

import "time"

// LDAPConfig keeps directory integration settings separate from public site
// settings so the bind password is never returned by the login configuration API.
type LDAPConfig struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	Enabled            bool      `json:"enabled" gorm:"default:false"`
	ServerURL          string    `json:"serverUrl" gorm:"size:255"`
	TLSMode            string    `json:"tlsMode" gorm:"size:16;default:'starttls'"`
	InsecureSkipVerify bool      `json:"insecureSkipVerify" gorm:"default:false"`
	BindDN             string    `json:"bindDn" gorm:"size:255"`
	BindPassword       string    `json:"-" gorm:"size:255"`
	BaseDN             string    `json:"baseDn" gorm:"size:255"`
	UserFilter         string    `json:"userFilter" gorm:"size:512"`
	UsernameAttribute  string    `json:"usernameAttribute" gorm:"size:64;default:'uid'"`
	DisplayAttribute   string    `json:"displayAttribute" gorm:"size:64;default:'displayName'"`
	EmailAttribute     string    `json:"emailAttribute" gorm:"size:64;default:'mail'"`
	PhoneAttribute     string    `json:"phoneAttribute" gorm:"size:64;default:'mobile'"`
	DefaultGroupIDs    []uint    `json:"defaultGroupIds" gorm:"serializer:json;type:text"`
	DefaultRoleID      uint      `json:"defaultRoleId"`
	CreatedAt          time.Time `json:"createTime"`
	UpdatedAt          time.Time `json:"updateTime"`
}

func (LDAPConfig) TableName() string {
	return "sys_ldap_config"
}
