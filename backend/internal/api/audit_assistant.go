package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/model"
)

type assistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type auditAssistantRequest struct {
	ProjectID     uint64             `json:"project_id"`
	AuditID       uint64             `json:"audit_id"`
	ProviderID    uint64             `json:"provider_id"`
	Messages      []assistantMessage `json:"messages"`
	CurrentConfig map[string]any     `json:"current_config"`
}

type assistantDraft struct {
	Name            string                `json:"name"`
	Code            string                `json:"code"`
	Description     string                `json:"description"`
	MediaTypes      []string              `json:"media_types"`
	DecisionMode    string                `json:"decision_mode"`
	Categories      []model.AuditCategory `json:"categories"`
	ProviderID      uint64                `json:"provider_id"`
	ReviewPrompt    string                `json:"review_prompt"`
	Rules           json.RawMessage       `json:"rules"`
	Routes          json.RawMessage       `json:"routes"`
	DefaultResult   string                `json:"default_result"`
	OnModelFail     string                `json:"on_model_fail"`
	DedupeWindowSec int                   `json:"dedupe_window_sec"`
}

// auditAssistant runs a guided interview and, once the model considers the
// answers sufficient, returns a complete audit-configuration draft. The
// frontend applies the draft through the normal audited create/update API.
func (s *Server) auditAssistant(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权配置审核")
		return
	}
	var req auditAssistantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if req.ProjectID == 0 {
		badRequest(c, "请选择项目")
		return
	}
	if !s.requireProjectAccess(c, req.ProjectID) {
		return
	}
	if len(req.Messages) == 0 {
		badRequest(c, "请输入需求描述")
		return
	}
	if len(req.Messages) > 24 {
		req.Messages = req.Messages[len(req.Messages)-24:]
	}
	provider, err := s.assistantProvider(req.ProjectID, req.ProviderID)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_PROVIDER_MISSING", err.Error(), nil)
		return
	}
	key, err := s.cipher.Decrypt(provider.APIKeyEnc)
	if err != nil {
		serverError(c, fmt.Errorf("模型密钥解密失败: %w", err))
		return
	}

	var currentAudit *model.Audit
	if req.AuditID > 0 {
		var audit model.Audit
		if err := s.db.Where("id = ? AND project_id = ? AND deleted_at IS NULL", req.AuditID, req.ProjectID).First(&audit).Error; err != nil {
			notFound(c, "要编辑的审核配置不存在或不属于当前项目")
			return
		}
		currentAudit = &audit
	}

	currentJSON := "无（新建审核配置）"
	if req.CurrentConfig != nil {
		if raw, err := json.Marshal(req.CurrentConfig); err == nil {
			currentJSON = truncateText(string(raw), 6000)
		}
	} else if currentAudit != nil {
		currentJSON = auditSnapshotJSON(*currentAudit)
	}

	systemPrompt := assistantSystemPrompt
	systemPrompt += "\n\n【当前项目】\n项目编码：" + s.projectCode(req.ProjectID) + "\n"
	if currentAudit != nil {
		systemPrompt += "正在编辑已有审核配置，编码保持为 " + currentAudit.Code + "，除非用户明确要求改名。\n"
	}
	systemPrompt += "\n【当前配置快照】\n" + currentJSON + "\n"

	messages := make([]ai.ChatMessage, 0, len(req.Messages))
	for _, item := range req.Messages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role != "assistant" {
			role = "user"
		}
		messages = append(messages, ai.ChatMessage{Role: role, Content: truncateText(content, 4000)})
	}
	if len(messages) == 0 {
		badRequest(c, "请输入需求描述")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	result, err := ai.Chat(ctx, ai.ChatRequest{
		Provider: *provider, APIKey: key, System: systemPrompt,
		Messages: messages, MaxTokens: 2600, Temperature: 0.25,
	})
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_FAILED", err.Error(), nil)
		return
	}
	reply, ready, draft, parseErr := parseAssistantReply(result.Content, provider.ID, req.ProjectID, req.AuditID, currentAudit)
	if parseErr != nil && ready {
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_CONFIG_INVALID", parseErr.Error(), gin.H{"raw": truncateText(result.Content, 3000)})
		return
	}
	response := gin.H{
		"reply": reply, "ready": ready, "model": result.Model,
		"provider_id": provider.ID, "provider_name": provider.Name,
		"mode": map[bool]string{true: "edit", false: "create"}[currentAudit != nil],
	}
	if currentAudit != nil {
		response["target"] = gin.H{"id": currentAudit.ID, "name": currentAudit.Name, "code": currentAudit.Code}
	}
	if ready {
		if req.AuditID == 0 {
			draft.Code = s.uniqueAssistantCode(req.ProjectID, draft.Code)
		}
		response["config"] = draft
	}
	ok(c, response)
}

