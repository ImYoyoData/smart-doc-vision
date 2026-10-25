package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
)

// JEVAdapter implements TypeSafe AI's System One endpoint:
// POST /v1/systemone with a state plus typed questions (choice/noul).
// Images are declared in assets and referenced from state.
type JEVAdapter struct {
	Client  *http.Client
	Fetcher *media.Fetcher
}

type jevRequest struct {
	Model     string                   `json:"model"`
	State     any                      `json:"state"`
	Assets    map[string]jevImageAsset `json:"assets,omitempty"`
	Questions map[string]jevQuestion   `json:"questions"`
}

type jevImageAsset struct {
	Type       string `json:"type"`
	Format     string `json:"format"`
	DataBase64 string `json:"data_base64"`
}

type jevQuestion struct {
	Type         string         `json:"type"`
	Instructions string         `json:"instructions"`
	Criteria     map[string]any `json:"criteria,omitempty"`
}

type jevResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   map[string]any             `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type jevChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type jevNoulAnswer struct {
	Type       string  `json:"type"`
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}

func (a *JEVAdapter) Review(ctx context.Context, req Request) (*model.Verdict, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("JEV 的 TypeSafe API Key 未配置")
	}
	cfg := ParseConfig(req.Provider)
	text := CollectText(req.Input)
	if len([]rune(text)) > 18000 {
		text = string([]rune(text)[:18000]) + "\n…（内容已截断）"
	}
	images := CollectImages(req.Input)
	if len(images) > cfg.MaxImages {
		images = images[:cfg.MaxImages]
	}
	if text == "" && len(images) == 0 {
		return nil, fmt.Errorf("JEV 审核需要文本或图片")
	}

	categories := AuditCategories(req.Audit)
	classify := IsClassification(req.Audit) && len(categories) > 0
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = media.NewFetcher(false)
	}
	assets := map[string]jevImageAsset{}
	state := map[string]any{
		"task": map[string]any{
			"biz_key": req.Input.BizKey,
			"ext":     req.Input.Ext,
		},
		"document_text":      text,
		"audit_requirements": req.Audit.ReviewPrompt,
		"provider_note":      req.Provider.Prompt,
		"decision_mode":      req.Audit.DecisionMode,
	}
	if classify {
		state["categories"] = categories
	}
	if req.Audit.ReviewPrompt == "" {
		state["audit_requirements"] = "审核文档内容安全、完整性与敏感信息；无法确认时转人工复核。"
	}
	var imageRefs []string
	for i, img := range images {
		prepared, err := fetcher.FetchImage(ctx, img.URL)
		if err != nil {
			return nil, fmt.Errorf("准备第 %d 张图片失败: %w", i+1, err)
		}
		ref := "img-" + uuid.NewString()
		assets[ref] = jevImageAsset{
			Type:       "image",
			Format:     prepared.Format,
			DataBase64: base64.StdEncoding.EncodeToString(prepared.Data),
		}
		if i == 0 {
			state["document_image"] = ref
		} else {
			imageRefs = append(imageRefs, ref)
		}
	}
	if len(imageRefs) > 0 {
		state["additional_images"] = imageRefs
	}

	questions := map[string]jevQuestion{}
	if classify {
		instruction := "根据 state 中的 document_text、图片、audit_requirements 和 categories，从给定分类中选择唯一最匹配的编码。不要创建新类别；证据不足时选择 other（若有）并降低置信度。"
		if IsClassifyOnly(req.Audit) {
			instruction += " 这是纯分类器任务，平台会将最终审核结论固定为 PASS；只关注分类结果。"
		}
		questions["document_category"] = jevQuestion{
			Type:         "choice",
			Instructions: instruction,
			Criteria:     CategoryCriteria(categories),
		}
	} else {
		questions["audit_decision"] = jevQuestion{
			Type:         "choice",
			Instructions: "根据 state 中的 document_text、图片和 audit_requirements，判断该图档的最终审核结论。只有在证据明确时选择 PASS 或 REJECT；不确定时选择 REVIEW。",
			Criteria: map[string]any{
				"PASS":   "图档清晰完整，未发现违规、敏感信息、伪造篡改或明显缺页，可安全放行。",
				"REVIEW": "图档存在模糊、遮挡、缺页、信息不足或无法确认的内容，需要人工复核。",
				"REJECT": "图档明确包含违法违规、敏感或不当内容，或存在明显伪造、篡改、严重缺失，应拒绝。",
			},
		}
	}
	questions["sensitive_content"] = jevQuestion{
		Type:         "noul",
		Instructions: "文档中是否包含需要拦截的违法、色情、暴力、政治敏感、广告导流或明确不应披露的个人敏感信息？",
	}
	questions["document_incomplete"] = jevQuestion{
		Type:         "noul",
		Instructions: "文档是否模糊、缺页、遮挡、关键要素无法辨认，因而无法可靠地完成审核？",
	}
	body := jevRequest{
		Model:     req.Provider.Model,
		State:     state,
		Assets:    assets,
		Questions: questions,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := jevEndpoint(req.Provider.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	if len(assets) > 0 {
		httpReq.Header.Set("TypeSafe-Danger-Zone-Images", "true")
	}
	ApplyCustomHeaders(httpReq, ParseConfig(req.Provider).Headers)
	start := time.Now()
	resp, err := a.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 JEV 失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	latency := time.Since(start).Milliseconds()
	var parsed jevResponse
	_ = json.Unmarshal(raw, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := truncate(strings.TrimSpace(string(raw)), 400)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, fmt.Errorf("JEV 返回 HTTP %d: %s", resp.StatusCode, msg)
	}
	sensitive, _ := parseJEVNoul(parsed.Answers["sensitive_content"])
	incomplete, _ := parseJEVNoul(parsed.Answers["document_incomplete"])
	if classify {
		choice, err := parseJEVChoice(parsed.Answers["document_category"], "document_category")
		if err != nil {
			return nil, fmt.Errorf("JEV 未返回有效分类: %w", err)
		}
		return jevClassificationVerdict(parsed, choice, categories, sensitive, incomplete, assets, latency, req), nil
	}

	choice, err := parseJEVChoice(parsed.Answers["audit_decision"], "audit_decision")
	if err != nil {
		return nil, fmt.Errorf("JEV 未返回有效裁决: %w", err)
	}

	decision := strings.ToUpper(strings.TrimSpace(choice.Choice))
	rejectProb := choice.Probabilities["REJECT"]
	risk := sensitive*0.55 + incomplete*0.2 + rejectProb*0.25 + (1-choice.Confidence)*0.1
	if risk > 1 {
		risk = 1
	}
	result := model.ResultReview
	switch {
	case decision == model.ResultReject || sensitive >= 0.75 || rejectProb >= 0.7:
		result = model.ResultReject
	case decision == model.ResultPass && sensitive < 0.35 && incomplete < 0.4:
		result = model.ResultPass
	case decision == model.ResultReview || incomplete >= 0.5:
		result = model.ResultReview
	}
	labels := []string{"JEV结构化裁决"}
	reasons := []string{
		fmt.Sprintf("JEV 裁决=%s，置信度=%.2f", decision, choice.Confidence),
		fmt.Sprintf("敏感内容概率=%.2f，文档不完整概率=%.2f", sensitive, incomplete),
	}
	if sensitive >= 0.5 {
		labels = append(labels, "疑似敏感内容")
	}
	if incomplete >= 0.5 {
		labels = append(labels, "文档不完整")
	}
	summary := "JEV 判定图档可放行"
	if result == model.ResultReject {
		summary = "JEV 判定图档存在高风险内容"
	} else if result == model.ResultReview {
		summary = "JEV 判定需要人工复核"
	}
	rawMap := map[string]any{
		"decision":            choice,
		"sensitive_content":   sensitive,
		"document_incomplete": incomplete,
		"usage":               parsed.Usage,
	}
	verdict := &model.Verdict{
		Result:    result,
		RiskLevel: riskLevel(risk),
		Score:     risk,
		Labels:    labels,
		Summary:   summary,
		Reasons:   reasons,
		Raw:       rawMap,
		Provider:  "JEV",
		Model:     parsed.Model,
		LatencyMS: latency,
		Meta: map[string]any{
			"decision_confidence": choice.Confidence,
			"image_count":         len(assets),
		},
	}
	if verdict.Model == "" {
		verdict.Model = req.Provider.Model
	}
	applyUsageToVerdict(verdict, usageFromMap(parsed.Usage))
	normalizeVerdict(verdict)
	return verdict, nil
}

func jevClassificationVerdict(parsed jevResponse, choice jevChoiceAnswer, categories []model.AuditCategory, sensitive, incomplete float64, assets map[string]jevImageAsset, latency int64, req Request) *model.Verdict {
	code := strings.ToLower(strings.TrimSpace(choice.Choice))
	category, found := FindCategory(categories, code)
	result := model.ResultReview
	categoryName := category.Name
	if !found {
		categoryName = "未知分类"
		code = strings.ToLower(code)
	} else if category.Result == model.ResultPass || category.Result == model.ResultReject || category.Result == model.ResultReview {
		result = category.Result
	}

	reasons := []string{
		fmt.Sprintf("JEV 分类=%s / %s，置信度=%.2f", code, categoryName, choice.Confidence),
		fmt.Sprintf("敏感内容概率=%.2f，文档不完整概率=%.2f", sensitive, incomplete),
	}
	if !found {
		result = model.ResultReview
		reasons = append(reasons, "分类编码不在当前审核配置的分类列表中，已转人工复核")
	}
	if sensitive >= 0.75 {
		result = model.ResultReject
		reasons = append(reasons, "敏感内容概率过高，覆盖分类映射并强制驳回")
	} else if incomplete >= 0.6 && result == model.ResultPass {
		result = model.ResultReview
		reasons = append(reasons, "文档不完整概率较高，通过结果转人工复核")
	}

	baseRisk := map[string]float64{
		model.ResultPass:   0.12,
		model.ResultReject: 0.85,
		model.ResultReview: 0.5,
	}[result]
	risk := baseRisk*0.6 + sensitive*0.3 + incomplete*0.1 + (1-choice.Confidence)*0.1
	if sensitive >= 0.75 && risk < 0.85 {
		risk = 0.85
	}
	if risk > 1 {
		risk = 1
	}

	labels := []string{"JEV分类裁决"}
	if categoryName != "" {
		labels = append(labels, "分类:"+categoryName)
	}
	if sensitive >= 0.5 {
		labels = append(labels, "疑似敏感内容")
	}
	if incomplete >= 0.5 {
		labels = append(labels, "文档不完整")
	}
	summary := fmt.Sprintf("JEV 分类为「%s」，映射结论 %s", categoryName, result)
	if !found {
		summary = "JEV 返回的分类不在配置中，已转人工复核"
	}
	rawMap := map[string]any{
		"category_choice":     choice,
		"sensitive_content":   sensitive,
		"document_incomplete": incomplete,
		"usage":               parsed.Usage,
	}
	verdict := &model.Verdict{
		Result:             result,
		RiskLevel:          riskLevel(risk),
		Score:              risk,
		Labels:             labels,
		Summary:            summary,
		Reasons:            reasons,
		Category:           code,
		CategoryName:       categoryName,
		CategoryConfidence: choice.Confidence,
		Raw:                rawMap,
		Provider:           "JEV",
		Model:              parsed.Model,
		LatencyMS:          latency,
		Meta: map[string]any{
			"category_confidence":    choice.Confidence,
			"category_probabilities": choice.Probabilities,
			"image_count":            len(assets),
		},
	}
	if verdict.Model == "" {
		verdict.Model = req.Provider.Model
	}
	applyUsageToVerdict(verdict, usageFromMap(parsed.Usage))
	normalizeVerdict(verdict)
	return verdict
}

func (a *JEVAdapter) Test(ctx context.Context, req Request) (TestResult, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return TestResult{OK: false, Message: "TypeSafe API Key 未配置"}, fmt.Errorf("TypeSafe API Key 未配置")
	}
	body := jevRequest{
		Model: req.Provider.Model,
		State: "SmartDoc Vision connectivity test",
		Questions: map[string]jevQuestion{
			"connectivity": {
				Type:         "noul",
				Instructions: "Is this a connectivity test message?",
			},
		},
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, jevEndpoint(req.Provider.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return TestResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	ApplyCustomHeaders(httpReq, ParseConfig(req.Provider).Headers)
	start := time.Now()
	resp, err := a.client().Do(httpReq)
	if err != nil {
		return TestResult{}, fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	elapsed := time.Since(start).Milliseconds()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TestResult{OK: false, LatencyMS: elapsed, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))},
			fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var parsed jevResponse
	_ = json.Unmarshal(raw, &parsed)
	if _, err := parseJEVNoul(parsed.Answers["connectivity"]); err != nil {
		return TestResult{OK: false, LatencyMS: elapsed, Message: "响应结构不符合 System One 规范"}, fmt.Errorf("响应结构不符合 System One 规范")
	}
	return TestResult{OK: true, LatencyMS: elapsed, Message: "System One 连接成功，Noul 返回正常", Model: parsed.Model}, nil
}

func (a *JEVAdapter) client() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return &http.Client{Timeout: 120 * time.Second}
}

func jevEndpoint(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/systemone") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/systemone"
	}
	return base + "/v1/systemone"
}

func parseJEVChoice(raw json.RawMessage, field string) (jevChoiceAnswer, error) {
	var out jevChoiceAnswer
	if len(raw) == 0 {
		return out, fmt.Errorf("缺少 %s", field)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if out.Choice == "" {
		return out, fmt.Errorf("choice 为空")
	}
	if out.Probabilities == nil {
		out.Probabilities = map[string]float64{}
	}
	return out, nil
}

func parseJEVNoul(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("缺少 noul")
	}
	var out jevNoulAnswer
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	return out.Noul, nil
}

func riskLevel(score float64) string {
	switch {
	case score >= 0.75:
		return "high"
	case score >= 0.4:
		return "medium"
	default:
		return "low"
	}
}
