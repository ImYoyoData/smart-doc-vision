package model

import "time"

// RedisConnection is a project-scoped Redis instance. The DB index is chosen
// per trigger/result action, so one connection can serve several logical queues.
type RedisConnection struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID   uint64 `gorm:"not null;index;uniqueIndex:idx_redis_conn_project_name,priority:1" json:"project_id"`
	Name        string `gorm:"size:64;not null;uniqueIndex:idx_redis_conn_project_name,priority:2" json:"name"`
	Addr        string `gorm:"size:255;not null" json:"addr"`
	Username    string `gorm:"size:128" json:"username"`
	PasswordEnc string `gorm:"type:text" json:"-"`
	TLS         bool   `gorm:"not null;default:false" json:"tls"`
	Status      string `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	Health      string `gorm:"size:16;not null;default:UNKNOWN" json:"health"`

	AutoResumeTriggers bool       `gorm:"not null;default:true" json:"auto_resume_triggers"`
	LastTestAt         *time.Time `json:"last_test_at"`
	LastTestOK         bool       `json:"last_test_ok"`
	LastTestMS         int64      `json:"last_test_ms"`
	LastTestMsg        string     `gorm:"size:512" json:"last_test_message"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (RedisConnection) TableName() string { return "redis_connections" }

const (
	RedisConnectionActive   = "ACTIVE"
	RedisConnectionDisabled = "DISABLED"

	RedisHealthUnknown   = "UNKNOWN"
	RedisHealthOK        = "OK"
	RedisHealthUnhealthy = "UNHEALTHY"

	TriggerPauseManual              = "MANUAL"
	TriggerPauseConnectionDisabled  = "CONNECTION_DISABLED"
	TriggerPauseConnectionUnhealthy = "CONNECTION_UNHEALTHY"
	TriggerPauseFailureLimit        = "FAILURE_LIMIT"
)
