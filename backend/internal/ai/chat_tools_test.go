package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestOpenAIChatMessageWithToolCalls(t *testing.T) {
	message := openAIChatMessage(ChatMessage{
		Role: "assistant", Content: "checking",
		ToolCalls: []ToolCall{{ID: "call_1", Name: "read_script", Arguments: `{"offset":1}`}},
	})
	calls, ok := message["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls missing: %#v", message)
	}
	function := calls[0]["function"].(map[string]any)
	if function["name"] != "read_script" {
		t.Fatalf("unexpected tool call: %#v", calls[0])
	}
	toolResult := openAIChatMessage(ChatMessage{Role: "tool", ToolCallID: "call_1", Content: "1| code"})
	if toolResult["tool_call_id"] != "call_1" || toolResult["role"] != "tool" {
		t.Fatalf("unexpected tool result: %#v", toolResult)
	}
}

func TestChatOpenAIParsesToolCalls(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_script","arguments":"{\"offset\":1,\"limit\":20}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()

	provider := model.AIProvider{Kind: model.ProviderOpenAI, BaseURL: server.URL, Model: "test-model"}
	result, err := Chat(context.Background(), ChatRequest{
		Provider: provider, APIKey: "key",
		Messages: []ChatMessage{{Role: "user", Content: "read"}},
		Tools: []ToolSpec{{
			Name: "read_script", Description: "read", Parameters: map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "read_script" {
		t.Fatalf("unexpected tool calls: %#v", result.ToolCalls)
	}
	if result.ToolCalls[0].Arguments != `{"offset":1,"limit":20}` {
		t.Fatalf("unexpected arguments: %q", result.ToolCalls[0].Arguments)
	}
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools not sent: %#v", body["tools"])
	}
}

func TestAnthropicMessagesWithToolResults(t *testing.T) {
	turns, err := anthropicMessages([]ChatMessage{
		{Role: "assistant", Content: "checking", ToolCalls: []ToolCall{{ID: "toolu_1", Name: "read_script", Arguments: `{"offset":1}`}}},
		{Role: "tool", ToolCallID: "toolu_1", Content: "1| ctx"},
		{Role: "tool", ToolCallID: "toolu_2", Content: "found"},
	})
	if err != nil {
		t.Fatalf("anthropicMessages returned error: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected assistant + merged user turns, got %#v", turns)
	}
	if turns[0]["role"] != "assistant" || turns[1]["role"] != "user" {
		t.Fatalf("unexpected roles: %#v", turns)
	}
	userBlocks := turns[1]["content"].([]any)
	if len(userBlocks) != 2 {
		t.Fatalf("tool results should merge into one user turn: %#v", userBlocks)
	}
	first := userBlocks[0].(map[string]any)
	if first["type"] != "tool_result" || first["tool_use_id"] != "toolu_1" {
		t.Fatalf("unexpected tool result block: %#v", first)
	}
}

func TestChatAnthropicParsesToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"claude","stop_reason":"tool_use","content":[{"type":"text","text":"let me check"},{"type":"tool_use","id":"toolu_1","name":"read_script","input":{"offset":1}}]}`)
	}))
	defer server.Close()

	provider := model.AIProvider{Kind: model.ProviderAnthropic, BaseURL: server.URL, Model: "claude"}
	result, err := Chat(context.Background(), ChatRequest{
		Provider: provider, APIKey: "key",
		Messages: []ChatMessage{{Role: "user", Content: "read"}},
		Tools: []ToolSpec{{
			Name: "read_script", Description: "read", Parameters: map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if result.Content != "let me check" || len(result.ToolCalls) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.ToolCalls[0].Name != "read_script" || result.ToolCalls[0].Arguments != `{"offset":1}` {
		t.Fatalf("unexpected tool call: %#v", result.ToolCalls[0])
	}
}
