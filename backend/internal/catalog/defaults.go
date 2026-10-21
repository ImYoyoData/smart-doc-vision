package catalog

import (
	"fmt"

	"smartdoc-vision/backend/internal/model"
)

const DefaultReviewPrompt = `你是“智审图档”平台的档案图档审核专家。请依据档案管理规范、内容安全要求和业务提供的审核要点，对提交的图档进行审核。

审核重点：
1. 文档/图片是否清晰完整，是否存在缺页、遮挡、篡改、伪造迹象；
2. 是否包含违法违规、色情低俗、暴力恐怖、政治敏感、广告导流或二维码等内容；
3. 是否包含身份证号、手机号、住址等不应出现的个人敏感信息；
4. 文档主体、印章、签名、日期等关键要素是否完整可辨；
5. 与业务提供的扩展信息是否存在明显矛盾。

无法确认时应返回 REVIEW 转人工复核，不要臆测。`

const DefaultClassifyPrompt = `你是“智审图档”平台的档案分类专家。请根据文档文本、OCR 内容和图片版面，从配置的分类列表中选择唯一最匹配的类别。

分类要求：
1. 只允许选择配置中给出的分类编码，不要创建新类别；
2. 优先依据文档标题、正文关键词、印章、表头和版面结构判断；
3. 多个类别都相近时，选择证据最充分的一个，并给出 0~1 的分类置信度；
4. 无法可靠判断时选择“其他/无法识别”类别，不要臆测；
5. 同时说明判断依据和是否存在敏感、伪造、缺页等风险。

分类结果会按审核配置中的“分类 → 终审结论”映射落库，因此分类编码必须准确。`

// ExampleClassificationCategories is also used by the seeded demo audit.
func ExampleClassificationCategories() []model.AuditCategory {
	return []model.AuditCategory{
		{Code: "contract", Name: "合同协议", Description: "合同、协议、补充条款、盖章的签约文件", Result: model.ResultPass},
		{Code: "invoice", Name: "发票票据", Description: "发票、收据、付款凭证、报销单据", Result: model.ResultPass},
		{Code: "engineering_drawing", Name: "工程图纸", Description: "施工图、竣工图、设计图纸、技术图样", Result: model.ResultPass},
		{Code: "id_document", Name: "身份证件", Description: "身份证、护照、驾驶证等个人证件影像", Result: model.ResultReject},
		{Code: "other", Name: "其他 / 无法识别", Description: "不属于以上类别，或图片/文本不足以可靠分类", Result: model.ResultReview},
	}
}

func RulesPayload(strictness string) model.JSONText {
	switch strictness {
	case "classify":
		return ClassificationRulesPayload(0.60)
	case "classify_strict":
		return ClassificationRulesPayload(0.75)
	case "classify_loose":
		return ClassificationRulesPayload(0.45)
	}
	return ReviewRulesPayload(strictness)
}

// ClassificationRulesPayload derives the final decision from the category mapping
// produced by the AI adapter. Low-confidence PASS results are escalated to review.
func ClassificationRulesPayload(minConfidence float64) model.JSONText {
	rules := []map[string]any{
		{
			"name":          "高风险内容强制驳回",
			"priority":      5,
			"logic":         "OR",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReject,
			"items": []map[string]any{
				{"field": "model.risk_level", "operator": "eq", "value": "high"},
				{"field": "model.result", "operator": "eq", "value": model.ResultReject},
			},
		},
		{
			"name":          "低置信度转人工复核",
			"priority":      10,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReview,
			"items": []map[string]any{
				{"field": "model.result", "operator": "eq", "value": model.ResultPass},
				{"field": "model.category_confidence", "operator": "lt", "value": minConfidence},
			},
		},
		{
			"name":          "分类映射通过",
			"priority":      20,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultPass,
			"items": []map[string]any{
				{"field": "model.result", "operator": "eq", "value": model.ResultPass},
				{"field": "model.category_confidence", "operator": "gte", "value": minConfidence},
				{"field": "model.risk_level", "operator": "ne", "value": "high"},
			},
		},
		{
			"name":          "分类映射驳回",
			"priority":      30,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReject,
			"items": []map[string]any{
				{"field": "model.result", "operator": "eq", "value": model.ResultReject},
			},
		},
		{
			"name":          "分类待人工确认",
			"priority":      40,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReview,
			"items": []map[string]any{
				{"field": "model.result", "operator": "in", "value": []string{model.ResultPass, model.ResultReject, model.ResultReview}},
			},
		},
	}
	return model.JSONBytes(rules)
}

