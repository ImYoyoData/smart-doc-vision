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
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images"`
}

type auditAssistantRequest struct {
	ProjectID     uint64             `json:"project_id"`
	AuditID       uint64             `json:"audit_id"`
	ProviderID    uint64             `json:"provider_id"`
	Messages      []assistantMessage `json:"messages"`
	CurrentConfig map[string]any     `json:"current_config"`
}

type assistantDraft struct {
	Name            string                 `json:"name"`
	Code            string                 `json:"code"`
	Description     string                 `json:"description"`
	MediaTypes      []string               `json:"media_types"`
	DecisionMode    string                 `json:"decision_mode"`
	Categories      []model.AuditCategory  `json:"categories"`
	ProviderID      uint64                 `json:"provider_id"`
	ReviewPrompt    string                 `json:"review_prompt"`
	Rules           json.RawMessage        `json:"rules"`
	Routes          json.RawMessage        `json:"routes"`
	DefaultResult   string                 `json:"default_result"`
	OnModelFail     string                 `json:"on_model_fail"`
	DedupeWindowSec int                    `json:"dedupe_window_sec"`
	Triggers        map[string]string      `json:"triggers,omitempty"`
	RedisTrigger    *assistantRedisTrigger `json:"redis_trigger,omitempty"`
}

type assistantRedisTrigger struct {
	ConnectionID  uint64 `json:"connection_id"`
	DB            int    `json:"db"`
	Stream        string `json:"stream"`
	ConsumerGroup string `json:"consumer_group"`
	StartPosition string `json:"start_position"`
	BatchSize     int    `json:"batch_size"`
	MaxInflight   int    `json:"max_inflight"`
}

// auditAssistant runs a guided interview and, once the model considers the
// answers sufficient, returns a complete audit-configuration draft. The
// frontend applies the draft through the normal audited create/update API.
func (s *Server) auditAssistant(c *gin.Context) {
	s.handleAuditAssistant(c, false)
}

// auditAssistantStream exposes the same interview over SSE so the page can show
// the real pipeline (context read, provider choice, model call, draft parsing)
// while it is running.
func (s *Server) auditAssistantStream(c *gin.Context) {
	s.handleAuditAssistant(c, true)
}

