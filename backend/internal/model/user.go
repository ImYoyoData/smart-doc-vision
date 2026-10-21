package model

import "time"

type User struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	DisplayName  string     `gorm:"size:64" json:"display_name"`
	Role         string     `gorm:"size:32;not null;index" json:"role"`
	Status       string     `gorm:"size:16;not null;default:ACTIVE" json:"status"`
	TokenVersion int        `gorm:"not null;default:1" json:"-"`
	MFAEnabled   bool       `gorm:"not null;default:false" json:"mfa_enabled"`
	MFASecretEnc string     `gorm:"size:255" json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	LastLoginIP  string     `gorm:"size:64" json:"last_login_ip"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	ProjectIDs  []uint64 `gorm:"-" json:"project_ids"`
	Permissions []string `gorm:"-" json:"permissions"`
	Menus       []string `gorm:"-" json:"menus"`
}

const (
	RoleSuperAdmin = "SUPER_ADMIN"
	RoleAdmin      = "PROJECT_ADMIN"
	RoleAuditor    = "AUDITOR"
	RoleReviewer   = "REVIEWER"
	RoleViewer     = "VIEWER"
)

func (User) TableName() string { return "sys_users" }
