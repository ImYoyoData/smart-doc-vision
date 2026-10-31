package model

import "time"

// SchemaVersion records the database structure version applied by the app.
// The table itself is created automatically; other schema changes require an
// explicit administrator confirmation in the upgrade flow.
type SchemaVersion struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Version    string    `gorm:"size:64;not null;index" json:"version"`
	AppVersion string    `gorm:"size:32" json:"app_version"`
	AppliedAt  time.Time `json:"applied_at"`
}

func (SchemaVersion) TableName() string { return "schema_versions" }
