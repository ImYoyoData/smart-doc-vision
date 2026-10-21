package model

import "time"

type ProjectMember struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_project_member,priority:1" json:"user_id"`
	ProjectID uint64    `gorm:"not null;uniqueIndex:idx_project_member,priority:2;index" json:"project_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (ProjectMember) TableName() string { return "project_members" }
