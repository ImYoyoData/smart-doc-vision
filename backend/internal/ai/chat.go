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

type ChatImage struct {
	DataURL string
}

type ChatMessage struct {
	Role    string      `json:"role"`
	Content string      `json:"content"`
	Images  []ChatImage `json:"-"`
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
	if req.MaxTokens > MaxOutputTokensLimit {
		req.MaxTokens = MaxOutputTokensLimit
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
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, message := range req.Messages {
		messages = append(messages, openAIChatMessage(message))
	}
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
	ApplyCustomHeaders(httpReq, ParseConfig(req.Provider).Headers)
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
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := message.Role
		if role != "assistant" {
			role = "user"
		}
		blocks := make([]any, 0, len(message.Images)+1)
		if strings.TrimSpace(message.Content) != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": message.Content})
		}
		for _, image := range message.Images {
			mediaType, data, ok := ParseImageDataURL(image.DataURL)
			if !ok {
				return nil, fmt.Errorf("图片格式不支持或超过 8MB，请粘贴 PNG/JPEG/WebP/GIF 图片")
			}
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type": "base64", "media_type": mediaType, "data": data,
				},
			})
		}
		if len(blocks) == 0 {
			continue
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
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
	ApplyCustomHeaders(httpReq, ParseConfig(req.Provider).Headers)
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

func openAIChatMessage(message ChatMessage) map[string]any {
	if len(message.Images) == 0 {
		return map[string]any{"role": message.Role, "content": message.Content}
	}
	parts := make([]map[string]any, 0, len(message.Images)+1)
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, image := range message.Images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": image.DataURL},
		})
	}
	return map[string]any{"role": message.Role, "content": parts}
}

// ParseImageDataURL validates a pasted image data URL and splits it into the
// media type and base64 payload required by the Anthropic message format.
func ParseImageDataURL(value string) (mediaType, data string, ok bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return "", "", false
	}
	meta := value[len("data:"):comma]
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mediaType = strings.ToLower(strings.TrimSuffix(meta, ";base64"))
	switch mediaType {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif":
	default:
		return "", "", false
	}
	if mediaType == "image/jpg" {
		mediaType = "image/jpeg"
	}
	data = value[comma+1:]
	if data == "" || len(data) > 8<<20 {
		return "", "", false
	}
	return mediaType, data, true
}

func truncateErr(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return value
}