func (s *Server) handleAuditAssistant(c *gin.Context, stream bool) {
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
	providerConfig := model.ProviderConfig{}
	_ = model.DecodeJSON(provider.ConfigJSON, &providerConfig)
	maxImages := providerConfig.MaxImages
	if maxImages <= 0 {
		maxImages = 4
	}
	if maxImages > 8 {
		maxImages = 8
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

	currentTriggers := map[string]string{}
	if currentAudit != nil {
		var triggers []model.TriggerSource
		_ = s.db.Where("audit_id = ?", currentAudit.ID).Order("priority asc").Find(&triggers).Error
		for _, trigger := range triggers {
			currentTriggers[trigger.Type] = trigger.Status
		}
	}

	messages := make([]ai.ChatMessage, 0, len(req.Messages))
	totalImages := 0
	for _, item := range req.Messages {
		content := strings.TrimSpace(item.Content)
		images, imageErr := normalizeAssistantImages(item.Images)
		if imageErr != nil {
			badRequest(c, imageErr.Error())
			return
		}
		if content == "" && len(images) == 0 {
			continue
		}
		totalImages += len(images)
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role != "assistant" {
			role = "user"
		}
		messages = append(messages, ai.ChatMessage{
			Role: role, Content: truncateText(content, 4000), Images: images,
		})
	}
	if len(messages) == 0 {
		badRequest(c, "请输入需求描述")
		return
	}
	if totalImages > maxImages {
		badRequest(c, fmt.Sprintf("一次对话最多支持 %d 张图片", maxImages))
		return
	}
	if totalImages > 0 && !providerConfig.SupportsVision {
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_VISION_UNSUPPORTED",
			"当前通用大模型未开启图片理解，请到「模型接入」开启视觉能力或更换支持视觉的模型", nil)
		return
	}

	currentJSON := "无（新建审核配置）"
	if req.CurrentConfig != nil {
		if raw, err := json.Marshal(req.CurrentConfig); err == nil {
			currentJSON = truncateText(string(raw), 6000)
		}
	} else if currentAudit != nil {
		currentJSON = auditSnapshotJSON(*currentAudit, currentTriggers)
	}

	var redisConnections []model.RedisConnection
	_ = s.db.Where("project_id = ? AND status = ? AND last_test_ok = ? AND health = ?",
		req.ProjectID, model.RedisConnectionActive, true, model.RedisHealthOK).
		Order("id asc").Find(&redisConnections).Error
	allowedRedis := make(map[uint64]bool, len(redisConnections))

	systemPrompt := assistantSystemPrompt
	systemPrompt += "\n\n【当前项目】\n项目编码：" + s.projectCode(req.ProjectID) + "\n"
	if len(redisConnections) == 0 {
		systemPrompt += "【可用 Redis 连接】\n无。禁止启用 REDIS；如果用户要求 Redis 接入，请在 reply 中提示先到「数据连接」创建并测试 Redis 连接。\n"
	} else {
		systemPrompt += "【可用 Redis 连接】\n"
		for _, connection := range redisConnections {
			allowedRedis[connection.ID] = true
			systemPrompt += fmt.Sprintf("id=%d name=%s addr=%s\n", connection.ID, connection.Name, connection.Addr)
		}
	}
	if currentAudit != nil {
		systemPrompt += "正在编辑已有审核配置，编码保持为 " + currentAudit.Code + "，除非用户明确要求改名。\n"
		systemPrompt += "\n【当前接入方式（触发源）】\n" + assistantTriggerSnapshot(currentTriggers) + "\n"
	} else {
		systemPrompt += "\n【当前接入方式（触发源）】\n新建审核默认：OPENAPI=ACTIVE，REDIS/WEBSOCKET=DISABLED。\n"
	}
	systemPrompt += "\n【当前配置快照】\n" + currentJSON + "\n"

	steps := make([]assistantStep, 0, 8)
	emit := func(step assistantStep) {
		step.At = time.Now().UnixMilli()
		updated := false
		for i := range steps {
			if steps[i].ID == step.ID {
				steps[i] = step
				updated = true
				break
			}
		}
		if !updated {
			steps = append(steps, step)
		}
		if stream {
			writeAssistantSSE(c, "step", step)
		}
	}
	if stream {
		prepareAssistantSSE(c)
	}

	contextDetail := "新建审核配置"
	if currentAudit != nil {
		contextDetail = fmt.Sprintf("编辑「%s」（%s）", currentAudit.Name, currentAudit.Code)
	}
	emit(assistantStep{ID: "context", Label: "读取当前配置", Status: "done", Detail: contextDetail})
	emit(assistantStep{ID: "provider", Label: "选择通用大模型", Status: "done", Detail: provider.Name + " · " + provider.Model})
	if totalImages > 0 {
		emit(assistantStep{ID: "images", Label: "读取粘贴图片", Status: "done", Detail: fmt.Sprintf("%d 张图片", totalImages)})
	}

	emit(assistantStep{ID: "thinking", Label: "调用大模型思考", Status: "running"})
	started := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 600*time.Second)
	defer cancel()
	result, err := ai.Chat(ctx, ai.ChatRequest{
		Provider: *provider, APIKey: key, System: systemPrompt,
		Messages: messages, MaxTokens: 2600, Temperature: 0.25,
	})
	if err != nil {
		s.recordProviderUsage(*provider, model.UsageSourceAuditAssistant, req.AuditID, nil, time.Since(started), err)
		emit(assistantStep{ID: "thinking", Label: "调用大模型思考", Status: "error", Detail: err.Error()})
		if stream {
			writeAssistantSSE(c, "error", gin.H{"code": "ASSISTANT_FAILED", "message": err.Error()})
			return
		}
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_FAILED", err.Error(), nil)
		return
	}
	emit(assistantStep{ID: "thinking", Label: "调用大模型思考", Status: "done",
		Detail: fmt.Sprintf("%s · %.1fs", result.Model, time.Since(started).Seconds())})
	s.recordProviderUsage(*provider, model.UsageSourceAuditAssistant, req.AuditID, result, time.Since(started), nil)

	emit(assistantStep{ID: "draft", Label: "解析并校验配置", Status: "running"})
	reply, ready, draft, parseErr := parseAssistantReply(result.Content, provider.ID, req.ProjectID, req.AuditID, currentAudit, currentTriggers)
	if ready && draft.RedisTrigger != nil && !allowedRedis[draft.RedisTrigger.ConnectionID] {
		parseErr = fmt.Errorf("模型选择了一个当前项目不可用或未通过测试的 Redis 连接，请先在「数据连接」创建并测试")
	}
	if parseErr != nil && ready {
		emit(assistantStep{ID: "draft", Label: "解析并校验配置", Status: "error", Detail: parseErr.Error()})
		extra := gin.H{"raw": truncateText(result.Content, 3000)}
		if stream {
			writeAssistantSSE(c, "error", gin.H{"code": "ASSISTANT_CONFIG_INVALID", "message": parseErr.Error(), "raw": extra["raw"]})
			return
		}
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_CONFIG_INVALID", parseErr.Error(), extra)
		return
	}
	draftDetail := "信息不足，继续提问"
	if ready {
		draftDetail = assistantDecisionLabel(draft.DecisionMode)
		if len(draft.Triggers) > 0 {
			draftDetail += " · " + assistantTriggerSnapshot(draft.Triggers)
		}
	}
	emit(assistantStep{ID: "draft", Label: "解析并校验配置", Status: "done", Detail: draftDetail})
	if ready && len(draft.Triggers) > 0 {
		emit(assistantStep{ID: "triggers", Label: "规划接入方式", Status: "done", Detail: assistantTriggerSnapshot(draft.Triggers)})
	}

	response := gin.H{
		"reply": reply, "ready": ready, "model": result.Model,
		"provider_id": provider.ID, "provider_name": provider.Name,
		"mode":   map[bool]string{true: "edit", false: "create"}[currentAudit != nil],
		"steps":  steps,
		"images": totalImages,
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
	if stream {
		writeAssistantSSE(c, "reply", response)
		writeAssistantSSE(c, "done", gin.H{})
		return
	}
	ok(c, response)
}

