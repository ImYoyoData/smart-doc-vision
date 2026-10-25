package usage

import (
	"log"
	"time"

	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

// Entry is one model request to persist. Tokens may be zero when the provider
// does not return usage data; the request is still counted.
type Entry struct {
	ProjectID    uint64
	ProviderID   uint64
	ProviderName string
	Kind         string
	Model        string
	Source       string
	AuditID      uint64
	TaskID       uint64
	InputTokens  int
	CachedTokens int
	OutputTokens int
	TotalTokens  int
	DurationMS   int64
	Status       string
	ErrorMessage string
}

// Summary aggregates a provider's history for the model card.
type Summary struct {
	Requests     int64      `json:"requests"`
	InputTokens  int64      `json:"input_tokens"`
	CachedTokens int64      `json:"cached_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	TotalTokens  int64      `json:"total_tokens"`
	LastUsedAt   *time.Time `json:"last_used_at"`
}

type summaryRow struct {
	ProviderID   uint64
	Requests     int64
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
	TotalTokens  int64
	LastUsedAt   *time.Time
}

func Record(db *gorm.DB, entry Entry) {
	if db == nil || entry.ProviderID == 0 {
		return
	}
	status := entry.Status
	if status == "" {
		status = model.UsageStatusSuccess
	}
	total := entry.TotalTokens
	if total == 0 {
		total = entry.InputTokens + entry.OutputTokens
	}
	row := model.ProviderUsageLog{
		ProjectID:    entry.ProjectID,
		ProviderID:   entry.ProviderID,
		ProviderName: entry.ProviderName,
		Kind:         entry.Kind,
		Model:        entry.Model,
		Source:       entry.Source,
		AuditID:      entry.AuditID,
		TaskID:       entry.TaskID,
		InputTokens:  entry.InputTokens,
		CachedTokens: entry.CachedTokens,
		OutputTokens: entry.OutputTokens,
		TotalTokens:  total,
		DurationMS:   entry.DurationMS,
		Status:       status,
		ErrorMessage: truncate(entry.ErrorMessage, 500),
		CreatedAt:    time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		log.Printf("record provider usage: %v", err)
	}
}

// Summaries returns totals keyed by provider ID. Providers without history are
// absent from the map.
func Summaries(db *gorm.DB, providerIDs []uint64) (map[uint64]Summary, error) {
	out := map[uint64]Summary{}
	if db == nil || len(providerIDs) == 0 {
		return out, nil
	}
	var rows []summaryRow
	err := db.Model(&model.ProviderUsageLog{}).
		Select("provider_id, COUNT(*) AS requests, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(cached_tokens),0) AS cached_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, MAX(created_at) AS last_used_at").
		Where("provider_id IN ?", providerIDs).
		Group("provider_id").
		Scan(&rows).Error
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out[row.ProviderID] = Summary{
			Requests:     row.Requests,
			InputTokens:  row.InputTokens,
			CachedTokens: row.CachedTokens,
			OutputTokens: row.OutputTokens,
			TotalTokens:  row.TotalTokens,
			LastUsedAt:   row.LastUsedAt,
		}
	}
	return out, nil
}

func Recent(db *gorm.DB, providerID uint64, limit int) ([]model.ProviderUsageLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []model.ProviderUsageLog
	err := db.Where("provider_id = ?", providerID).
		Order("id desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
