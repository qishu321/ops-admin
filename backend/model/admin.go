package model

import "time"

type Admin struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"size:64;uniqueIndex;not null"`
	Password  string    `json:"-"`
	Nickname  string    `json:"nickname" gorm:"size:64;not null"`
	Status    int       `json:"status" gorm:"default:1;not null"`
	Email     string    `json:"email" gorm:"size:128"`
	Phone     string    `json:"phone" gorm:"size:32"`
	Note      string    `json:"note" gorm:"size:255"`
	CreatedAt time.Time `json:"createTime"`
	UpdatedAt time.Time `json:"updateTime"`
}

func (Admin) TableName() string {
	return "sys_admin"
}

type AdminRole struct {
	ID      uint `json:"id" gorm:"primaryKey"`
	AdminID uint `json:"adminId" gorm:"index;not null"`
	RoleID  uint `json:"roleId" gorm:"index;not null"`
}

func (AdminRole) TableName() string {
	return "sys_admin_role"
}

type AdminListItem struct {
	AccountType string `json:"accountType"`
	GroupIDs    []uint `json:"groupIds" gorm:"-"`

	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Status    int       `json:"status"`
	RoleID    uint      `json:"roleId"`
	RoleName  string    `json:"roleName"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createTime"`
}
