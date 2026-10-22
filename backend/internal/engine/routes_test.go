package engine

import (
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestRenderStringIncludesResultFields(t *testing.T) {
	render := routeRender{
		Task:        model.Task{TaskNo: "SDV1", BizKey: "order-1", SourceType: model.TriggerRedis},
		Audit:       model.Audit{Code: "archive_review"},
		Project:     model.Project{Code: "demo"},
		FinalResult: model.ResultPass,
		Detail: map[string]any{
			"summary": "内容合规", "risk_level": "low", "score": 0.12,
			"category": "contract", "category_name": "合同协议",
		},
	}
	out := renderString("${task_no}|${result}|${score}|${category}|${category_name}|${risk_level}|${source_type}", render)
	want := "SDV1|PASS|0.12|contract|合同协议|low|REDIS"
	if out != want {
		t.Fatalf("renderString = %q, want %q", out, want)
	}
}

func TestWebSocketURLDetection(t *testing.T) {
	for _, target := range []string{"ws://example.com/cb", "wss://example.com/cb", " WSS://example.com/cb "} {
		if !isWebSocketURL(target) {
			t.Fatalf("%q should be treated as websocket", target)
		}
	}
	if isWebSocketURL("https://example.com/cb") {
		t.Fatal("https URL must not be treated as websocket")
	}
	if err := validateWebSocketURL("https://example.com/cb"); err == nil || !strings.Contains(err.Error(), "ws/wss") {
		t.Fatalf("expected scheme rejection, got %v", err)
	}
	if err := validateWebSocketURL("ws://127.0.0.1:9000/cb"); err == nil {
		t.Fatal("loopback websocket must be rejected by SSRF validation")
	}
}
