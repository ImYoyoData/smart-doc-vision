package api

import (
	"encoding/json"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestParseAssistantReplyNormalizesDraft(t *testing.T) {
	content := "```json\n{\"reply\":\"已生成\",\"ready\":true,\"config\":{\"name\":\"合同审核\",\"code\":\"Contract Review!\",\"media_types\":[\"image\",\"video\"],\"decision_mode\":\"REVIEW\",\"rules\":[],\"routes\":{},\"dedupe_window_sec\":0}}\n```"
	reply, ready, draft, err := parseAssistantReply(content, 42, 1, 0, nil, nil)
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
	reply, ready, _, err := parseAssistantReply("{\"reply\":\"请告诉我驳回红线\",\"ready\":false}", 1, 1, 0, nil, nil)
	if err != nil || ready || reply != "请告诉我驳回红线" {
		t.Fatalf("unexpected result ready=%v reply=%q err=%v", ready, reply, err)
	}
}

func TestParseAssistantReplyEnablesRedisAndDisablesOpenAPI(t *testing.T) {
	content := `{"reply":"已生成","ready":true,"config":{"name":"档案审核","code":"archive_review","media_types":["text","image"],"decision_mode":"REVIEW","triggers":{"REDIS":"ACTIVE"},"redis_trigger":{"connection_id":1,"db":0,"stream":"sdv:demo:audit:archive_review"}}}`
	_, ready, draft, err := parseAssistantReply(content, 7, 1, 0, nil, nil)
	if err != nil || !ready {
		t.Fatalf("unexpected parse result ready=%v err=%v", ready, err)
	}
	want := map[string]string{
		"OPENAPI": "DISABLED", "REDIS": "ACTIVE", "WEBSOCKET": "DISABLED",
	}
	if !sameTriggerStatuses(draft.Triggers, want) {
		t.Fatalf("triggers = %#v, want %#v", draft.Triggers, want)
	}
}

func TestParseAssistantReplyEditSwitchesToRedis(t *testing.T) {
	content := `{"reply":"已切换","ready":true,"config":{"name":"合同审核","code":"contract_review","media_types":["text"],"decision_mode":"REVIEW","triggers":{"REDIS":"ACTIVE"},"redis_trigger":{"connection_id":1,"db":0,"stream":"sdv:demo:audit:contract_review"}}}`
	current := map[string]string{
		"OPENAPI": "ACTIVE", "REDIS": "DISABLED", "WEBSOCKET": "DISABLED",
	}
	_, ready, draft, err := parseAssistantReply(content, 1, 1, 5, &model.Audit{Code: "contract_review"}, current)
	if err != nil || !ready {
		t.Fatalf("unexpected parse result ready=%v err=%v", ready, err)
	}
	if draft.Triggers["OPENAPI"] != "DISABLED" || draft.Triggers["REDIS"] != "ACTIVE" {
		t.Fatalf("expected Redis-only triggers, got %#v", draft.Triggers)
	}
}

func TestParseAssistantReplyKeepsTriggersWhenNotMentioned(t *testing.T) {
	content := `{"reply":"已生成","ready":true,"config":{"name":"合同审核","code":"contract_review","media_types":["text"],"decision_mode":"REVIEW"}}`
	current := map[string]string{
		"OPENAPI": "ACTIVE", "REDIS": "DISABLED", "WEBSOCKET": "DISABLED",
	}
	_, ready, draft, err := parseAssistantReply(content, 1, 1, 5, &model.Audit{Code: "contract_review"}, current)
	if err != nil || !ready {
		t.Fatalf("unexpected parse result ready=%v err=%v", ready, err)
	}
	if draft.Triggers != nil {
		t.Fatalf("expected triggers to stay untouched, got %#v", draft.Triggers)
	}
}

func sameTriggerStatuses(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
