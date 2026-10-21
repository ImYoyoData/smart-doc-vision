package model

import "time"

// Role is a code-level RBAC role. Built-in roles are seeded from the catalog
// and cannot be removed; administrators can add custom roles with their own
// permission and menu subsets.
type Role struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Code        string    `gorm:"size:48;uniqueIndex;not null" json:"code"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	Permissions JSONText  `json:"permissions"`
	Menus       JSONText  `json:"menus"`
	Builtin     bool      `gorm:"not null;default:false" json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Role) TableName() string { return "sys_roles" }
