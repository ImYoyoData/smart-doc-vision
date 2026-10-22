package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/textenc"
)

type Request struct {
	Provider model.AIProvider
	APIKey   string
	Audit    model.Audit
	Input    model.TaskInput
}

type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message"`
	Model     string `json:"model"`
}

type Adapter interface {
	Review(ctx context.Context, req Request) (*model.Verdict, error)
	Test(ctx context.Context, req Request) (TestResult, error)
}

func AdapterFor(kind string) (Adapter, error) {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case model.ProviderOpenAI, "OPENAI":
		return &OpenAIAdapter{}, nil
	case model.ProviderAnthropic, "CLAUDE":
		return &AnthropicAdapter{}, nil
	case model.ProviderJEV, "TYPESAFE", "TYPESAFE_JEV":
		return &JEVAdapter{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", kind)
	}
}

func ParseConfig(p model.AIProvider) model.ProviderConfig {
	var cfg model.ProviderConfig
	_ = model.DecodeJSON(p.ConfigJSON, &cfg)
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2200
	}
	if cfg.MaxTokens > 8000 {
		cfg.MaxTokens = 8000
	}
	if cfg.MaxImages <= 0 {
		cfg.MaxImages = 6
	}
	return cfg
}

func CollectText(input model.TaskInput) string {
	var parts []string
	for _, m := range input.Media {
		if strings.EqualFold(m.Type, "text") {
			if content := strings.TrimSpace(textenc.Normalize(m.Content)); content != "" {
				parts = append(parts, content)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func CollectImages(input model.TaskInput) []model.MediaItem {
	var out []model.MediaItem
	for _, m := range input.Media {
		if strings.EqualFold(m.Type, "image") && strings.TrimSpace(m.URL) != "" {
			out = append(out, m)
		}
	}
	return out
}

func ValidateInput(input model.TaskInput) error {
	text := CollectText(input)
	images := CollectImages(input)
	if text == "" && len(images) == 0 {
		return errors.New("至少提供一段文本或一张图片")
	}
	for _, m := range input.Media {
		t := strings.ToLower(strings.TrimSpace(m.Type))
		if t != "text" && t != "image" {
			return fmt.Errorf("当前版本仅支持多文本 + 多图片审核，不支持 %q（视频/音频暂不支持）", m.Type)
		}
	}
	return nil
}

func deriveMediaType(input model.TaskInput) string {
	text := CollectText(input) != ""
	images := len(CollectImages(input)) > 0
	switch {
	case text && images:
		return "mixed"
	case images:
		return "image"
	default:
		return "text"
	}
}

func MediaType(input model.TaskInput) string { return deriveMediaType(input) }

func IsClassification(audit model.Audit) bool {
	mode := strings.TrimSpace(audit.DecisionMode)
	return strings.EqualFold(mode, model.DecisionClassify) || strings.EqualFold(mode, model.DecisionClassifyOnly)
}

// IsClassifyOnly identifies a pure classifier: it always returns the selected
// category while the platform fixes the final result to PASS.
func IsClassifyOnly(audit model.Audit) bool {
	return strings.EqualFold(strings.TrimSpace(audit.DecisionMode), model.DecisionClassifyOnly)
}

func AuditCategories(audit model.Audit) []model.AuditCategory {
	var categories []model.AuditCategory
	if err := model.DecodeJSON(audit.Categories, &categories); err != nil {
		return nil
	}
	out := make([]model.AuditCategory, 0, len(categories))
	for _, category := range categories {
		category.Code = strings.ToLower(strings.TrimSpace(category.Code))
		category.Name = strings.TrimSpace(category.Name)
		category.Description = strings.TrimSpace(category.Description)
		category.Result = strings.ToUpper(strings.TrimSpace(category.Result))
		if category.Code != "" && category.Name != "" {
			out = append(out, category)
		}
	}
	return out
}

func FindCategory(categories []model.AuditCategory, code string) (model.AuditCategory, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, category := range categories {
		if strings.EqualFold(category.Code, code) {
			return category, true
		}
	}
	for _, category := range categories {
		if strings.EqualFold(category.Name, code) {
			return category, true
		}
	}
	return model.AuditCategory{}, false
}

func CategoryCriteria(categories []model.AuditCategory) map[string]any {
	criteria := make(map[string]any, len(categories))
	for _, category := range categories {
		description := category.Description
		if description == "" {
			description = category.Name
		}
		criteria[category.Code] = fmt.Sprintf("%s。命中后终审结论：%s。", description, category.Result)
	}
	return criteria
}

func normalizeVerdict(v *model.Verdict) {
	v.Category = strings.ToLower(strings.TrimSpace(v.Category))
	v.CategoryName = strings.TrimSpace(v.CategoryName)
	if v.CategoryConfidence < 0 {
		v.CategoryConfidence = 0
	}
	if v.CategoryConfidence > 1 {
		v.CategoryConfidence = 1
	}
	v.Result = strings.ToUpper(strings.TrimSpace(v.Result))
	switch v.Result {
	case "PASS", "REJECT", "REVIEW":
	default:
		v.Result = ""
	}
	v.RiskLevel = strings.ToLower(strings.TrimSpace(v.RiskLevel))
	switch v.RiskLevel {
	case "low", "medium", "high":
	default:
		switch {
		case v.Score >= 0.8:
			v.RiskLevel = "high"
		case v.Score >= 0.4:
			v.RiskLevel = "medium"
		default:
			v.RiskLevel = "low"
		}
	}
	if v.Score < 0 {
		v.Score = 0
	}
	if v.Score > 1 {
		v.Score = 1
	}
	if v.Result == "" {
		switch {
		case v.Score >= 0.8:
			v.Result = model.ResultReject
		case v.Score < 0.35:
			v.Result = model.ResultPass
		default:
			v.Result = model.ResultReview
		}
	}
	if v.Labels == nil {
		v.Labels = []string{}
	}
	if v.Reasons == nil {
		v.Reasons = []string{}
	}
}
