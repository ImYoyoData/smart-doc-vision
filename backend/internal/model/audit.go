package model

import "time"

type Audit struct {
	ID            uint64   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID     uint64   `gorm:"not null;index;index:idx_audit_project_code,unique" json:"project_id"`
	Name          string   `gorm:"size:64;not null" json:"name"`
	Code          string   `gorm:"size:32;not null;index:idx_audit_project_code,unique" json:"code"`
	Description   string   `gorm:"size:255" json:"description"`
	MediaTypes    JSONText `gorm:"type:text" json:"media_types"`
	DecisionMode  string   `gorm:"size:16;not null;default:REVIEW" json:"decision_mode"`
	Categories    JSONText `json:"categories"`
	ProviderID    uint64   `gorm:"not null;index" json:"provider_id"`
	ReviewPrompt  string   `gorm:"type:text" json:"review_prompt"`
	Treaties      JSONText `json:"treaties"`
	Rules         JSONText `json:"rules"`
	Routes        JSONText `json:"routes"`
	DefaultResult string   `gorm:"size:16;not null;default:REVIEW" json:"default_result"`
	OnModelFail   string   `gorm:"size:16;not null;default:REVIEW" json:"on_model_fail"`
	// ReviewMode decides where a REVIEW verdict waits for humans: INTERNAL
	// queues the task in this platform's review workbench; EXTERNAL finishes
	// the task immediately and only notifies the business system via result
	// routes / callbacks, which runs its own manual review.
	ReviewMode             string     `gorm:"size:16;not null;default:INTERNAL" json:"review_mode"`
	DedupeWindowSec        int        `gorm:"not null" json:"dedupe_window_sec"`
	CallbackEnabled        bool       `gorm:"not null;default:false" json:"callback_enabled"`
	CallbackURL            string     `gorm:"size:1024" json:"callback_url"`
	CallbackResults        JSONText   `gorm:"type:text" json:"callback_results"`
	CallbackHeaders        JSONText   `gorm:"type:text" json:"callback_headers"`
	CallbackIncludeContent bool       `gorm:"not null" json:"callback_include_content"`
	CallbackSecretEnc      string     `gorm:"type:text" json:"-"`
	CallbackSecretSet      bool       `gorm:"-" json:"callback_secret_set"`
	Status                 string     `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	Version                int        `gorm:"not null;default:1" json:"version"`
	DeletedAt              *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	DeletedWithProject     bool       `gorm:"not null;default:false" json:"-"`
	CreatedBy              uint64     `json:"created_by"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`

	ProjectName    string   `gorm:"-" json:"project_name,omitempty"`
	ActiveTriggers []string `gorm:"-" json:"active_triggers,omitempty"`
	ProviderName   string   `gorm:"-" json:"provider_name,omitempty"`
	ProviderKind   string   `gorm:"-" json:"provider_kind,omitempty"`
	TodayTasks     int64    `gorm:"-" json:"today_tasks"`
}

const (
	AuditDraft   = "DRAFT"
	AuditActive  = "ACTIVE"
	AuditPaused  = "PAUSED"
	AuditDeleted = "DELETED"
)

const (
	DecisionReview       = "REVIEW"
	DecisionClassify     = "CLASSIFY"
	DecisionClassifyOnly = "CLASSIFY_ONLY"
)

const (
	// ReviewModeInternal queues REVIEW tasks in this platform's workbench.
	ReviewModeInternal = "INTERNAL"
	// ReviewModeExternal finishes REVIEW tasks and defers manual review to the
	// business system notified via result routes / callbacks.
	ReviewModeExternal = "EXTERNAL"
)

// AuditCategory maps one model-selected category to a final audit decision.
// It is stored on the audit so classification is deterministic and auditable.
type AuditCategory struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Result      string `json:"result"`
}

const (
	TreatySourceFile = "FILE"
	TreatySourceText = "TEXT"

	TreatyStatusPARSING = "PARSING"
	TreatyStatusReady   = "READY"
	TreatyStatusFailed  = "FAILED"
)

// AuditTreaty is one parsed document of the review treaty. Only READY
// documents with compiled content are injected into model prompts.
type AuditTreaty struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	SourceType    string    `json:"source_type"`
	FileName      string    `json:"file_name,omitempty"`
	MimeType      string    `json:"mime_type,omitempty"`
	Status        string    `json:"status"`
	Content       string    `json:"content"`
	OriginalChars int       `json:"original_chars"`
	ContentChars  int       `json:"content_chars"`
	Summary       string    `json:"summary,omitempty"`
	ParseMS       int64     `json:"parse_ms,omitempty"`
	Error         string    `json:"error,omitempty"`
	Retryable     bool      `json:"retryable,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type TriggerSource struct {
	ID         uint64   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID  uint64   `gorm:"not null;index" json:"project_id"`
	AuditID    uint64   `gorm:"not null;index;uniqueIndex:idx_trigger_audit_type,priority:1" json:"audit_id"`
	Type       string   `gorm:"size:16;not null;uniqueIndex:idx_trigger_audit_type,priority:2" json:"type"`
	Name       string   `gorm:"size:64;not null" json:"name"`
	ConfigJSON JSONText `gorm:"type:text" json:"config"`
	Priority   int      `gorm:"not null;default:40" json:"priority"`
	Status     string   `gorm:"size:16;not null;default:ACTIVE" json:"status"`

	RedisConnectionID *uint64 `gorm:"index;uniqueIndex:idx_redis_stream_slot,priority:1" json:"redis_connection_id,omitempty"`
	RedisDB           int     `gorm:"uniqueIndex:idx_redis_stream_slot,priority:2" json:"redis_db"`
	Stream            string  `gorm:"size:128;uniqueIndex:idx_redis_stream_slot,priority:3" json:"stream"`
	ConsumerGroup     string  `gorm:"size:128" json:"consumer_group"`
	StartPosition     string  `gorm:"size:16;default:$" json:"start_position"`
	BatchSize         int     `gorm:"not null;default:16" json:"batch_size"`
	MaxInflight       int     `gorm:"not null;default:1" json:"max_inflight"`
	PauseReason       string  `gorm:"size:32" json:"pause_reason"`
	AutoPaused        bool    `gorm:"not null;default:false" json:"auto_paused"`

	SecretEnc     string     `gorm:"type:text" json:"-"`
	SecretSet     bool       `gorm:"-" json:"secret_set"`
	CursorValue   string     `gorm:"type:text" json:"cursor"`
	LastConsumeAt *time.Time `json:"last_consume_at"`
	LastRunAt     *time.Time `json:"last_run_at"`
	LastError     string     `gorm:"size:512" json:"last_error"`
	FailCount     int        `gorm:"not null;default:0" json:"fail_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

const (
	TriggerOpenAPI   = "OPENAPI"
	TriggerRedis     = "REDIS"
	TriggerWebSocket = "WEBSOCKET"
)

func (Audit) TableName() string { return "audits" }

func (TriggerSource) TableName() string { return "trigger_sources" }
