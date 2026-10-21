package api

import (
	"encoding/json"
	"testing"
)

func TestParseAssistantReplyNormalizesDraft(t *testing.T) {
	content := "```json\n{\"reply\":\"已生成\",\"ready\":true,\"config\":{\"name\":\"合同审核\",\"code\":\"Contract Review!\",\"media_types\":[\"image\",\"video\"],\"decision_mode\":\"REVIEW\",\"rules\":[],\"routes\":{},\"dedupe_window_sec\":0}}\n```"
	reply, ready, draft, err := parseAssistantReply(content, 42, 1, 0, nil)
	if err != nil || !ready || reply != "已生成" {
		t.Fatalf("unexpected parse result ready=%v reply=%q err=%v", ready, reply, err)
	}
	if draft.Code != "contract_review" {
		t.Fatalf("code = %q", draft.Code)
	}
	if draft.ProviderID != 42 || len(draft.MediaTypes) != 1 || draft.MediaTypes[0] != "image" {
		t.Fatalf("unexpected provider/media: %+v", draft)
	}
	if draft.DedupeWindowSec != 86400 {
		t.Fatalf("dedupe = %d", draft.DedupeWindowSec)
	}
	var rules []any
	if err := json.Unmarshal(draft.Rules, &rules); err != nil || len(rules) == 0 {
		t.Fatalf("expected default rules, got %s", string(draft.Rules))
	}
	var routes map[string]any
	if err := json.Unmarshal(draft.Routes, &routes); err != nil || len(routes) != 3 {
		t.Fatalf("expected default routes, got %s", string(draft.Routes))
	}
}

func TestParseAssistantReplyKeepsQuestion(t *testing.T) {
	reply, ready, _, err := parseAssistantReply("{\"reply\":\"请告诉我驳回红线\",\"ready\":false}", 1, 1, 0, nil)
	if err != nil || ready || reply != "请告诉我驳回红线" {
		t.Fatalf("unexpected result ready=%v reply=%q err=%v", ready, reply, err)
	}
}
