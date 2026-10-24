package api

import (
	"encoding/json"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestParseGeneratedRules(t *testing.T) {
	content := `{"reply":"已生成高风险驳回规则","rules":[{"name":"高风险直接驳回","priority":10,"logic":"OR","stop_on_hit":true,"on_hit_result":"REJECT","items":[{"field":"model.score","operator":"gte","value":0.85}]}]}`
	reply, text, err := parseGeneratedRules(content, model.DecisionReview)
	if err != nil {
		t.Fatalf("parseGeneratedRules: %v", err)
	}
	if reply != "已生成高风险驳回规则" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	var groups []map[string]any
	if err := json.Unmarshal([]byte(text), &groups); err != nil {
		t.Fatalf("generated text is not JSON: %v\n%s", err, text)
	}
	if len(groups) != 1 {
		t.Fatalf("expected one rule group, got %d", len(groups))
	}
}

func TestParseGeneratedRulesClassifierEmpty(t *testing.T) {
	reply, text, err := parseGeneratedRules(`{"reply":"纯分类器无需规则","rules":[]}`, model.DecisionClassifyOnly)
	if err != nil {
		t.Fatalf("parseGeneratedRules: %v", err)
	}
	if text != "[]" {
		t.Fatalf("expected empty JSON array, got %q", text)
	}
	if reply == "" {
		t.Fatal("expected explanation for classifier empty rules")
	}
}

func TestParseGeneratedRulesRejectsUnknownField(t *testing.T) {
	content := `{"reply":"","rules":[{"name":"bad","priority":10,"logic":"AND","on_hit_result":"REJECT","items":[{"field":"model.not_exists","operator":"eq","value":"x"}]}]}`
	_, _, err := parseGeneratedRules(content, model.DecisionReview)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "不支持的字段") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGeneratedPrompt(t *testing.T) {
	content := `{"reply":"补充了缺页检查","prompt":"你是档案审核专家。请检查文档是否缺页、印章和签名是否完整。存在明确违规或关键要素缺失时返回 REJECT；无法确认时返回 REVIEW。"}`
	reply, text, err := parseGeneratedPrompt(content)
	if err != nil {
		t.Fatalf("parseGeneratedPrompt: %v", err)
	}
	if reply != "补充了缺页检查" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if !strings.Contains(text, "缺页") {
		t.Fatalf("unexpected prompt: %q", text)
	}
}

func TestParseGeneratedPromptPlainText(t *testing.T) {
	content := "你是档案审核专家。请检查文档是否清晰完整、是否存在敏感内容，并在无法确认时返回 REVIEW。"
	_, text, err := parseGeneratedPrompt(content)
	if err != nil {
		t.Fatalf("parseGeneratedPrompt: %v", err)
	}
	if text != content {
		t.Fatalf("expected plain text prompt, got %q", text)
	}
}

func TestParseGeneratedPromptRejectsJSONWithoutPrompt(t *testing.T) {
	_, _, err := parseGeneratedPrompt(`{"reply":"只返回了说明，没有 prompt"}`)
	if err == nil {
		t.Fatal("expected error when JSON has no prompt")
	}
}
