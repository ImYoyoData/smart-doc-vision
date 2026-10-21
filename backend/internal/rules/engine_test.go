package rules

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func auditWithRules(rules any) model.Audit {
	return model.Audit{
		Rules:         model.JSONBytes(rules),
		DefaultResult: model.ResultReview,
	}
}

func TestEvaluateHighRiskRejects(t *testing.T) {
	audit := auditWithRules([]map[string]any{
		{
			"name": "高风险", "priority": 10, "logic": "OR", "stop_on_hit": true, "on_hit_result": "REJECT",
			"items": []map[string]any{
				{"field": "model.risk_level", "operator": "eq", "value": "high"},
				{"field": "model.score", "operator": "gte", "value": 0.85},
			},
		},
		{
			"name": "低风险", "priority": 20, "logic": "AND", "stop_on_hit": true, "on_hit_result": "PASS",
			"items": []map[string]any{
				{"field": "model.score", "operator": "lt", "value": 0.3},
			},
		},
	})
	result := Evaluate(audit, Context{
		Verdict: &model.Verdict{Result: model.ResultReview, RiskLevel: "high", Score: 0.92},
		Input:   model.TaskInput{BizKey: "abc"},
	})
	if result.FinalResult != model.ResultReject {
		t.Fatalf("expected REJECT, got %s", result.FinalResult)
	}
	if result.MatchedRule != "高风险" {
		t.Fatalf("unexpected matched rule: %s", result.MatchedRule)
	}
	if len(result.Trace) != 1 {
		t.Fatalf("expected stop_on_hit to stop evaluation, trace=%d", len(result.Trace))
	}
}

func TestEvaluateLowRiskPasses(t *testing.T) {
	audit := auditWithRules([]map[string]any{
		{
			"name": "高风险", "priority": 10, "logic": "OR", "stop_on_hit": true, "on_hit_result": "REJECT",
			"items": []map[string]any{
				{"field": "model.score", "operator": "gte", "value": 0.85},
			},
		},
		{
			"name": "低风险", "priority": 20, "logic": "AND", "stop_on_hit": true, "on_hit_result": "PASS",
			"items": []map[string]any{
				{"field": "model.score", "operator": "lt", "value": 0.3},
				{"field": "ext.scene", "operator": "eq", "value": "archive"},
			},
		},
	})
	result := Evaluate(audit, Context{
		Verdict: &model.Verdict{Result: model.ResultPass, RiskLevel: "low", Score: 0.12},
		Input:   model.TaskInput{Ext: map[string]any{"scene": "archive"}},
	})
	if result.FinalResult != model.ResultPass || result.MatchedRule != "低风险" {
		t.Fatalf("expected low-risk PASS, got %+v", result)
	}
}

func TestEvaluateNoMatchReturnsEmptyResult(t *testing.T) {
	audit := auditWithRules([]map[string]any{
		{
			"name": "极高风险", "priority": 10, "logic": "AND", "stop_on_hit": true, "on_hit_result": "REJECT",
			"items": []map[string]any{
				{"field": "model.score", "operator": "gte", "value": 0.99},
				{"field": "ext.blocked", "operator": "eq", "value": true},
			},
		},
	})
	result := Evaluate(audit, Context{
		Verdict: &model.Verdict{Result: model.ResultReview, RiskLevel: "medium", Score: 0.5},
		Input:   model.TaskInput{Ext: map[string]any{"blocked": false}},
	})
	if result.FinalResult != "" {
		t.Fatalf("expected no rule hit, got %s", result.FinalResult)
	}
	if result.MatchedRule != "" {
		t.Fatalf("expected no matched rule")
	}
}

func TestEvaluateInvalidJSONIsSafe(t *testing.T) {
	audit := model.Audit{Rules: model.JSONText(`{not json`)}
	result := Evaluate(audit, Context{Verdict: &model.Verdict{Result: model.ResultReview}})
	if result.FinalResult != "" || len(result.Trace) == 0 {
		t.Fatalf("invalid rules should produce a skipped trace, got %+v", result)
	}
}

func TestEvaluateCategoryRules(t *testing.T) {
	audit := auditWithRules([]map[string]any{
		{
			"name": "低置信度转人工", "priority": 10, "logic": "AND", "stop_on_hit": true, "on_hit_result": "REVIEW",
			"items": []map[string]any{
				{"field": "model.category_confidence", "operator": "lt", "value": 0.6},
				{"field": "model.result", "operator": "eq", "value": "PASS"},
			},
		},
		{
			"name": "分类通过", "priority": 20, "logic": "AND", "stop_on_hit": true, "on_hit_result": "PASS",
			"items": []map[string]any{
				{"field": "model.category", "operator": "in", "value": []string{"contract", "invoice"}},
				{"field": "model.category_confidence", "operator": "gte", "value": 0.6},
			},
		},
	})
	result := Evaluate(audit, Context{
		Verdict: &model.Verdict{Result: model.ResultPass, Category: "contract", CategoryConfidence: 0.88},
	})
	if result.FinalResult != model.ResultPass || result.MatchedRule != "分类通过" {
		t.Fatalf("expected category PASS, got %+v", result)
	}
}
