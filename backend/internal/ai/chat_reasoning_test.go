package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestNormalizeReasoning(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"auto":     "",
		"LOW":      "low",
		"fast":     "low",
		"medium":   "medium",
		"standard": "medium",
		"deep":     "high",
		"unknown":  "",
	}
	for input, want := range cases {
		if got := normalizeReasoning(input); got != want {
			t.Fatalf("normalizeReasoning(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAppendReasoningInstruction(t *testing.T) {
	got := appendReasoningInstruction("base prompt", "high")
	if !strings.HasPrefix(got, "base prompt\n\n") {
		t.Fatalf("system prompt prefix lost: %q", got)
	}
	if !strings.Contains(got, "推理深度：深度") {
		t.Fatalf("deep instruction missing: %q", got)
	}
	if !strings.Contains(got, "最终输出必须严格遵守") {
		t.Fatalf("format reminder missing: %q", got)
	}
	if empty := appendReasoningInstruction("  ", "low"); strings.HasPrefix(empty, "\n") || !strings.Contains(empty, "快速") {
		t.Fatalf("unexpected empty-system instruction: %q", empty)
	}
}

func TestChatOpenAIReasoningEffortAndPromptFallback(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	provider := model.AIProvider{
		Kind: model.ProviderOpenAI, BaseURL: server.URL, Model: "test-model",
		ConfigJSON: model.JSONText(`{"supports_thinking":true}`),
	}
	result, err := Chat(context.Background(), ChatRequest{
		Provider: provider, APIKey: "key", System: "system", Reasoning: "high",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	if len(bodies) != 1 || bodies[0]["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort not sent: %#v", bodies)
	}

	bodies = nil
	plain := provider
	plain.ConfigJSON = model.JSONText(`{}`)
	if _, err := Chat(context.Background(), ChatRequest{
		Provider: plain, APIKey: "key", System: "system", Reasoning: "medium",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("unexpected request count: %d", len(bodies))
	}
	if _, exists := bodies[0]["reasoning_effort"]; exists {
		t.Fatalf("reasoning_effort must not be sent to unsupported providers")
	}
	messages, _ := bodies[0]["messages"].([]any)
	system, _ := messages[0].(map[string]any)
	if content, _ := system["content"].(string); !strings.Contains(content, "推理深度：标准") {
		t.Fatalf("prompt-level reasoning instruction missing: %q", content)
	}
}

func TestChatOpenAIRetriesWhenReasoningEffortRejected(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Unrecognized request argument supplied: reasoning_effort"}}`)
			return
		}
		if _, exists := body["reasoning_effort"]; exists {
			t.Errorf("retry must not include reasoning_effort")
		}
		messages, _ := body["messages"].([]any)
		system, _ := messages[0].(map[string]any)
		if content, _ := system["content"].(string); !strings.Contains(content, "推理深度：深度") {
			t.Errorf("retry should fall back to a prompt instruction, got %q", content)
		}
		_, _ = io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	provider := model.AIProvider{
		Kind: model.ProviderOpenAI, BaseURL: server.URL, Model: "test-model",
		ConfigJSON: model.JSONText(`{"supports_thinking":true}`),
	}
	if _, err := Chat(context.Background(), ChatRequest{
		Provider: provider, APIKey: "key", System: "system", Reasoning: "high",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestChatAnthropicThinkingBudget(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"claude","content":[{"type":"text","text":"ok"}]}`)
	}))
	defer server.Close()

	provider := model.AIProvider{
		Kind: model.ProviderAnthropic, BaseURL: server.URL, Model: "claude",
		ConfigJSON: model.JSONText(`{"supports_thinking":true}`),
	}
	result, err := Chat(context.Background(), ChatRequest{
		Provider: provider, APIKey: "key", System: "system", Reasoning: "high",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}}, MaxTokens: 3200,
	})
	if err != nil || result.Content != "ok" {
		t.Fatalf("Chat result = %#v, err = %v", result, err)
	}
	thinking, _ := body["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(4096) {
		t.Fatalf("thinking block missing or invalid: %#v", body["thinking"])
	}
	if body["max_tokens"] != float64(6144) {
		t.Fatalf("max_tokens should allow the thinking budget: %#v", body["max_tokens"])
	}
	if _, exists := body["temperature"]; exists {
		t.Fatalf("temperature must be omitted when thinking is enabled")
	}
}