type assistantStep struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
	At     int64  `json:"at"`
}

func prepareAssistantSSE(c *gin.Context) {
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()
}

func writeAssistantSSE(c *gin.Context, event string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw)
	c.Writer.Flush()
}

func normalizeAssistantImages(values []string) ([]ai.ChatImage, error) {
	out := make([]ai.ChatImage, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, _, ok := ai.ParseImageDataURL(value); !ok {
			return nil, fmt.Errorf("图片格式不支持或超过 8MB，请粘贴 PNG/JPEG/WebP/GIF 图片")
		}
		out = append(out, ai.ChatImage{DataURL: value})
	}
	return out, nil
}

func assistantDecisionLabel(mode string) string {
	switch mode {
	case model.DecisionClassifyOnly:
		return "纯分类器配置"
	case model.DecisionClassify:
		return "分类审核配置"
	default:
		return "合规审核配置"
	}
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
	if err := s.db.Where("project_id = ? AND status = ?", projectID, model.ProviderActive).
		Order("priority asc, id asc").Find(&providers).Error; err != nil {
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

func auditSnapshotJSON(audit model.Audit, triggers map[string]string) string {
	var mediaTypes, categories, rules, routes any
	var treaties []model.AuditTreaty
	_ = model.DecodeJSON(audit.MediaTypes, &mediaTypes)
	_ = model.DecodeJSON(audit.Categories, &categories)
	_ = model.DecodeJSON(audit.Rules, &rules)
	_ = model.DecodeJSON(audit.Routes, &routes)
	_ = model.DecodeJSON(audit.Treaties, &treaties)
	treatySummary := make([]map[string]any, 0, len(treaties))
	for _, treaty := range treaties {
		if treaty.Status != model.TreatyStatusReady {
			continue
		}
		treatySummary = append(treatySummary, map[string]any{
			"name": treaty.Name, "content_chars": treaty.ContentChars,
		})
	}
	payload := map[string]any{
		"name": audit.Name, "code": audit.Code, "description": audit.Description,
		"media_types": mediaTypes, "decision_mode": audit.DecisionMode,
		"categories": categories, "review_prompt": audit.ReviewPrompt,
		"rules": rules, "routes": routes, "default_result": audit.DefaultResult,
		"on_model_fail": audit.OnModelFail, "dedupe_window_sec": audit.DedupeWindowSec,
	}
	if len(treatySummary) > 0 {
		payload["treaties"] = treatySummary
	}
	if len(triggers) > 0 {
		payload["triggers"] = triggers
	}
	raw, _ := json.Marshal(payload)
	return truncateText(string(raw), 6000)
}

func parseAssistantReply(content string, providerID, projectID, auditID uint64, currentAudit *model.Audit, currentTriggers map[string]string) (string, bool, assistantDraft, error) {
	text := strings.TrimSpace(content)
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return text, false, assistantDraft{}, nil
	}
	var raw struct {
		Reply  string `json:"reply"`
		Ready  bool   `json:"ready"`
		Config *struct {
			Name            string                 `json:"name"`
			Code            string                 `json:"code"`
			Description     string                 `json:"description"`
			MediaTypes      []string               `json:"media_types"`
			DecisionMode    string                 `json:"decision_mode"`
			Categories      []model.AuditCategory  `json:"categories"`
			ReviewPrompt    string                 `json:"review_prompt"`
			Rules           json.RawMessage        `json:"rules"`
			RulesPreset     string                 `json:"rules_preset"`
			Routes          json.RawMessage        `json:"routes"`
			DefaultResult   string                 `json:"default_result"`
			OnModelFail     string                 `json:"on_model_fail"`
			DedupeWindowSec int                    `json:"dedupe_window_sec"`
			Triggers        map[string]string      `json:"triggers"`
			RedisTrigger    *assistantRedisTrigger `json:"redis_trigger"`
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
	draft.Triggers = normalizeAssistantTriggers(raw.Config.Triggers, auditID == 0, currentTriggers)
	if raw.Config.RedisTrigger != nil {
		redisTrigger := *raw.Config.RedisTrigger
		redisTrigger.Stream = strings.TrimSpace(redisTrigger.Stream)
		redisTrigger.ConsumerGroup = strings.TrimSpace(redisTrigger.ConsumerGroup)
		if redisTrigger.DB < 0 || redisTrigger.DB > 255 {
			redisTrigger.DB = 0
		}
		if redisTrigger.StartPosition != "0" {
			redisTrigger.StartPosition = "$"
		}
		if redisTrigger.BatchSize < 1 || redisTrigger.BatchSize > 32 {
			redisTrigger.BatchSize = 16
		}
		if redisTrigger.MaxInflight < 1 || redisTrigger.MaxInflight > 32 {
			redisTrigger.MaxInflight = 1
		}
		if redisTrigger.ConnectionID > 0 && redisTrigger.Stream != "" {
			draft.RedisTrigger = &redisTrigger
			if draft.Triggers == nil {
				draft.Triggers = map[string]string{
					model.TriggerOpenAPI: "DISABLED", model.TriggerRedis: "ACTIVE", model.TriggerWebSocket: "DISABLED",
				}
			}
			draft.Triggers[model.TriggerRedis] = "ACTIVE"
		}
	}
	if len(raw.Config.Triggers) > 0 && draft.Triggers[model.TriggerRedis] == "ACTIVE" && draft.RedisTrigger == nil {
		return "", false, draft, fmt.Errorf("启用 Redis 接入时必须选择项目 Redis 连接并填写 Stream")
	}
	return reply, true, draft, nil
}

var assistantTriggerTypes = []string{model.TriggerOpenAPI, model.TriggerRedis, model.TriggerWebSocket}

func assistantDefaultTriggerStatus(triggerType string) string {
	if triggerType == model.TriggerOpenAPI {
		return "ACTIVE"
	}
	return "DISABLED"
}

// normalizeAssistantTriggers turns the model's access-method choice into the full
// desired state for all four trigger types. Because selecting any method disables
// the unlisted ones, a user who asks for Redis can never end up with the default
// Open API trigger silently left on.
func normalizeAssistantTriggers(input map[string]string, creating bool, current map[string]string) map[string]string {
	out := make(map[string]string, len(assistantTriggerTypes))
	for key, value := range input {
		key = strings.ToUpper(strings.TrimSpace(key))
		if !containsString(assistantTriggerTypes, key) {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(value))
		if status != "ACTIVE" && status != "DISABLED" {
			continue
		}
		out[key] = status
	}
	if len(out) == 0 {
		return nil
	}
	hasActive := false
	for _, status := range out {
		if status == "ACTIVE" {
			hasActive = true
			break
		}
	}
	for _, triggerType := range assistantTriggerTypes {
		if _, exists := out[triggerType]; exists {
			continue
		}
		switch {
		case creating || hasActive:
			out[triggerType] = "DISABLED"
		case current[triggerType] != "":
			out[triggerType] = current[triggerType]
		default:
			out[triggerType] = assistantDefaultTriggerStatus(triggerType)
		}
	}
	return out
}

func assistantTriggerSnapshot(triggers map[string]string) string {
	parts := make([]string, 0, len(assistantTriggerTypes))
	for _, triggerType := range assistantTriggerTypes {
		status := triggers[triggerType]
		if status == "" {
			status = "DISABLED"
		}
		parts = append(parts, triggerType+"="+status)
	}
	return strings.Join(parts, "；")
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

const assistantSystemPrompt = `你是「智审图档 SmartDoc Vision」的审核配置顾问。你的任务是通过简短问答，帮助管理员创建或修改一条图档审核配置，并同时确定任务接入方式。

【平台能力边界】
- 仅支持多文本 + 多图片审核，不支持视频/音频。
- decision_mode 只能是 REVIEW（合规审核）、CLASSIFY（分类并映射结论）或 CLASSIFY_ONLY（纯分类器，全部通过、只返回类型）。
- 分类审核需要提供至少两个分类；纯分类器可提供任意数量分类，分类结果始终 PASS，不需要映射驳回。
- 可绑定通用大模型（OpenAI 兼容 / Anthropic 协议）或 JEV 决策模型；分类场景优先建议 JEV。
- rules 是规则组数组；routes 是 PASS / REJECT / REVIEW 三路结果动作对象，通常保持空动作，除非用户明确要求。
- 接入方式（触发源）与审核配置一起保存，决定任务如何进入平台：OPENAPI（开放 API 提交）、REDIS（Redis Stream 消息）、WEBSOCKET（WebSocket）。可以同时开启多种，也可以只开启一种。平台不再支持 HTTP/MySQL 轮询。

【对话规则】
1. 用简体中文，每次最多问 2~3 个最关键的问题，不要一次罗列冗长清单；用户已经说清楚的信息不要重复追问。
2. 先确认：审核对象/业务场景、通过标准、驳回红线、是否需要分类、是否需要人工复核、任务通过什么方式接入（开放 API / Redis Stream / WebSocket）。
3. 如果用户回答“都行/你决定/直接生成/开始生成”，立即采用合理默认值并输出完整配置，不要继续追问。
4. 最多进行 3 轮提问；信息足够时直接生成。不要声称已经保存，保存由前端执行。
5. 用户明确给出的要求必须原样落进配置，禁止被平台默认值覆盖。例如用户说“用 Redis 接收消息”，必须输出 REDIS=ACTIVE、OPENAPI=DISABLED；除非用户同时要求保留开放 API。
6. reply 可以使用简洁的 Markdown（列表、加粗、行内代码、代码块），但整个回复仍必须是严格 JSON 对象，不要在 JSON 外面输出解释或前后缀。
7. 用户可能直接粘贴界面截图、表单图片或参考图；请结合图片内容理解需求，不要说无法查看图片。

【每轮输出 JSON】
未准备好时：
{"reply":"向用户提出的问题或说明","ready":false}
准备好时：
{"reply":"简短说明生成了什么配置、启用了哪种接入方式","ready":true,"config":{
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
  "routes":{"PASS":{"exec_mode":"sequential","on_error":"continue","actions":[]},"REJECT":{"exec_mode":"sequential","on_error":"continue","actions":[]},"REVIEW":{"exec_mode":"sequential","on_error":"to_review","actions":[]}},
  "triggers":{"OPENAPI":"DISABLED","REDIS":"ACTIVE","WEBSOCKET":"DISABLED"},
  "redis_trigger":{"connection_id":1,"db":0,"stream":"sdv:demo:audit:archive_review","consumer_group":"","start_position":"$","batch_size":16,"max_inflight":1}
}}

【字段约束】
- media_types 仅可包含 text、image。
- rules_preset 可选 standard / strict / loose；分类模式由服务端使用分类规则。rules 可留空数组，由平台补默认规则。
- 分类模式下 categories 必须给出至少两个分类；纯分类器可给任意数量类型，result 会被平台统一为 PASS。
- 纯分类器使用 decision_mode=CLASSIFY_ONLY，categories 中每项仍填写 code/name/description，result 写 PASS 即可。
- routes 的键必须是 PASS、REJECT、REVIEW；动作默认空数组。
- triggers 的键固定为 OPENAPI、REDIS、WEBSOCKET，值只能是 ACTIVE 或 DISABLED，表示三种接入方式的最终期望状态。
  - 用户提到接入方式时，必须输出完整的三个键；未提到的方式写 DISABLED，除非用户明确要求保留。
  - 用户说“用 Redis 接收消息/走 Redis 队列”时：REDIS=ACTIVE、OPENAPI=DISABLED。
  - 用户说“用开放 API / HTTP 接口提交”时：OPENAPI=ACTIVE，其他未要求的方式 DISABLED。
  - 用户说“用 WebSocket 推送”时：WEBSOCKET=ACTIVE，其他未要求的方式 DISABLED。
  - HTTP/MySQL 轮询已下线，不要建议或输出轮询配置。
  - REDIS=ACTIVE 时必须同时输出 redis_trigger，且 connection_id 只能从【可用 Redis 连接】中选择。
  - 如果【可用 Redis 连接】为空，禁止启用 REDIS；在 reply 中提示用户先到「数据连接」创建并测试 Redis 连接。
  - redis_trigger.stream 是要监听的业务队列名；consumer_group 可留空，平台会自动生成；start_position 只能是 "$"（只消费新消息）或 "0"（从头消费）。
  - 新建且用户完全未提及接入方式时，可省略 triggers，平台默认启用开放 API；编辑且用户未提及接入方式时，必须省略 triggers，保持现有触发源不变。
- 接入方式不能写进 review_prompt、rules 或 routes。
- 如果【当前配置快照】中有非空 treaties（已生效审核条约），review_prompt 必须包含占位符 {{AUDIT_TREATY}}，表示运行时注入条约内容；不要自行复述或编造条约条款。没有条约时不要输出该占位符。
- 编辑已有配置时，只修改用户要求的部分，保留未提及的字段和名称；code 由平台保持原值。
- 不要输出任何密钥、内网地址或虚构的业务事实。`