func (s *Server) assistantProvider(projectID, preferred uint64) (*model.AIProvider, error) {
	if preferred != 0 {
		var provider model.AIProvider
		if err := s.db.Where("id = ? AND project_id = ? AND status = ?", preferred, projectID, model.ProviderActive).First(&provider).Error; err != nil {
			return nil, fmt.Errorf("所选模型供应商不可用，请先在模型接入中启用")
		}
		if strings.EqualFold(provider.Kind, model.ProviderJEV) {
			return nil, fmt.Errorf("AI 配置助手需要通用大模型（OpenAI 兼容或 Anthropic 协议）；JEV 是结构化决策模型，不能进行自由对话")
		}
		return &provider, nil
	}
	var providers []model.AIProvider
	if err := s.db.Where("project_id = ? AND status = ?", projectID, model.ProviderActive).Order("id asc").Find(&providers).Error; err != nil {
		return nil, err
	}
	for i := range providers {
		if strings.EqualFold(providers[i].Kind, model.ProviderOpenAI) || strings.EqualFold(providers[i].Kind, model.ProviderAnthropic) {
			return &providers[i], nil
		}
	}
	return nil, fmt.Errorf("当前项目没有可用的通用大模型（OpenAI 兼容或 Anthropic 协议），请先完成模型接入")
}

func (s *Server) uniqueAssistantCode(projectID uint64, code string) string {
	base := code
	if base == "" {
		base = fmt.Sprintf("ai_%d", time.Now().Unix()%1000000)
	}
	for attempt := 0; attempt < 50; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s_%d", base, attempt+1)
		}
		var count int64
		if err := s.db.Model(&model.Audit{}).Where("project_id = ? AND code = ?", projectID, candidate).Count(&count).Error; err != nil {
			return candidate
		}
		if count == 0 {
			return candidate
		}
	}
	return fmt.Sprintf("ai_%d", time.Now().UnixNano()%1000000000)
}

func (s *Server) projectCode(projectID uint64) string {
	var project model.Project
	if err := s.db.Select("code").First(&project, projectID).Error; err != nil {
		return fmt.Sprintf("#%d", projectID)
	}
	return project.Code
}

func auditSnapshotJSON(audit model.Audit) string {
	var mediaTypes, categories, rules, routes any
	_ = model.DecodeJSON(audit.MediaTypes, &mediaTypes)
	_ = model.DecodeJSON(audit.Categories, &categories)
	_ = model.DecodeJSON(audit.Rules, &rules)
	_ = model.DecodeJSON(audit.Routes, &routes)
	payload := map[string]any{
		"name": audit.Name, "code": audit.Code, "description": audit.Description,
		"media_types": mediaTypes, "decision_mode": audit.DecisionMode,
		"categories": categories, "review_prompt": audit.ReviewPrompt,
		"rules": rules, "routes": routes, "default_result": audit.DefaultResult,
		"on_model_fail": audit.OnModelFail, "dedupe_window_sec": audit.DedupeWindowSec,
	}
	raw, _ := json.Marshal(payload)
	return truncateText(string(raw), 6000)
}

