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
