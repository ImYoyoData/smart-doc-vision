package api

import (
	"time"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/usage"
)

func (s *Server) recordProviderUsage(
	provider model.AIProvider,
	source string,
	auditID uint64,
	result *ai.ChatResult,
	duration time.Duration,
	callErr error,
) {
	entry := usage.Entry{
		ProjectID:    provider.ProjectID,
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		Kind:         provider.Kind,
		Model:        provider.Model,
		Source:       source,
		AuditID:      auditID,
		DurationMS:   duration.Milliseconds(),
	}
	if callErr != nil {
		entry.Status = model.UsageStatusError
		entry.ErrorMessage = callErr.Error()
	} else if result != nil {
		if result.Model != "" {
			entry.Model = result.Model
		}
		entry.InputTokens = result.Usage.InputTokens
		entry.CachedTokens = result.Usage.CachedTokens
		entry.OutputTokens = result.Usage.OutputTokens
		entry.TotalTokens = result.Usage.TotalTokens
	}
	usage.Record(s.db, entry)
}
