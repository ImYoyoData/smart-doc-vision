package model

import "time"

// ProviderUsageLog records one model request. Review calls and assistant calls
// are both stored so administrators can audit token consumption per provider.
type ProviderUsageLog struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID    uint64    `gorm:"not null;index" json:"project_id"`
	ProviderID   uint64    `gorm:"not null;index" json:"provider_id"`
	ProviderName string    `gorm:"size:64" json:"provider_name"`
	Kind         string    `gorm:"size:24;index" json:"kind"`
	Model        string    `gorm:"size:128" json:"model"`
	Source       string    `gorm:"size:32;index" json:"source"`
	AuditID      uint64    `gorm:"index" json:"audit_id"`
	TaskID       uint64    `gorm:"index" json:"task_id"`
	InputTokens  int       `gorm:"not null;default:0" json:"input_tokens"`
	CachedTokens int       `gorm:"not null;default:0" json:"cached_tokens"`
	OutputTokens int       `gorm:"not null;default:0" json:"output_tokens"`
	TotalTokens  int       `gorm:"not null;default:0" json:"total_tokens"`
	DurationMS   int64     `gorm:"not null;default:0" json:"duration_ms"`
	Status       string    `gorm:"size:16;not null;default:SUCCESS" json:"status"`
	ErrorMessage string    `gorm:"size:512" json:"error_message,omitempty"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (ProviderUsageLog) TableName() string { return "provider_usage_logs" }

const (
	UsageSourceReview          = "REVIEW"
	UsageSourceScriptAssistant = "SCRIPT_ASSISTANT"
	UsageSourceAuditAssistant  = "AUDIT_ASSISTANT"
	UsageSourceLogsAssistant   = "LOGS_ASSISTANT"
	UsageSourceTreaty          = "TREATY"

	UsageStatusSuccess = "SUCCESS"
	UsageStatusError   = "ERROR"
)
