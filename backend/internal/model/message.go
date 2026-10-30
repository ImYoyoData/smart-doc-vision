package model

import "time"

const MessageTypeReviewPending = "AUDIT_REVIEW_PENDING"

// InAppMessage is a personal inbox item. DedupeKey makes repeated engine
// transitions for the same task idempotent per user.
type InAppMessage struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64     `gorm:"not null;index:idx_message_user_created,priority:1;uniqueIndex:idx_message_user_dedupe,priority:1" json:"user_id"`
	DedupeKey string     `gorm:"size:128;not null;uniqueIndex:idx_message_user_dedupe,priority:2" json:"-"`
	Type      string     `gorm:"size:32;not null" json:"type"`
	Title     string     `gorm:"size:200;not null" json:"title"`
	Body      string     `gorm:"size:1000" json:"body"`
	Link      string     `gorm:"size:255" json:"link"`
	ProjectID uint64     `gorm:"index" json:"project_id"`
	AuditID   uint64     `gorm:"index" json:"audit_id"`
	TaskID    uint64     `gorm:"index" json:"task_id"`
	ReadAt    *time.Time `gorm:"index" json:"read_at"`
	CreatedAt time.Time  `gorm:"index:idx_message_user_created,priority:2" json:"created_at"`
}

func (InAppMessage) TableName() string { return "in_app_messages" }
