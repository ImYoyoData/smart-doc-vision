package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/model"
)

type providerModelsInput struct {
	ProjectID  uint64            `json:"project_id"`
	ProviderID uint64            `json:"provider_id"`
	Kind       string            `json:"kind"`
	BaseURL    string            `json:"base_url"`
	APIKey     string            `json:"api_key"`
	Headers    map[string]string `json:"headers"`
}

type providerModelItem struct {
	ID                 string `json:"id"`
	Name               string `json:"name,omitempty"`
	ContextWindow      int    `json:"context_window"`
	ContextWindowKnown bool   `json:"context_window_known"`
	MaxOutputTokens    int    `json:"max_output_tokens"`
	MaxOutputKnown     bool   `json:"max_output_known"`
	SupportsVision     bool   `json:"supports_vision"`
	VisionKnown        bool   `json:"vision_known"`
	SupportsThinking   bool   `json:"supports_thinking"`
	ThinkingKnown      bool   `json:"thinking_known"`
	Source             string `json:"source"`
}

// listProviderModels asks the upstream provider for its model catalog and
// enriches each entry with context/output/vision/thinking capabilities so the
// admin page can pick a model instead of typing an ID from memory.
func (s *Server) listProviderModels(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "provider:write") {
		forbidden(c, "当前角色无权读取模型列表")
		return
	}
	var req providerModelsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if req.ProjectID == 0 {
		badRequest(c, "请选择项目")
		return
	}
	if !s.requireProjectAccess(c, req.ProjectID) {
		return
	}

	kind := normalizeProviderKind(req.Kind)
	baseURL := strings.TrimSpace(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	headers := req.Headers
	if req.ProviderID > 0 {
		var provider model.AIProvider
		if err := s.db.Where("id = ? AND project_id = ?", req.ProviderID, req.ProjectID).First(&provider).Error; err != nil {
			notFound(c, "模型接入不存在或不属于当前项目")
			return
		}
		if kind == "" {
			kind = provider.Kind
		}
		if baseURL == "" {
			baseURL = provider.BaseURL
		}
		if apiKey == "" {
			apiKey, _ = s.cipher.Decrypt(provider.APIKeyEnc)
		}
		if len(headers) == 0 {
			var cfg model.ProviderConfig
			_ = model.DecodeJSON(provider.ConfigJSON, &cfg)
			headers = cfg.Headers
		}
	}
	if kind == model.ProviderJEV {
		badRequest(c, "JEV 决策模型使用固定接口，不支持模型列表")
		return
	}
	if kind != model.ProviderOpenAI && kind != model.ProviderAnthropic {
		badRequest(c, "只有通用大模型支持获取模型列表")
		return
	}
	if baseURL == "" {
		baseURL, _ = catalog.ProviderDefaults(kind)
	}
	if err := validateBaseURL(baseURL); err != nil {
		badRequest(c, err.Error())
		return
	}
	if apiKey == "" {
		badRequest(c, "请先填写 API Key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	items, err := fetchProviderModels(ctx, kind, baseURL, apiKey, headers)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "MODEL_LIST_FAILED", err.Error(), nil)
		return
	}
	ok(c, gin.H{"items": items, "total": len(items)})
}

func fetchProviderModels(ctx context.Context, kind, baseURL, apiKey string, headers map[string]string) ([]providerModelItem, error) {
	endpoint := providerModelsEndpoint(baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	if kind == model.ProviderAnthropic {
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	ai.ApplyCustomHeaders(httpReq, headers)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求模型列表失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型列表返回 HTTP %d: %s", resp.StatusCode, truncateText(string(raw), 400))
	}
	var payload struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	rows := payload.Data
	if len(rows) == 0 {
		rows = payload.Models
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("供应商没有返回可用模型")
	}
	items := make([]providerModelItem, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		item := parseProviderModel(row)
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("供应商没有返回可用模型")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func providerModelsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	base = strings.TrimSuffix(base, "/messages")
	if strings.HasSuffix(base, "/models") {
		return base
	}
	return base + "/models"
}

func parseProviderModel(row map[string]any) providerModelItem {
	id := firstStringValue(row, "id", "model", "name")
	item := providerModelItem{
		ID: id, Name: firstStringValue(row, "display_name", "displayName", "name"), Source: "unknown",
	}
	if item.Name == item.ID {
		item.Name = ""
	}
	item.ContextWindow = firstIntValue(row,
		"context_length", "context_window", "max_context_length", "max_context_tokens", "context_size")
	item.ContextWindowKnown = item.ContextWindow > 0
	item.MaxOutputTokens = firstIntValue(row,
		"max_output_tokens", "max_completion_tokens", "output_token_limit", "max_tokens")
	item.MaxOutputKnown = item.MaxOutputTokens > 0

	if vision, found := lookupBoolValue(row, "supports_vision", "vision", "supports_images"); found {
		item.SupportsVision = vision
		item.VisionKnown = true
	}
	if thinking, found := lookupBoolValue(row, "supports_thinking", "supports_reasoning", "reasoning", "thinking"); found {
		item.SupportsThinking = thinking
		item.ThinkingKnown = true
	}
	if architecture, ok := row["architecture"].(map[string]any); ok {
		if modalities, ok := architecture["input_modalities"].([]any); ok {
			item.VisionKnown = true
			for _, modality := range modalities {
				if strings.EqualFold(fmt.Sprint(modality), "image") {
					item.SupportsVision = true
				}
			}
		}
	}
	if top, ok := row["top_provider"].(map[string]any); ok {
		if !item.ContextWindowKnown {
			if value := firstIntValue(top, "context_length", "context_window"); value > 0 {
				item.ContextWindow = value
				item.ContextWindowKnown = true
			}
		}
		if !item.MaxOutputKnown {
			if value := firstIntValue(top, "max_completion_tokens", "max_output_tokens"); value > 0 {
				item.MaxOutputTokens = value
				item.MaxOutputKnown = true
			}
		}
	}
	if params, ok := row["supported_parameters"].([]any); ok {
		item.ThinkingKnown = true
		for _, param := range params {
			switch strings.ToLower(fmt.Sprint(param)) {
			case "reasoning", "include_reasoning", "reasoning_effort", "thinking":
				item.SupportsThinking = true
			}
		}
	}
	if capabilities, ok := row["capabilities"].(map[string]any); ok {
		if vision, found := lookupBoolValue(capabilities, "vision", "image_input"); found {
			item.VisionKnown = true
			item.SupportsVision = item.SupportsVision || vision
		}
		if thinking, found := lookupBoolValue(capabilities, "thinking", "reasoning"); found {
			item.ThinkingKnown = true
			item.SupportsThinking = item.SupportsThinking || thinking
		}
	}
	if item.ContextWindowKnown || item.MaxOutputKnown || item.VisionKnown || item.ThinkingKnown {
		item.Source = "metadata"
	}
	return item
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func firstIntValue(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return int(typed)
			}
		case json.Number:
			if number, err := typed.Int64(); err == nil && number > 0 {
				return int(number)
			}
		case int:
			if typed > 0 {
				return typed
			}
		case string:
			if number, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && number > 0 {
				return number
			}
		}
	}
	return 0
}

func lookupBoolValue(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			return strings.EqualFold(typed, "true") || typed == "1" || strings.EqualFold(typed, "yes"), true
		case float64:
			return typed != 0, true
		}
	}
	return false, false
}

func normalizeProviderKind(kind string) string {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "OPENAI", model.ProviderOpenAI:
		return model.ProviderOpenAI
	case "CLAUDE", model.ProviderAnthropic:
		return model.ProviderAnthropic
	case "JEV", "TYPESAFE", "TYPESAFE_JEV":
		return model.ProviderJEV
	default:
		return strings.ToUpper(strings.TrimSpace(kind))
	}
}
