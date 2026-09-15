package model

import "time"

// AuthSession keeps refresh-token state on the server so sessions can be
// revoked and idle/absolute timeouts are enforced independently of the browser.
type AuthSession struct {
	ID               string     `json:"id" gorm:"primaryKey;size:64"`
	AdminID          uint       `json:"adminId" gorm:"index;not null"`
	RefreshTokenHash string     `json:"-" gorm:"size:64;uniqueIndex;not null"`
	LastActivityAt   time.Time  `json:"lastActivityAt" gorm:"index;not null"`
	ExpiresAt        time.Time  `json:"expiresAt" gorm:"index;not null"`
	RememberLogin    bool       `json:"rememberLogin" gorm:"not null;default:false"`
	RevokedAt        *time.Time `json:"revokedAt" gorm:"index"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func (AuthSession) TableName() string {
	return "sys_auth_session"
}
