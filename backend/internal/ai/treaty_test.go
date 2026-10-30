package ai

import (
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func treatyAudit(docs ...model.AuditTreaty) model.Audit {
	return model.Audit{Treaties: model.JSONBytes(docs)}
}

func TestAuditTreatyPromptIncludesOnlyReadyContent(t *testing.T) {
	audit := treatyAudit(
		model.AuditTreaty{Name: "归档条约", Status: model.TreatyStatusReady, Content: "超过 90 天必须转人工。"},
		model.AuditTreaty{Name: "失败文档", Status: model.TreatyStatusFailed, Content: "不应注入"},
		model.AuditTreaty{Name: "空文档", Status: model.TreatyStatusReady, Content: "  "},
	)
	got := AuditTreatyPrompt(audit)
	if !strings.Contains(got, "归档条约") || !strings.Contains(got, "超过 90 天必须转人工") {
		t.Fatalf("ready treaty missing: %q", got)
	}
	if strings.Contains(got, "失败文档") || strings.Contains(got, "不应注入") {
		t.Fatalf("failed treaty was injected: %q", got)
	}
}

func TestApplyTreatyPromptReplacesPlaceholder(t *testing.T) {
	audit := treatyAudit(model.AuditTreaty{Name: "合同条约", Status: model.TreatyStatusReady, Content: "缺少盖章必须 REVIEW。"})
	got := ApplyTreatyPrompt("按审核要求判断。\n"+TreatyPlaceholder, audit)
	if strings.Contains(got, TreatyPlaceholder) {
		t.Fatalf("placeholder was not replaced: %q", got)
	}
	if !strings.Contains(got, "合同条约") || !strings.Contains(got, "缺少盖章必须 REVIEW") {
		t.Fatalf("treaty content missing: %q", got)
	}
}

func TestApplyTreatyPromptAppendsWithoutPlaceholder(t *testing.T) {
	audit := treatyAudit(model.AuditTreaty{Name: "合同条约", Status: model.TreatyStatusReady, Content: "缺少盖章必须 REVIEW。"})
	got := ApplyTreatyPrompt("按审核要求判断。", audit)
	if !strings.Contains(got, "【审核条约】") || !strings.Contains(got, "缺少盖章必须 REVIEW") {
		t.Fatalf("treaty was not appended: %q", got)
	}
}

func TestApplyTreatyPromptWithoutTreaty(t *testing.T) {
	got := ApplyTreatyPrompt("按审核要求判断。\n"+TreatyPlaceholder, model.Audit{})
	if strings.Contains(got, TreatyPlaceholder) {
		t.Fatalf("placeholder should be removed: %q", got)
	}
	if !strings.Contains(got, "未配置审核条约") {
		t.Fatalf("empty treaty marker missing: %q", got)
	}
}
