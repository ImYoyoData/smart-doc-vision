package api

import (
	"errors"
	"testing"

	"smartdoc-vision/backend/internal/ai"
)

func TestLogsAssistantTools(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range logsAssistantTools() {
		names[tool.Name] = true
	}
	for _, name := range []string{"search_audit_logs", "aggregate_audit_logs", "get_audit_log_detail", "apply_log_filters"} {
		if !names[name] {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestParseLogsFilter(t *testing.T) {
	filter := parseLogsFilter(map[string]any{
		"operator": "admin", "actions": []any{"LOGIN", "TASK_SUBMIT"},
		"object_type": "USER", "result": "FAILED", "keyword": "127.0.0.1",
		"trace_id": "trace-1", "start": "2026-10-01", "end": "2026-10-31",
	})
	if filter.Operator != "admin" || len(filter.Actions) != 2 || filter.Actions[0] != "LOGIN" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
	if filter.ObjectType != "USER" || filter.Result != "FAILED" || filter.Keyword != "127.0.0.1" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
	if filter.TraceID != "trace-1" || filter.Start != "2026-10-01" || filter.End != "2026-10-31" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestAuditActionLabel(t *testing.T) {
	if auditActionLabel("LOGIN_MFA_ONLY") != "Authenticator 登录" {
		t.Fatal("unexpected action label")
	}
	if auditActionLabel("CUSTOM_ACTION") != "CUSTOM_ACTION" {
		t.Fatal("unknown action should keep its code")
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	if got := estimateMessagesTokens([]ai.ChatMessage{{Role: "user", Content: "abc"}}); got < 3 {
		t.Fatalf("estimate too small: %d", got)
	}
	if isContextLimitError(nil) {
		t.Fatal("nil error must not be a context error")
	}
	if !isContextLimitError(errors.New("maximum context length exceeded")) {
		t.Fatal("context limit error was not recognized")
	}
}