func parseAssistantReply(content string, providerID, projectID, auditID uint64, currentAudit *model.Audit) (string, bool, assistantDraft, error) {
	text := strings.TrimSpace(content)
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return text, false, assistantDraft{}, nil
	}
	var raw struct {
		Reply  string `json:"reply"`
		Ready  bool   `json:"ready"`
		Config *struct {
			Name            string                `json:"name"`
			Code            string                `json:"code"`
			Description     string                `json:"description"`
			MediaTypes      []string              `json:"media_types"`
			DecisionMode    string                `json:"decision_mode"`
			Categories      []model.AuditCategory `json:"categories"`
			ReviewPrompt    string                `json:"review_prompt"`
			Rules           json.RawMessage       `json:"rules"`
			RulesPreset     string                `json:"rules_preset"`
			Routes          json.RawMessage       `json:"routes"`
			DefaultResult   string                `json:"default_result"`
			OnModelFail     string                `json:"on_model_fail"`
			DedupeWindowSec int                   `json:"dedupe_window_sec"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		return text, false, assistantDraft{}, nil
	}
	reply := strings.TrimSpace(raw.Reply)
	if reply == "" {
		reply = "我已整理好审核配置。"
	}
	if !raw.Ready || raw.Config == nil {
		return reply, false, assistantDraft{}, nil
	}
	draft := assistantDraft{
		Name: strings.TrimSpace(raw.Config.Name), Code: strings.ToLower(strings.TrimSpace(raw.Config.Code)),
		Description: strings.TrimSpace(raw.Config.Description), MediaTypes: raw.Config.MediaTypes,
		DecisionMode: normalizeDecisionMode(raw.Config.DecisionMode), Categories: raw.Config.Categories,
		ReviewPrompt: strings.TrimSpace(raw.Config.ReviewPrompt), DefaultResult: defaultResult(raw.Config.DefaultResult),
		OnModelFail: defaultResult(raw.Config.OnModelFail), DedupeWindowSec: dedupeWindow(&raw.Config.DedupeWindowSec),
		ProviderID: providerID,
	}
	if draft.DecisionMode == "" {
		draft.DecisionMode = model.DecisionReview
	}
	if draft.Name == "" || len([]rune(draft.Name)) < 2 {
		return "", false, assistantDraft{}, fmt.Errorf("模型生成的审核名称不完整，请补充更多信息后重试")
	}
	if len([]rune(draft.Name)) > 64 {
		draft.Name = string([]rune(draft.Name)[:64])
	}
	draft.Code = sanitizeAuditCode(draft.Code)
	if currentAudit != nil {
		draft.Code = currentAudit.Code
	} else if draft.Code == "" {
		draft.Code = fmt.Sprintf("ai_%d", time.Now().Unix()%1000000)
	}
	draft.MediaTypes = normalizeAssistantMediaTypes(draft.MediaTypes)
	if len(draft.Categories) == 0 && (draft.DecisionMode == model.DecisionClassify || draft.DecisionMode == model.DecisionClassifyOnly) {
		draft.Categories = catalog.ExampleClassificationCategories()
	}
	if draft.DecisionMode == model.DecisionClassifyOnly {
		draft.Categories = catalog.EnsureClassifierUnknown(draft.Categories)
	}
	filteredCategories := make([]model.AuditCategory, 0, len(draft.Categories))
	for _, category := range draft.Categories {
		category.Code = strings.ToLower(strings.TrimSpace(category.Code))
		category.Name = strings.TrimSpace(category.Name)
		category.Description = strings.TrimSpace(category.Description)
		category.Result = defaultResult(category.Result)
		if category.Code != "" && category.Name != "" {
			filteredCategories = append(filteredCategories, category)
		}
	}
	draft.Categories = filteredCategories
	if draft.DecisionMode == model.DecisionClassify && len(draft.Categories) < 2 {
		return "", false, assistantDraft{}, fmt.Errorf("分类审核至少需要两个有效分类")
	}
	if draft.DecisionMode == model.DecisionClassifyOnly && len(draft.Categories) < 1 {
		return "", false, assistantDraft{}, fmt.Errorf("纯分类器至少需要一个有效分类")
	}
	if draft.ReviewPrompt == "" {
		switch draft.DecisionMode {
		case model.DecisionClassifyOnly:
			draft.ReviewPrompt = catalog.DefaultClassifyPrompt + "\n\n这是纯分类器任务：只返回最匹配的分类类型，不需要判定通过或驳回，平台会把成功分类的结论固定为 PASS。"
		case model.DecisionClassify:
			draft.ReviewPrompt = catalog.DefaultClassifyPrompt
		default:
			draft.ReviewPrompt = catalog.DefaultReviewPrompt
		}
	}
	if len(raw.Config.Rules) > 0 && string(raw.Config.Rules) != "null" {
		var rules []any
		if json.Unmarshal(raw.Config.Rules, &rules) == nil && len(rules) > 0 && validateRulesJSON(raw.Config.Rules) == nil {
			draft.Rules = raw.Config.Rules
		}
	}
	if len(draft.Rules) == 0 || string(draft.Rules) == "[]" {
		if draft.DecisionMode == model.DecisionClassifyOnly {
			draft.Rules = json.RawMessage("[]")
		} else if draft.DecisionMode == model.DecisionClassify {
			draft.Rules = json.RawMessage(catalog.ClassificationRulesPayload(0.60))
		} else {
			preset := strings.ToLower(strings.TrimSpace(raw.Config.RulesPreset))
			if preset != "strict" && preset != "loose" {
				preset = "standard"
			}
			draft.Rules = json.RawMessage(catalog.RulesPayload(preset))
		}
	}
	if len(raw.Config.Routes) > 0 && string(raw.Config.Routes) != "null" {
		var routes map[string]any
		if json.Unmarshal(raw.Config.Routes, &routes) == nil && len(routes) > 0 {
			draft.Routes = raw.Config.Routes
		}
	}
	if len(draft.Routes) == 0 || string(draft.Routes) == "{}" {
		draft.Routes = json.RawMessage(catalog.DefaultRoutes())
	}
	if draft.DedupeWindowSec <= 0 {
		draft.DedupeWindowSec = 86400
	}
	return reply, true, draft, nil
}

func sanitizeAuditCode(value string) string {
	cleaned := regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(strings.ToLower(value), "_")
	cleaned = strings.Trim(cleaned, "_-")
	if len(cleaned) > 32 {
		cleaned = cleaned[:32]
	}
	if cleaned == "" || cleaned[0] < 'a' || cleaned[0] > 'z' {
		return ""
	}
	return cleaned
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func normalizeAssistantMediaTypes(types []string) []string {
	out := make([]string, 0, len(types))
	for _, item := range types {
		item = strings.ToLower(strings.TrimSpace(item))
		if (item == "text" || item == "image") && !containsString(out, item) {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		out = []string{"text", "image"}
	}
	return out
}

func extractJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		if newline := strings.IndexByte(value, '\n'); newline >= 0 {
			value = value[newline+1:]
		}
		value = strings.TrimSuffix(strings.TrimSpace(value), "```")
	}
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(value[start : end+1])
}

