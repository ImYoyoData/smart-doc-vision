package model

import "time"

type AIProvider struct {
	ID         uint64   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID  uint64   `gorm:"not null;index:idx_provider_project_name,unique" json:"project_id"`
	Name       string   `gorm:"size:64;not null;index:idx_provider_project_name,unique" json:"name"`
	Kind       string   `gorm:"size:24;not null;index" json:"kind"`
	BaseURL    string   `gorm:"size:255;not null" json:"base_url"`
	Model      string   `gorm:"size:128;not null" json:"model"`
	APIKeyEnc  string   `gorm:"type:text" json:"-"`
	Prompt     string   `gorm:"type:text" json:"prompt"`
	ConfigJSON JSONText `gorm:"type:longtext" json:"config"`
	Status     string   `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	// Priority orders failover candidates within the same business category:
	// smaller numbers are tried first. The audit-bound provider always goes
	// first, then the remaining models follow this order.
	Priority    int        `gorm:"not null;default:100" json:"priority"`
	LastTestAt  *time.Time `json:"last_test_at"`
	LastTestOK  bool       `json:"last_test_ok"`
	LastTestMS  int64      `json:"last_test_ms"`
	LastTestMsg string     `gorm:"size:512" json:"last_test_message"`

	// Runtime circuit breaker. Status stays under the administrator's control;
	// these fields only describe automatic failover health.
	BreakerUntil  *time.Time `json:"breaker_until"`
	BreakerReason string     `gorm:"size:512" json:"breaker_reason"`
	FailureCount  int        `gorm:"not null;default:0" json:"failure_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	ProviderOpenAI    = "OPENAI_COMPATIBLE"
	ProviderAnthropic = "ANTHROPIC"
	ProviderJEV       = "JEV"

	ProviderActive   = "ACTIVE"
	ProviderDisabled = "DISABLED"
)

type ProviderConfig struct {
	SupportsVision   bool              `json:"supports_vision"`
	MaxTokens        int               `json:"max_tokens"`
	Temperature      float64           `json:"temperature"`
	MaxImages        int               `json:"max_images"`
	ContextWindow    int               `json:"context_window,omitempty"`
	SupportsThinking bool              `json:"supports_thinking,omitempty"`
	PreferReasoning  bool              `json:"prefer_reasoning,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}

func (AIProvider) TableName() string { return "ai_providers" }
