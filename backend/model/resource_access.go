package model

import "time"

type UserGroup struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null;uniqueIndex"`
	Description string    `json:"description" gorm:"size:500"`
	Status      int       `json:"status" gorm:"not null;default:1"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (UserGroup) TableName() string { return "sys_user_group" }

type UserGroupMember struct {
	GroupID uint `json:"groupId" gorm:"primaryKey"`
	AdminID uint `json:"adminId" gorm:"primaryKey;index"`
}

func (UserGroupMember) TableName() string { return "sys_user_group_member" }

// Policy bindings reuse grants; templates require an explicit resource scope.
type GroupAccessBinding struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	GroupID      uint   `json:"groupId" gorm:"not null;index"`
	Kind         string `json:"kind" gorm:"size:16;not null"`
	SourceID     uint   `json:"sourceId" gorm:"not null;index"`
	ResourceType string `json:"resourceType" gorm:"size:32"`
	ResourceID   uint   `json:"resourceId"`
	Scope        string `json:"scope" gorm:"size:255"`
}

func (GroupAccessBinding) TableName() string { return "sys_group_access_binding" }

// An empty Scope covers the entire instance. Namespaces and log indexes use
// exact names, never user-controlled glob/SQL expressions.
type ResourceGrant struct {
	BindingID   uint   `json:"bindingId,omitempty" gorm:"-"`
	BindingKind string `json:"bindingKind,omitempty" gorm:"-"`
	SourceID    uint   `json:"sourceId,omitempty" gorm:"-"`
	Name        string `json:"name" gorm:"size:128"`
	Enabled     *bool  `json:"enabled" gorm:"default:true"`
	Unified     bool   `json:"unified" gorm:"not null;default:false"`
	TemplateID  uint   `json:"templateId"`

	ID           uint      `json:"id" gorm:"primaryKey"`
	SubjectType  string    `json:"subjectType" gorm:"size:16;not null;index:idx_grant_subject"`
	SubjectID    uint      `json:"subjectId" gorm:"not null;index:idx_grant_subject"`
	ResourceType string    `json:"resourceType" gorm:"size:32;not null;index"`
	ResourceID   uint      `json:"resourceId" gorm:"not null;index"`
	Scope        string    `json:"scope" gorm:"size:255"`
	Actions      []string  `json:"actions" gorm:"serializer:json;type:text"`
	Description  string    `json:"description" gorm:"size:500"`
	CreatedAt    time.Time `json:"createTime"`
	UpdatedAt    time.Time `json:"updateTime"`
}

func (ResourceGrant) TableName() string { return "sys_resource_grant" }