const assistantSystemPrompt = `你是「智审图档 SmartDoc Vision」的审核配置顾问。你的任务是通过简短问答，帮助管理员创建或修改一条图档审核配置。

【平台能力边界】
- 仅支持多文本 + 多图片审核，不支持视频/音频。
- decision_mode 只能是 REVIEW（合规审核）、CLASSIFY（分类并映射结论）或 CLASSIFY_ONLY（纯分类器，全部通过、只返回类型）。
- 分类审核需要提供至少两个分类；纯分类器可提供任意数量分类，分类结果始终 PASS，不需要映射驳回。
- 可绑定通用大模型（OpenAI 兼容 / Anthropic 协议）或 JEV 决策模型；分类场景优先建议 JEV。
- rules 是规则组数组；routes 是 PASS / REJECT / REVIEW 三路结果动作对象，通常保持空动作，除非用户明确要求。

【对话规则】
1. 用简体中文，每次最多问 2~3 个最关键的问题，不要一次罗列冗长清单。
2. 先确认：审核对象/业务场景、通过标准、驳回红线、是否需要分类、是否需要人工复核、触发方式是否已知（触发方式不属于配置字段，可只在回复中提及）。
3. 如果用户回答“都行/你决定/直接生成/开始生成”，立即采用合理默认值并输出完整配置，不要继续追问。
4. 最多进行 3 轮提问；信息足够时直接生成。不要声称已经保存，保存由前端执行。
5. 你的每一轮回复都必须是严格 JSON 对象，不要输出 Markdown 代码块、解释或前后缀。

【每轮输出 JSON】
未准备好时：
{"reply":"向用户提出的问题或说明","ready":false}
准备好时：
{"reply":"简短说明生成了什么配置","ready":true,"config":{
  "name":"2~64字审核名称",
  "code":"小写字母开头的英文编码，如 contract_review",
  "description":"审核用途说明",
  "media_types":["text","image"],
  "decision_mode":"REVIEW",
  "categories":[],
  "review_prompt":"发送给模型的完整审核要求，包含审核重点、通过/驳回边界、不确定时转人工",
  "default_result":"REVIEW",
  "on_model_fail":"REVIEW",
  "dedupe_window_sec":86400,
  "rules_preset":"standard",
  "rules":[],
  "routes":{"PASS":{"exec_mode":"sequential","on_error":"continue","actions":[]},"REJECT":{"exec_mode":"sequential","on_error":"continue","actions":[]},"REVIEW":{"exec_mode":"sequential","on_error":"to_review","actions":[]}}
}}

【字段约束】
- media_types 仅可包含 text、image。
- rules_preset 可选 standard / strict / loose；分类模式由服务端使用分类规则。rules 可留空数组，由平台补默认规则。
- 分类模式下 categories 必须给出至少两个分类；纯分类器可给任意数量类型，result 会被平台统一为 PASS。
- 纯分类器使用 decision_mode=CLASSIFY_ONLY，categories 中每项仍填写 code/name/description，result 写 PASS 即可。
- routes 的键必须是 PASS、REJECT、REVIEW；动作默认空数组。
- 编辑已有配置时，只修改用户要求的部分，保留未提及的字段和名称；code 由平台保持原值。
- 不要输出任何密钥、内网地址或虚构的业务事实。`
