package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/model"
)

type ChatImage struct {
	DataURL string
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Images     []ChatImage `json:"-"`
	ToolCalls  []ToolCall  `json:"-"`
	ToolCallID string      `json:"-"`
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatRequest struct {
	Provider    model.AIProvider
	APIKey      string
	System      string
	Messages    []ChatMessage
	Tools       []ToolSpec
	MaxTokens   int
	Temperature float64
	Reasoning   string
}

type ChatResult struct {
	Content    string     `json:"content"`
	Model      string     `json:"model"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason,omitempty"`
	Usage      Usage      `json:"usage"`
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
	req.Reasoning = normalizeReasoning(req.Reasoning)
	if req.Reasoning != "" && (len(req.Tools) > 0 || !ParseConfig(req.Provider).SupportsThinking) {
		// Tool loops need the assistant/tool messages preserved verbatim; native
		// thinking blocks cannot be replayed safely, so use a prompt-level hint.
		req.System = appendReasoningInstruction(req.System, req.Reasoning)
		req.Reasoning = ""
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
	raw, status, err := callOpenAI(ctx, req, true)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		// Some OpenAI-compatible gateways expose reasoning models but still reject
		// reasoning_effort. Retry once with a prompt-level instruction instead of
		// surfacing an opaque 400 to the administrator.
		if req.Reasoning != "" && (status == http.StatusBadRequest || status == http.StatusUnprocessableEntity) && mentionsReasoning(raw) {
			fallback := req
			fallback.System = appendReasoningInstruction(req.System, req.Reasoning)
			raw, status, err = callOpenAI(ctx, fallback, false)
			if err != nil {
				return nil, err
			}
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("模型返回 HTTP %d: %s", status, truncateErr(string(raw)))
		}
	}
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage openAIUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("模型未返回内容")
	}
	choice := parsed.Choices[0]
	calls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		calls = append(calls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	if strings.TrimSpace(choice.Message.Content) == "" && len(calls) == 0 {
		return nil, fmt.Errorf("模型未返回内容")
	}
	return &ChatResult{
		Content: strings.TrimSpace(choice.Message.Content), Model: parsed.Model,
		ToolCalls: calls, StopReason: choice.FinishReason, Usage: parsed.Usage.usage(),
	}, nil
}

func callOpenAI(ctx context.Context, req ChatRequest, useReasoning bool) ([]byte, int, error) {
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	for _, message := range req.Messages {
		messages = append(messages, openAIChatMessage(message))
	}
	body := map[string]any{
		"model":      req.Provider.Model,
		"messages":   messages,
		"max_tokens": req.MaxTokens,
		"stream":     false,
	}
	// OpenAI reasoning models expect the default temperature when
	// reasoning_effort is supplied.
	if useReasoning && req.Reasoning != "" {
		body["reasoning_effort"] = req.Reasoning
	} else {
		body["temperature"] = req.Temperature
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters,
				},
			})
		}
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint(req.Provider.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	ApplyCustomHeaders(httpReq, ParseConfig(req.Provider).Headers)
	client := &http.Client{Timeout: 150 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, 0, chatTransportError(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return raw, resp.StatusCode, nil
}

func mentionsReasoning(raw []byte) bool {
	lower := strings.ToLower(string(raw))
	for _, keyword := range []string{"reasoning_effort", "reasoning effort", "reasoning", "thinking", "unknown field"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func chatTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("模型响应超时，请重试或更换更快的模型")
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("模型请求已取消")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("模型响应超时，请重试或更换更快的模型")
	}
	return fmt.Errorf("请求模型失败: %w", err)
}

func chatAnthropic(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("Anthropic API Key 未配置")
	}
	messages, err := anthropicMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":      req.Provider.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"name": tool.Name, "description": tool.Description, "input_schema": tool.Parameters,
			})
		}
		body["tools"] = tools
	}
	if strings.TrimSpace(req.System) != "" {
		body["system"] = req.System
	}
	if budget := anthropicThinkingBudget(req.Reasoning); budget > 0 {
		if req.MaxTokens < budget+2048 {
			req.MaxTokens = budget + 2048
		}
		body["max_tokens"] = req.MaxTokens
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	} else {
		body["temperature"] = req.Temperature
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
	client := &http.Client{Timeout: 150 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, chatTransportError(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, truncateErr(string(raw)))
	}
	var parsed struct {
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Error      *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage anthropicUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	var builder strings.Builder
	calls := make([]ToolCall, 0)
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			builder.WriteString(block.Text)
		case "tool_use":
			arguments := strings.TrimSpace(string(block.Input))
			if arguments == "" || arguments == "null" {
				arguments = "{}"
			}
			calls = append(calls, ToolCall{ID: block.ID, Name: block.Name, Arguments: arguments})
		}
	}
	if strings.TrimSpace(builder.String()) == "" && len(calls) == 0 {
		return nil, fmt.Errorf("模型未返回内容")
	}
	return &ChatResult{
		Content: strings.TrimSpace(builder.String()), Model: parsed.Model,
		ToolCalls: calls, StopReason: parsed.StopReason, Usage: parsed.Usage.usage(),
	}, nil
}

