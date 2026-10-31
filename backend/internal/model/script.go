package model

import "time"

// ProjectSecret is a project-scoped value exposed to scripts through
// ctx.secret(name). Values are encrypted with the platform cipher and never
// returned by list APIs.
type ProjectSecret struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID uint64    `gorm:"not null;index;uniqueIndex:idx_project_secret_name,priority:1" json:"project_id"`
	Name      string    `gorm:"size:64;not null;uniqueIndex:idx_project_secret_name,priority:2" json:"name"`
	ValueEnc  string    `gorm:"type:text;not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProjectSecret) TableName() string { return "project_secrets" }

const (
	ScriptSuccess   = "SUCCESS"
	ScriptError     = "SCRIPT_ERROR"
	ScriptTimeout   = "TIMEOUT"
	ScriptOOM       = "OOM"
	ScriptHostError = "HOST_ERROR"
)

// ScriptExecution is one run of a JS result action. The full log is kept per
// audit config so operators can inspect failures without reading task JSON.
type ScriptExecution struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID    uint64    `gorm:"not null;index" json:"project_id"`
	AuditID      uint64    `gorm:"not null;index" json:"audit_id"`
	TaskID       uint64    `gorm:"not null;index" json:"task_id"`
	TaskNo       string    `gorm:"size:40;index" json:"task_no"`
	ActionIndex  int       `json:"action_index"`
	Remark       string    `gorm:"size:128" json:"remark"`
	ScriptHash   string    `gorm:"size:64" json:"script_hash"`
	StartedAt    time.Time `gorm:"index" json:"started_at"`
	DurationMS   int64     `json:"duration_ms"`
	Status       string    `gorm:"size:16;not null;index" json:"status"`
	ErrorType    string    `gorm:"size:32" json:"error_type"`
	ErrorMessage string    `gorm:"size:1024" json:"error_message"`
	LogText      LongText  `json:"log_text"`
	MemoryBytes  int64     `json:"memory_bytes"`
	ResultJSON   JSONText  `json:"result_json"`
	CreatedAt    time.Time `json:"created_at"`
}

func (ScriptExecution) TableName() string { return "script_executions" }
