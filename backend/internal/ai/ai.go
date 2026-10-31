package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"smartdoc-vision/backend/internal/media"
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

func AdapterFor(kind string, loader *media.Loader) (Adapter, error) {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case model.ProviderOpenAI, "OPENAI":
		return &OpenAIAdapter{Loader: loader}, nil
	case model.ProviderAnthropic, "CLAUDE":
		return &AnthropicAdapter{Loader: loader}, nil
	case model.ProviderJEV, "TYPESAFE", "TYPESAFE_JEV":
		return &JEVAdapter{Loader: loader}, nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", kind)
	}
}

func resolveLoader(loader *media.Loader, fetcher *media.Fetcher) *media.Loader {
	if loader != nil {
		return loader
	}
	return media.NewLoader(fetcher, nil)
}

func imageLabel(img model.MediaItem, index int) string {
	if name := strings.TrimSpace(img.Name); name != "" {
		return name
	}
	if url := strings.TrimSpace(img.URL); url != "" && !strings.HasPrefix(url, "data:") {
		return url
	}
	return fmt.Sprintf("图片 %d", index+1)
}

// MaxOutputTokensLimit is a safety ceiling only. Individual providers still
// reject values above what their selected model actually supports.
const MaxOutputTokensLimit = 200000

// TreatyPlaceholder can appear in an audit review prompt. It is replaced by
// the compiled content of every READY treaty document at review time.
const TreatyPlaceholder = "{{AUDIT_TREATY}}"

const maxTreatyPromptRunes = 30000

// AuditTreatyPrompt returns the compiled treaty text that is allowed to be
// injected into a model prompt. Failed or empty documents are ignored.
func AuditTreatyPrompt(audit model.Audit) string {
	var treaties []model.AuditTreaty
	if err := model.DecodeJSON(audit.Treaties, &treaties); err != nil {
		return ""
	}
	parts := make([]string, 0, len(treaties))
	for _, treaty := range treaties {
		if treaty.Status != model.TreatyStatusReady {
			continue
		}
		content := strings.TrimSpace(treaty.Content)
		if content == "" {
			continue
		}
		name := strings.TrimSpace(treaty.Name)
		if name == "" {
			name = "未命名条约"
		}
		parts = append(parts, "《"+name+"》\n"+content)
	}
	if len(parts) == 0 {
		return ""
	}
	return truncatePromptRunes(strings.Join(parts, "\n\n"), maxTreatyPromptRunes,
		"\n…（审核条约内容过长已截断）")
}

// ApplyTreatyPrompt injects treaty content into a review prompt. When the
// prompt has no placeholder, the treaty is appended automatically so existing
// audit configurations also benefit from newly attached treaties.
func ApplyTreatyPrompt(requirements string, audit model.Audit) string {
	treaty := AuditTreatyPrompt(audit)
	if treaty == "" {
		return strings.ReplaceAll(requirements, TreatyPlaceholder, "（未配置审核条约）")
	}
	injected := "以下审核条约是本次判定的正式依据，必须优先遵守；条约未覆盖的事项再按审核要求判断，不得编造条款：\n" + treaty
	if strings.Contains(requirements, TreatyPlaceholder) {
		return strings.ReplaceAll(requirements, TreatyPlaceholder, injected)
	}
	return strings.TrimSpace(requirements) + "\n\n【审核条约】\n" + injected
}

func truncatePromptRunes(value string, limit int, suffix string) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + suffix
}

// ApplyCustomHeaders adds provider-defined headers (for example a session ID
// required by a proxy) without letting them replace the protocol's auth headers.
func ApplyCustomHeaders(req *http.Request, headers map[string]string) {
	hasSession := req.Header.Get("x-opencode-session") != ""
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch strings.ToLower(key) {
		case "authorization", "x-api-key", "content-type":
			continue
		}
		if strings.EqualFold(key, "x-opencode-session") {
			hasSession = true
		}
		if strings.EqualFold(value, "{{random}}") || strings.EqualFold(value, "auto") {
			value = uuid.NewString()
		}
		req.Header.Set(key, value)
	}
	// Some OpenCode-compatible gateways require a per-request session header.
	// Unknown headers are harmless for other providers, so attach one by default
	// and let an explicit custom header override it.
	if !hasSession {
		req.Header.Set("x-opencode-session", uuid.NewString())
	}
}

// WithRandomSessionHeader returns a copy of the provider whose gateway session
// header is a fresh UUID. Health probes must never reuse a fixed session ID.
func WithRandomSessionHeader(provider model.AIProvider) model.AIProvider {
	cfg := ParseConfig(provider)
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	for key := range cfg.Headers {
		if strings.EqualFold(key, "x-opencode-session") {
			delete(cfg.Headers, key)
			break
		}
	}
	cfg.Headers["x-opencode-session"] = uuid.NewString()
	if raw, err := json.Marshal(cfg); err == nil {
		provider.ConfigJSON = model.JSONText(raw)
	}
	return provider
}

func ParseConfig(p model.AIProvider) model.ProviderConfig {
	var cfg model.ProviderConfig
	_ = model.DecodeJSON(p.ConfigJSON, &cfg)
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2200
	}
	if cfg.MaxTokens > MaxOutputTokensLimit {
		cfg.MaxTokens = MaxOutputTokensLimit
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
		if !strings.EqualFold(m.Type, "image") {
			continue
		}
		if strings.TrimSpace(m.URL) != "" || strings.TrimSpace(m.Key) != "" {
			out = append(out, m)
			continue
		}
		// 内联 data URL 图片可以放在 content 字段；归一到 URL 让取图逻辑统一。
		if content := strings.TrimSpace(m.Content); strings.HasPrefix(content, "data:image/") {
			out = append(out, model.MediaItem{Type: m.Type, URL: content, Name: m.Name})
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
