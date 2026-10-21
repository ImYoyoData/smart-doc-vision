package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestAnthropicReviewRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("missing anthropic-version header")
		}
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "claude-test" || req.MaxTokens <= 0 {
			t.Fatalf("unexpected request: %+v", req)
		}
		if len(req.Messages) != 1 || len(req.Messages[0].Content) == 0 {
			t.Fatalf("missing message content")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test-2026",
			"stop_reason":"end_turn",
			"content":[{"type":"text","text":"{\"result\":\"PASS\",\"risk_level\":\"low\",\"score\":0.08,\"summary\":\"清晰完整\",\"reasons\":[\"要素齐全\"]}"}]
		}`))
	}))
	defer server.Close()

	adapter := &AnthropicAdapter{Client: server.Client()}
	verdict, err := adapter.Review(context.Background(), Request{
		Provider: model.AIProvider{
			Kind:    model.ProviderAnthropic,
			BaseURL: server.URL,
			Model:   "claude-test",
			ConfigJSON: model.JSONBytes(model.ProviderConfig{
				MaxTokens: 1200,
			}),
		},
		APIKey: "test-key",
		Audit:  model.Audit{ReviewPrompt: "检查档案完整性"},
		Input: model.TaskInput{
			BizKey: "archive:test",
			Media:  []model.MediaItem{{Type: "text", Content: "合同正文"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Result != model.ResultPass || verdict.Provider != model.ProviderAnthropic {
		t.Fatalf("unexpected verdict: %+v", verdict)
	}
	if verdict.Model != "claude-test-2026" {
		t.Fatalf("unexpected model: %s", verdict.Model)
	}
}
