package model

import "time"

type AuditLog struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID  *uint64   `gorm:"index" json:"project_id,omitempty"`
	OperatorID *uint64   `json:"operator_id,omitempty"`
	Operator   string    `gorm:"size:64;not null" json:"operator"`
	OperatorIP string    `gorm:"size:64" json:"operator_ip"`
	Action     string    `gorm:"size:64;not null;index" json:"action"`
	ObjectType string    `gorm:"size:32;index" json:"object_type"`
	ObjectID   string    `gorm:"size:64;index" json:"object_id"`
	Result     string    `gorm:"size:16;not null;default:SUCCESS" json:"result"`
	DetailJSON JSONText  `gorm:"type:text" json:"detail"`
	TraceID    string    `gorm:"size:64;index" json:"trace_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }
