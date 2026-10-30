package model

import "time"

const (
	NotificationTypeEmail = "EMAIL"

	NotificationStatusActive   = "ACTIVE"
	NotificationStatusDisabled = "DISABLED"
)

// NotificationChannel is a project-scoped outbound notification account.
// Type keeps the table open for future channels (SMS, webhook, ...), while
// ConfigEnc stores the decrypted-into-memory JSON for the concrete type.
type NotificationChannel struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID       uint64     `gorm:"not null;index:idx_notification_project_name,unique" json:"project_id"`
	Name            string     `gorm:"size:64;not null;index:idx_notification_project_name,unique" json:"name"`
	Type            string     `gorm:"size:16;not null" json:"type"`
	Provider        string     `gorm:"size:32" json:"provider"`
	ConfigEnc       string     `gorm:"type:text" json:"-"`
	Status          string     `gorm:"size:16;not null;default:DISABLED" json:"status"`
	LastTestAt      *time.Time `json:"last_test_at"`
	LastTestOK      bool       `json:"last_test_ok"`
	LastTestMessage string     `gorm:"size:500" json:"last_test_message"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (NotificationChannel) TableName() string { return "notification_channels" }
