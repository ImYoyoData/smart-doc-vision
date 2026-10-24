package model

import "time"

type Project struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string `gorm:"size:64;not null" json:"name"`
	Code             string `gorm:"size:32;not null;uniqueIndex" json:"code"`
	Env              string `gorm:"size:16;not null;default:prod" json:"env"`
	Status           string `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	Description      string `gorm:"size:255" json:"description"`
	OwnerID          uint64 `gorm:"index" json:"owner_id"`
	AppKey           string `gorm:"size:48;uniqueIndex" json:"-"`
	AppSecretEnc     string `gorm:"type:text" json:"-"`
	RetentionDays    int    `gorm:"not null;default:30" json:"retention_days"`
	MaxTasksPerDay   int    `gorm:"not null;default:0" json:"max_tasks_per_day"`
	MaxConcurrent    int    `gorm:"not null;default:2" json:"max_concurrent_tasks"`
	OpenAPIRateLimit int    `gorm:"not null;default:120" json:"openapi_rate_limit"`

	JSMaxMemoryMB          int `gorm:"not null;default:64" json:"js_max_memory_mb"`
	JSTimeoutMS            int `gorm:"not null;default:5000" json:"js_timeout_ms"`
	JSMaxLogBytes          int `gorm:"not null;default:32768" json:"js_max_log_bytes"`
	JSMaxResultBytes       int `gorm:"not null;default:65536" json:"js_max_result_bytes"`
	JSMaxHTTPRequests      int `gorm:"not null;default:20" json:"js_max_http_requests"`
	JSMaxHTTPResponseBytes int `gorm:"not null;default:2097152" json:"js_max_http_response_bytes"`
	JSMaxDBRows            int `gorm:"not null;default:1000" json:"js_max_db_rows"`
	JSMaxConcurrency       int `gorm:"not null;default:2" json:"js_max_concurrency"`

	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	CreatedBy uint64     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	AuditCount    int64 `gorm:"-" json:"audit_count"`
	ProviderCount int64 `gorm:"-" json:"provider_count"`
	TodayTasks    int64 `gorm:"-" json:"today_tasks"`
}

const (
	ProjectActive   = "ACTIVE"
	ProjectPaused   = "PAUSED"
	ProjectDisabled = "DISABLED"
	ProjectDeleted  = "DELETED"
)

func (Project) TableName() string { return "projects" }
