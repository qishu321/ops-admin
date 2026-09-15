package model

import "time"

type Role struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	RoleName    string    `json:"roleName" gorm:"size:64;not null"`
	RoleKey     string    `json:"roleKey" gorm:"size:64;uniqueIndex;not null"`
	Status      int       `json:"status" gorm:"default:1;not null"`
	IsReadOnly  bool      `json:"isReadOnly" gorm:"not null;default:false"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

// IsGlobalReadOnlyMenu identifies the safe navigation set for the platform
// maintained global read-only role. Button permissions are always excluded.
func IsGlobalReadOnlyMenu(menu Menu) bool {
	if menu.MenuType == 3 {
		return false
	}
	switch menu.Value {
	case "system", "logs",
		"assets:terminal", "assets:credential:list", "assets:cloudaccount:list", "assets:gateway:list",
		"assets:database:workbench", "assets:database:import", "assets:database:backup",
		"ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:file", "ops:job:approval",
		"domains:account:list", "domains:settings:view":
		return false
	}
	return menu.MenuType == 1 || menu.MenuType == 2
}

func (Role) TableName() string {
	return "sys_role"
}

type RoleMenu struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	RoleID uint `json:"roleId" gorm:"index;not null"`
	MenuID uint `json:"menuId" gorm:"index;not null"`
}

func (RoleMenu) TableName() string {
	return "sys_role_menu"
}
