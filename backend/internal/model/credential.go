package model

import "time"

// APICredential is a project-scoped machine credential. Each credential is
// explicitly authorized for a subset of audit definitions; deleting or
// disabling an audit therefore also removes the ability to reach it.
type APICredential struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID  uint64     `gorm:"not null;index;uniqueIndex:idx_credential_project_name,priority:1" json:"project_id"`
	Name       string     `gorm:"size:64;not null;uniqueIndex:idx_credential_project_name,priority:2" json:"name"`
	AppKey     string     `gorm:"size:48;not null;uniqueIndex" json:"app_key"`
	SecretEnc  string     `gorm:"type:text" json:"-"`
	AuditIDs   JSONText   `gorm:"type:text" json:"audit_ids"`
	Status     string     `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedBy  uint64     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (APICredential) TableName() string { return "api_credentials" }

const (
	CredentialActive   = "ACTIVE"
	CredentialDisabled = "DISABLED"
)
