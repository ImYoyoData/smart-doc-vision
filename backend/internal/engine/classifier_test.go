package engine

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestPureClassifierAlwaysPassesCategory(t *testing.T) {
	audit := model.Audit{
		DecisionMode: model.DecisionClassifyOnly,
		Categories: model.JSONBytes([]model.AuditCategory{
			{Code: "contract", Name: "合同协议", Result: model.ResultReject},
			{Code: "invoice", Name: "发票票据", Result: model.ResultReject},
		}),
	}
	verdict := &model.Verdict{Category: "contract", Result: model.ResultReject}
	applyClassificationMapping(audit, verdict)
	if verdict.Result != model.ResultPass || verdict.Category != "contract" || verdict.CategoryName != "合同协议" {
		t.Fatalf("unexpected pure classifier verdict: %+v", verdict)
	}

	unknown := &model.Verdict{Category: "not_configured"}
	applyClassificationMapping(audit, unknown)
	if unknown.Result != model.ResultPass || unknown.CategoryName != "未知分类" {
		t.Fatalf("unknown category should still return PASS: %+v", unknown)
	}
}
