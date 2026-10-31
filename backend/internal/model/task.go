package model

import "time"

type Task struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskNo           string     `gorm:"size:40;not null;uniqueIndex" json:"task_no"`
	ProjectID        uint64     `gorm:"not null;index;index:idx_task_project_created;uniqueIndex:idx_project_dedupe,priority:1;uniqueIndex:idx_project_request,priority:1" json:"project_id"`
	AuditID          uint64     `gorm:"not null;index" json:"audit_id"`
	SourceType       string     `gorm:"size:16;not null" json:"source_type"`
	SourceID         uint64     `gorm:"not null;default:0" json:"source_id"`
	BizKey           string     `gorm:"size:128;index" json:"biz_key"`
	RequestID        *string    `gorm:"size:128;index;uniqueIndex:idx_project_request,priority:2" json:"request_id,omitempty"`
	RequestHash      string     `gorm:"size:64" json:"-"`
	DedupeHash       string     `gorm:"size:64;index;uniqueIndex:idx_project_dedupe,priority:2" json:"dedupe_hash,omitempty"`
	MediaType        string     `gorm:"size:16;not null" json:"media_type"`
	Status           string     `gorm:"size:24;not null;index" json:"status"`
	Result           string     `gorm:"size:16;not null;default:UNKNOWN;index" json:"result"`
	InputJSON        JSONText   `json:"input"`
	ModelResult      JSONText   `json:"model_result"`
	RuleTrace        JSONText   `json:"rule_trace"`
	ResultJSON       JSONText   `json:"result_detail"`
	ProviderID       uint64     `json:"provider_id"`
	ProviderName     string     `gorm:"-" json:"provider_name,omitempty"`
	AuditName        string     `gorm:"-" json:"audit_name,omitempty"`
	ProjectName      string     `gorm:"-" json:"project_name,omitempty"`
	ErrorMessage     string     `gorm:"size:512" json:"error_message"`
	RetryCount       int        `gorm:"not null;default:0" json:"retry_count"`
	DurationMS       int64      `json:"duration_ms"`
	ReviewerID       *uint64    `json:"reviewer_id,omitempty"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	ReviewNote       string     `gorm:"size:512" json:"review_note,omitempty"`
	ReviewReasonCode string     `gorm:"size:64" json:"review_reason_code,omitempty"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

const (
	TaskQueued        = "QUEUED"
	TaskProcessing    = "PROCESSING"
	TaskReviewPending = "REVIEW_PENDING"
	TaskSuccess       = "SUCCESS"
	TaskFailed        = "FAILED"
	TaskDuplicate     = "DUPLICATED"
	TaskStopped       = "STOPPED"

	ResultPass    = "PASS"
	ResultReject  = "REJECT"
	ResultReview  = "REVIEW"
	ResultUnknown = "UNKNOWN"
)

type TaskEvent struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID     uint64    `gorm:"not null;index" json:"task_id"`
	ProjectID  uint64    `gorm:"not null;index" json:"project_id"`
	FromStatus string    `gorm:"size:24" json:"from_status"`
	ToStatus   string    `gorm:"size:24" json:"to_status"`
	Message    string    `gorm:"size:512" json:"message"`
	CostMS     int64     `json:"cost_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

type MediaItem struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
	Name    string `json:"name,omitempty"`
	// Key identifies an object in the configured file storage. New uploads
	// store the image outside the database; URL is filled only for display.
	Key     string `json:"key,omitempty"`
	Storage string `json:"storage,omitempty"`
	MIME    string `json:"mime,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

type TaskInput struct {
	BizKey      string         `json:"biz_key"`
	RequestID   string         `json:"request_id,omitempty"`
	SourceRef   string         `json:"source_ref,omitempty"`
	Media       []MediaItem    `json:"media"`
	Ext         map[string]any `json:"ext"`
	CallbackURL string         `json:"callback_url,omitempty"`
}

type Verdict struct {
	Result             string         `json:"result"`
	RiskLevel          string         `json:"risk_level"`
	Score              float64        `json:"score"`
	Labels             []string       `json:"labels"`
	Summary            string         `json:"summary"`
	Reasons            []string       `json:"reasons,omitempty"`
	Category           string         `json:"category,omitempty"`
	CategoryName       string         `json:"category_name,omitempty"`
	CategoryConfidence float64        `json:"category_confidence,omitempty"`
	Raw                any            `json:"raw,omitempty"`
	Provider           string         `json:"provider"`
	Model              string         `json:"model"`
	LatencyMS          int64          `json:"latency_ms"`
	InputTokens        int            `json:"input_tokens,omitempty"`
	CachedTokens       int            `json:"cached_tokens,omitempty"`
	OutputTokens       int            `json:"output_tokens,omitempty"`
	TotalTokens        int            `json:"total_tokens,omitempty"`
	Meta               map[string]any `json:"meta,omitempty"`
}

func (Task) TableName() string { return "tasks" }

func (TaskEvent) TableName() string { return "task_events" }
