package ai

import "testing"

func TestOpenAIUsagePrefersDetails(t *testing.T) {
	raw := openAIUsage{
		PromptTokens: 1200, CompletionTokens: 300, TotalTokens: 1500,
		PromptCacheHitTokens: 100,
	}
	raw.PromptTokensDetails.CachedTokens = 800
	got := raw.usage()
	if got.InputTokens != 1200 || got.CachedTokens != 800 || got.OutputTokens != 300 || got.TotalTokens != 1500 {
		t.Fatalf("unexpected usage: %+v", got)
	}
}

func TestOpenAIUsageFallsBackToTopLevelCache(t *testing.T) {
	got := openAIUsage{PromptTokens: 500, CompletionTokens: 50, PromptCacheHitTokens: 120}.usage()
	if got.CachedTokens != 120 {
		t.Fatalf("cached = %d, want 120", got.CachedTokens)
	}
	if got.TotalTokens != 550 {
		t.Fatalf("total = %d, want 550", got.TotalTokens)
	}
}

func TestAnthropicUsageCacheRead(t *testing.T) {
	got := anthropicUsage{InputTokens: 2000, OutputTokens: 400, CacheReadInputTokens: 1500}.usage()
	if got.InputTokens != 2000 || got.CachedTokens != 1500 || got.OutputTokens != 400 {
		t.Fatalf("unexpected usage: %+v", got)
	}
	if got.TotalTokens != 2400 {
		t.Fatalf("total = %d, want 2400", got.TotalTokens)
	}
}

func TestJEVUsageFromMap(t *testing.T) {
	got := usageFromMap(map[string]any{"input_tokens": float64(320), "output_tokens": float64(0)})
	if got.InputTokens != 320 || got.OutputTokens != 0 || got.TotalTokens != 320 {
		t.Fatalf("unexpected usage: %+v", got)
	}
}
