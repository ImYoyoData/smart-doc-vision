package api

import (
	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

func assistantUsageView(provider *model.AIProvider, result *ai.ChatResult) gin.H {
	if result == nil {
		return gin.H{}
	}
	providerConfig := model.ProviderConfig{}
	_ = model.DecodeJSON(provider.ConfigJSON, &providerConfig)
	contextWindow := providerConfig.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 128000
	}
	return gin.H{
		"model":          result.Model,
		"requests":       1,
		"input_tokens":   result.Usage.InputTokens,
		"cached_tokens":  result.Usage.CachedTokens,
		"output_tokens":  result.Usage.OutputTokens,
		"total_tokens":   result.Usage.TotalTokens,
		"context_tokens": result.Usage.InputTokens + result.Usage.OutputTokens,
		"context_window": contextWindow,
	}
}
