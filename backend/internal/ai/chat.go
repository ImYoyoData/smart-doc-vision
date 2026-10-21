package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/model"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Provider    model.AIProvider
	APIKey      string
	System      string
	Messages    []ChatMessage
	MaxTokens   int
	Temperature float64
}

type ChatResult struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

// Chat powers the audit-configuration assistant. It intentionally supports
// conversational providers only; JEV System One is a structured decision API.
func Chat(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2400
	}
	if req.MaxTokens > 8000 {
		req.MaxTokens = 8000
	}
	if req.Temperature <= 0 {
		req.Temperature = 0.3
	}
	switch strings.ToUpper(strings.TrimSpace(req.Provider.Kind)) {
	case model.ProviderOpenAI, "OPENAI":
		return chatOpenAI(ctx, req)
	case model.ProviderAnthropic, "CLAUDE":
		return chatAnthropic(ctx, req)
	case model.ProviderJEV, "TYPESAFE", "TYPESAFE_JEV":
		return nil, fmt.Errorf("JEV System One 不支持自由对话，请为 AI 配置助手选择 OpenAI 兼容或 Anthropic 模型")
	default:
		return nil, fmt.Errorf("不支持的模型类型 %q", req.Provider.Kind)
	}
}

func chatOpenAI(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("供应商 API Key 未配置")
	}
	messages := make([]ChatMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, req.Messages...)
	body := map[string]any{
		"model":       req.Provider.Model,
		"messages":    messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint(req.Provider.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, truncateErr(string(raw)))
	}
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("模型未返回内容")
	}
	return &ChatResult{Content: strings.TrimSpace(parsed.Choices[0].Message.Content), Model: parsed.Model}, nil
}

func chatAnthropic(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("Anthropic API Key 未配置")
	}
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := message.Role
		if role != "assistant" {
			role = "user"
		}
		messages = append(messages, map[string]any{
			"role":    role,
			"content": []contentBlock{{Type: "text", Text: message.Content}},
		})
	}
	body := map[string]any{
		"model":       req.Provider.Model,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"messages":    messages,
	}
	if strings.TrimSpace(req.System) != "" {
		body["system"] = req.System
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicEndpoint(req.Provider.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, truncateErr(string(raw)))
	}
	var parsed struct {
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	var builder strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			builder.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(builder.String()) == "" {
		return nil, fmt.Errorf("模型未返回内容")
	}
	return &ChatResult{Content: strings.TrimSpace(builder.String()), Model: parsed.Model}, nil
}

func truncateErr(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return value
}
