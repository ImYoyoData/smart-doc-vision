package api

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestPureClassifierCategoriesBecomePass(t *testing.T) {
	categories := []model.AuditCategory{
		{Code: "contract", Name: "合同协议", Result: model.ResultReject},
		{Code: "invoice", Name: "发票票据", Result: model.ResultReject},
	}
	if err := validateDecisionConfig(model.DecisionClassifyOnly, categories); err != nil {
		t.Fatal(err)
	}
	for _, category := range categories {
		if category.Result != model.ResultPass {
			t.Fatalf("category %s result = %s, want PASS", category.Code, category.Result)
		}
	}
}

func TestValidateRoutesJSON(t *testing.T) {
	valid := []byte(`{
		"PASS": {"exec_mode":"sequential","on_error":"continue","actions":[
			{"type":"http","method":"POST","url":"https://example.com/cb","params":{"task":"${task_no}"},"headers":{"X-Token":"abc"}},
			{"type":"websocket","url":"wss://example.com/ws","headers":{"Authorization":"Bearer x"}},
			{"type":"redis","command":"publish","target":"audit:done"},
			{"type":"js","remark":"写回","script":"function main(ctx) { return { ok: true, result: ctx.result } }","timeout_ms":5000,"memory_mb":64}
		]},
		"REJECT": {"on_error":"stop","actions":[]},
		"REVIEW": {"on_error":"to_review","actions":[]}
	}`)
	if err := validateRoutesJSON(valid); err != nil {
		t.Fatalf("valid routes rejected: %v", err)
	}
	invalid := []string{
		`{"UNKNOWN":{"actions":[]}}`,
		`{"PASS":{"actions":[{"type":"grpc"}]}}`,
		`{"PASS":{"actions":[{"type":"http"}]}}`,
		`{"PASS":{"actions":[{"type":"websocket","url":"https://example.com"}]}}`,
		`{"PASS":{"actions":[{"type":"redis","command":"del","target":"x"}]}}`,
		`{"PASS":{"actions":[{"type":"js","script":""}]}}`,
		`{"PASS":{"actions":[{"type":"js","script":"function main( {"}]}}`,
		`{"PASS":{"on_error":"retry","actions":[]}}`,
	}
	for _, raw := range invalid {
		if err := validateRoutesJSON([]byte(raw)); err == nil {
			t.Fatalf("invalid routes accepted: %s", raw)
		}
	}
}

func TestValidateCallbackURLWebSocket(t *testing.T) {
	if err := validateCallbackURL("wss://example.com/hook", true); err != nil {
		t.Fatalf("websocket callback should be accepted: %v", err)
	}
	if err := validateCallbackURL("ftp://example.com/hook", true); err == nil {
		t.Fatal("ftp callback must be rejected")
	}
}