func anthropicMessages(input []ChatMessage) ([]map[string]any, error) {
	type turn struct {
		role   string
		blocks []any
	}
	turns := make([]turn, 0, len(input))
	appendTurn := func(role string, blocks []any) {
		if len(blocks) == 0 {
			return
		}
		if len(turns) > 0 && turns[len(turns)-1].role == role {
			turns[len(turns)-1].blocks = append(turns[len(turns)-1].blocks, blocks...)
			return
		}
		turns = append(turns, turn{role: role, blocks: blocks})
	}
	for _, message := range input {
		switch {
		case message.Role == "tool":
			appendTurn("user", []any{map[string]any{
				"type": "tool_result", "tool_use_id": message.ToolCallID, "content": message.Content,
			}})
		case message.Role == "assistant" && len(message.ToolCalls) > 0:
			blocks := make([]any, 0, len(message.ToolCalls)+1)
			if strings.TrimSpace(message.Content) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": message.Content})
			}
			for _, call := range message.ToolCalls {
				var input any = map[string]any{}
				if raw := strings.TrimSpace(call.Arguments); raw != "" {
					if err := json.Unmarshal([]byte(raw), &input); err != nil {
						input = map[string]any{"_raw": raw}
					}
				}
				blocks = append(blocks, map[string]any{
					"type": "tool_use", "id": call.ID, "name": call.Name, "input": input,
				})
			}
			appendTurn("assistant", blocks)
		default:
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
			appendTurn(role, blocks)
		}
	}
	out := make([]map[string]any, 0, len(turns))
	for _, item := range turns {
		out = append(out, map[string]any{"role": item.role, "content": item.blocks})
	}
	return out, nil
}

func normalizeReasoning(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "fast", "quick":
		return "low"
	case "medium", "standard", "normal", "balanced":
		return "medium"
	case "high", "deep":
		return "high"
	default:
		return ""
	}
}

func appendReasoningInstruction(system, level string) string {
	var instruction string
	switch level {
	case "low":
		instruction = "【推理深度：快速】优先直接给出结果，只做必要的检查，不要展开冗长的推理过程。"
	case "high":
		instruction = "【推理深度：深度】在给出结果前进行充分推理与自检，覆盖边界条件、失败路径和潜在风险，但最终答复保持简洁。"
	default:
		instruction = "【推理深度：标准】在给出结果前进行适度分析，权衡关键约束，避免无关推演。"
	}
	instruction += "无论推理深度如何，最终输出必须严格遵守系统提示要求的格式。"
	system = strings.TrimSpace(system)
	if system == "" {
		return instruction
	}
	return system + "\n\n" + instruction
}

func anthropicThinkingBudget(level string) int {
	switch level {
	case "low":
		return 1024
	case "high":
		return 4096
	case "medium":
		return 2048
	default:
		return 0
	}
}

func openAIChatMessage(message ChatMessage) map[string]any {
	switch {
	case message.Role == "tool":
		return map[string]any{
			"role": "tool", "tool_call_id": message.ToolCallID, "content": message.Content,
		}
	case len(message.ToolCalls) > 0:
		calls := make([]map[string]any, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			calls = append(calls, map[string]any{
				"id": call.ID, "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
			})
		}
		return map[string]any{"role": "assistant", "content": message.Content, "tool_calls": calls}
	case len(message.Images) == 0:
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
