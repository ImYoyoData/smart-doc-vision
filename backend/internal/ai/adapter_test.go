package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestOpenAIEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":                   "https://api.openai.com/v1/chat/completions",
		"https://api.example.com/v1/":                 "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1/chat/completions": "https://api.example.com/v1/chat/completions",
	}
	for input, want := range cases {
		if got := openAIEndpoint(input); got != want {
			t.Fatalf("openAIEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnthropicEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.anthropic.com":             "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/":            "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/v1":          "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/v1/messages": "https://api.anthropic.com/v1/messages",
	}
	for input, want := range cases {
		if got := anthropicEndpoint(input); got != want {
			t.Fatalf("anthropicEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnthropicText(t *testing.T) {
	parsed := anthropicResponse{}
	parsed.Content = append(parsed.Content, struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "text", Text: `{"result":"PASS","score":0.2}`})
	if got := anthropicText(parsed); got != `{"result":"PASS","score":0.2}` {
		t.Fatalf("unexpected anthropic text: %q", got)
	}
}

func TestJEVEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.typesafe.ai":              "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai/":             "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai/v1":           "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai/v1/systemone": "https://api.typesafe.ai/v1/systemone",
	}
	for input, want := range cases {
		if got := jevEndpoint(input); got != want {
			t.Fatalf("jevEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExtractJSONObjectFromMarkdown(t *testing.T) {
	obj, err := extractJSONObject("```json\n{\"result\":\"PASS\",\"score\":0.1}\n```")
	if err != nil {
		t.Fatal(err)
	}
	v := verdictFromObject(obj)
	normalizeVerdict(v)
	if v.Result != model.ResultPass {
		t.Fatalf("expected PASS, got %s", v.Result)
	}
}

func TestNormalizeVerdictDerivesResult(t *testing.T) {
	v := &model.Verdict{Score: 0.93}
	normalizeVerdict(v)
	if v.Result != model.ResultReject || v.RiskLevel != "high" {
		t.Fatalf("unexpected normalized verdict: %+v", v)
	}
}

func TestClassificationVerdictMapsCategoryResult(t *testing.T) {
	categories := []model.AuditCategory{
		{Code: "contract", Name: "合同协议", Result: model.ResultPass},
		{Code: "id_document", Name: "身份证件", Result: model.ResultReject},
		{Code: "other", Name: "其他", Result: model.ResultReview},
	}
	parsed := jevResponse{Model: "jev-test"}
	choice := jevChoiceAnswer{Type: "choice", Choice: "contract", Confidence: 0.88, Probabilities: map[string]float64{"contract": 0.88}}
	verdict := jevClassificationVerdict(parsed, choice, categories, 0.02, 0.05, nil, 12, Request{})
	if verdict.Result != model.ResultPass || verdict.Category != "contract" || verdict.CategoryName != "合同协议" {
		t.Fatalf("unexpected classification verdict: %+v", verdict)
	}
	if verdict.CategoryConfidence != 0.88 {
		t.Fatalf("expected category confidence 0.88, got %v", verdict.CategoryConfidence)
	}
}

func TestExtractJSONObjectRepairsTrailingCommas(t *testing.T) {
	obj, err := extractJSONObject(`{"result":"PASS","labels":["a","b",],"score":0.1,}`)
	if err != nil {
		t.Fatal(err)
	}
	v := verdictFromObject(obj)
	normalizeVerdict(v)
	if v.Result != model.ResultPass || len(v.Labels) != 2 {
		t.Fatalf("unexpected repaired verdict: %+v", v)
	}
}

func TestIsModelOutputError(t *testing.T) {
	wrapped := fmt.Errorf("重试后仍失败: %w", ErrModelOutput)
	if !IsModelOutputError(wrapped) {
		t.Fatal("wrapped output error should be detected")
	}
	if IsModelOutputError(fmt.Errorf("大模型返回 HTTP 500")) {
		t.Fatal("provider errors must not be treated as output errors")
	}
}

func TestOpenAIRetriesMalformedOutputWithLargerBudget(t *testing.T) {
	var calls int
	var budgets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		budgets = append(budgets, payload.MaxTokens)
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":"{\"result\":\"PASS\",\"reasons\":[\"x\"}"},"finish_reason":"length"}],"usage":{}}`)
			return
		}
		io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":"{\"result\":\"PASS\",\"risk_level\":\"low\",\"score\":0.1,\"summary\":\"ok\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer server.Close()

	provider := model.AIProvider{
		Name:       "测试模型",
		Kind:       model.ProviderOpenAI,
		BaseURL:    server.URL + "/v1",
		Model:      "test-model",
		ConfigJSON: model.JSONBytes(model.ProviderConfig{MaxTokens: 1600, Temperature: 0.1}),
		Status:     model.ProviderActive,
	}
	adapter := &OpenAIAdapter{Client: server.Client()}
	verdict, err := adapter.Review(context.Background(), Request{
		Provider: provider,
		APIKey:   "test-key",
		Audit:    model.Audit{ReviewPrompt: "检查档案完整性"},
		Input:    model.TaskInput{BizKey: "biz-1", Media: []model.MediaItem{{Type: "text", Content: "档案正文"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
	if len(budgets) != 2 || budgets[0] != 1600 || budgets[1] != 3200 {
		t.Fatalf("unexpected token budgets: %v", budgets)
	}
	if verdict.Result != model.ResultPass {
		t.Fatalf("unexpected verdict: %+v", verdict)
	}
}

func TestClassificationVerdictEscalatesSensitiveContent(t *testing.T) {
	categories := []model.AuditCategory{{Code: "contract", Name: "合同协议", Result: model.ResultPass}}
	choice := jevChoiceAnswer{Choice: "contract", Confidence: 0.95}
	verdict := jevClassificationVerdict(jevResponse{}, choice, categories, 0.9, 0.1, nil, 10, Request{})
	if verdict.Result != model.ResultReject {
		t.Fatalf("expected sensitive content to force REJECT, got %s", verdict.Result)
	}
}

func TestClassifySchemaFields(t *testing.T) {
	v := verdictFromObject(map[string]any{
		"category":            "contract",
		"category_name":       "合同协议",
		"category_confidence": 0.91,
		"score":               0.1,
		"summary":             "合同类",
	})
	normalizeVerdict(v)
	if v.Category != "contract" || v.CategoryConfidence != 0.91 {
		t.Fatalf("unexpected category verdict: %+v", v)
	}
}
