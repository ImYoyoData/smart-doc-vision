package ai

import (
	"encoding/json"

	"smartdoc-vision/backend/internal/model"
)

// Usage is the token accounting returned by a model gateway. CachedTokens is
// the portion of InputTokens served from the provider's prompt cache.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	CachedTokens int `json:"cached_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (u Usage) normalized() Usage {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	return u
}

type openAIUsage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) usage() Usage {
	cached := u.PromptTokensDetails.CachedTokens
	if cached == 0 {
		// DeepSeek and some proxies expose cache hits under a top-level field.
		cached = u.PromptCacheHitTokens
	}
	return Usage{
		InputTokens:  u.PromptTokens,
		CachedTokens: cached,
		OutputTokens: u.CompletionTokens,
		TotalTokens:  u.TotalTokens,
	}.normalized()
}

type anthropicUsage struct {
	InputTokens          int `json:"input_tokens"`
	OutputTokens         int `json:"output_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) usage() Usage {
	return Usage{
		InputTokens:  u.InputTokens,
		CachedTokens: u.CacheReadInputTokens,
		OutputTokens: u.OutputTokens,
	}.normalized()
}

// usageFromMap reads the loosely typed usage object returned by JEV.
func usageFromMap(raw map[string]any) Usage {
	if len(raw) == 0 {
		return Usage{}
	}
	return Usage{
		InputTokens:  intFromAny(raw["input_tokens"]),
		CachedTokens: intFromAny(raw["cached_tokens"]),
		OutputTokens: intFromAny(raw["output_tokens"]),
		TotalTokens:  intFromAny(raw["total_tokens"]),
	}.normalized()
}

// applyUsageToVerdict copies token accounting onto a review verdict so the
// engine can persist and log it.
func applyUsageToVerdict(verdict *model.Verdict, u Usage) {
	if verdict == nil {
		return
	}
	verdict.InputTokens = u.InputTokens
	verdict.CachedTokens = u.CachedTokens
	verdict.OutputTokens = u.OutputTokens
	verdict.TotalTokens = u.TotalTokens
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}