func ReviewRulesPayload(strictness string) model.JSONText {
	rejectAt := 0.85
	passBelow := 0.30
	switch strictness {
	case "strict":
		rejectAt = 0.72
		passBelow = 0.20
	case "loose":
		rejectAt = 0.92
		passBelow = 0.40
	}

	rules := []map[string]any{
		{
			"name":          "高风险直接驳回",
			"priority":      10,
			"logic":         "OR",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReject,
			"items": []map[string]any{
				{"field": "model.risk_level", "operator": "eq", "value": "high"},
				{"field": "model.score", "operator": "gte", "value": rejectAt},
				{"field": "model.result", "operator": "eq", "value": model.ResultReject},
			},
		},
		{
			"name":          "低风险快速通过",
			"priority":      20,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultPass,
			"items": []map[string]any{
				{"field": "model.risk_level", "operator": "ne", "value": "high"},
				{"field": "model.score", "operator": "lt", "value": passBelow},
				{"field": "model.result", "operator": "eq", "value": model.ResultPass},
			},
		},
		{
			"name":          "灰区转人工复核",
			"priority":      30,
			"logic":         "AND",
			"stop_on_hit":   true,
			"on_hit_result": model.ResultReview,
			"items": []map[string]any{
				{"field": "model.result", "operator": "in", "value": []string{model.ResultReview, model.ResultPass}},
			},
		},
	}
	return model.JSONBytes(rules)
}

func DefaultRoutes() model.JSONText {
	routes := map[string]any{
		"PASS": map[string]any{
			"exec_mode": "sequential",
			"on_error":  "continue",
			"actions":   []any{},
		},
		"REJECT": map[string]any{
			"exec_mode": "sequential",
			"on_error":  "continue",
			"actions":   []any{},
		},
		"REVIEW": map[string]any{
			"exec_mode": "sequential",
			"on_error":  "to_review",
			"actions":   []any{},
		},
	}
	return model.JSONBytes(routes)
}

func DefaultProviderConfig(kind string, vision bool) model.JSONText {
	cfg := model.ProviderConfig{
		SupportsVision: vision,
		MaxTokens:      1600,
		Temperature:    0.1,
		MaxImages:      6,
	}
	if kind == model.ProviderJEV {
		cfg.SupportsVision = true
		cfg.MaxTokens = 0
		cfg.Temperature = 0
		cfg.MaxImages = 8
	}
	return model.JSONBytes(cfg)
}

func ReviewOutputExample() string {
	return `{"result":"PASS|REJECT|REVIEW","risk_level":"low|medium|high","score":0.0,"labels":["标签"],"summary":"一句话结论","reasons":["原因"]}`
}

func ProviderDefaults(kind string) (baseURL, modelName string) {
	if kind == model.ProviderJEV {
		return "https://api.typesafe.ai/v1", "jev-latest"
	}
	if kind == model.ProviderAnthropic {
		return "https://api.anthropic.com/v1", "claude-sonnet-4-5"
	}
	return "https://api.openai.com/v1", "gpt-4o-mini"
}

func MediaTypesJSON(types []string) model.JSONText {
	if len(types) == 0 {
		types = []string{"text", "image"}
	}
	return model.JSONBytes(types)
}

func SeedAuditName(code string) string {
	return fmt.Sprintf("档案智能审核（%s）", code)
}

type RuleTemplate struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Kind        string         `json:"kind"`
	Rules       model.JSONText `json:"rules"`
}

func RuleTemplates() []RuleTemplate {
	return []RuleTemplate{
		{Key: "standard", Name: "标准审核", Description: "风险分 ≥ 0.85 驳回，< 0.30 通过，其余转人工", Kind: model.DecisionReview, Rules: RulesPayload("standard")},
		{Key: "strict", Name: "严格审核", Description: "风险分 ≥ 0.72 驳回，< 0.20 通过，适合生产档案", Kind: model.DecisionReview, Rules: RulesPayload("strict")},
		{Key: "loose", Name: "宽松审核", Description: "风险分 ≥ 0.92 驳回，< 0.40 通过，适合内部资料", Kind: model.DecisionReview, Rules: RulesPayload("loose")},
		{Key: "classify", Name: "分类标准", Description: "按分类映射结论；置信度 < 0.60 的通过结果转人工", Kind: model.DecisionClassify, Rules: ClassificationRulesPayload(0.60)},
		{Key: "classify_strict", Name: "分类严格", Description: "按分类映射结论；置信度 < 0.75 的通过结果转人工", Kind: model.DecisionClassify, Rules: ClassificationRulesPayload(0.75)},
		{Key: "classify_loose", Name: "分类宽松", Description: "按分类映射结论；置信度 < 0.45 的通过结果转人工", Kind: model.DecisionClassify, Rules: ClassificationRulesPayload(0.45)},
	}
}
