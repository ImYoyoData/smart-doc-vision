package catalog

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestEnsureClassifierUnknown(t *testing.T) {
	categories := []model.AuditCategory{{Code: "contract", Name: "合同协议", Result: model.ResultPass}}
	out := EnsureClassifierUnknown(categories)
	if len(out) != 2 || out[1].Code != "unknown" || out[1].Result != model.ResultPass {
		t.Fatalf("unexpected categories: %+v", out)
	}
	custom := []model.AuditCategory{{Code: "unknown", Name: "自定义未知类型", Result: model.ResultPass}}
	kept := EnsureClassifierUnknown(custom)
	if len(kept) != 1 || kept[0].Name != "自定义未知类型" {
		t.Fatalf("existing editable unknown category must be preserved: %+v", kept)
	}
}
