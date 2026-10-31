package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

const (
	assistantFieldRules  = "rules"
	assistantFieldPrompt = "review_prompt"

	maxFieldAssistantRunes  = 12000
	maxGeneratedPromptRunes = 12000
)

type auditFieldAssistantRequest struct {
	ProjectID   uint64         `json:"project_id"`
	ProviderID  uint64         `json:"provider_id"`
	Field       string         `json:"field"`
	Instruction string         `json:"instruction"`
	Current     string         `json:"current"`
	Context     map[string]any `json:"context"`
}

// auditFieldAssistant generates one audit field (rules or review prompt) from
// a natural-language instruction. It never saves the audit; the caller applies
// the returned text to the form through the normal audited update API.
func (s *Server) auditFieldAssistant(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权使用 AI 生成")
		return
	}
	var req auditFieldAssistantRequest
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
	field := strings.ToLower(strings.TrimSpace(req.Field))
	if field != assistantFieldRules && field != assistantFieldPrompt {
		badRequest(c, "仅支持生成规则或审核要求")
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		badRequest(c, "请描述希望 AI 如何调整")
		return
	}
	if len([]rune(instruction)) > 4000 {
		badRequest(c, "需求描述不能超过 4000 个字符")
		return
	}
	current := strings.TrimSpace(req.Current)
	if len([]rune(current)) > maxFieldAssistantRunes {
		badRequest(c, "当前内容过长，请先精简后再让 AI 处理")
		return
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

	systemPrompt := rulesFieldAssistantSystemPrompt
	maxTokens := 3200
	if field == assistantFieldPrompt {
		systemPrompt = promptFieldAssistantSystemPrompt
		maxTokens = 2200
	}
	systemPrompt += "\n\n【当前审核配置】\n" + assistantFieldContext(req.Context) + "\n"

	userContent := "【用户要求】\n" + instruction
	switch field {
	case assistantFieldRules:
		if current != "" {
			userContent += "\n\n【当前规则 JSON】\n```json\n" + current + "\n```"
		} else {
			userContent += "\n\n【当前规则 JSON】\n（空）"
		}
	case assistantFieldPrompt:
		if current != "" {
			userContent += "\n\n【当前审核要求】\n" + current
		} else {
			userContent += "\n\n【当前审核要求】\n（空）"
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 600*time.Second)
	defer cancel()
	started := time.Now()
	result, err := ai.Chat(ctx, ai.ChatRequest{
		Provider: *provider, APIKey: key, System: systemPrompt,
		Messages:    []ai.ChatMessage{{Role: "user", Content: userContent}},
		MaxTokens:   maxTokens,
		Temperature: 0.2,
	})
	if err != nil {
		s.recordProviderUsage(*provider, model.UsageSourceAuditAssistant, 0, nil, time.Since(started), err)
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_FAILED", err.Error(), nil)
		return
	}
	s.recordProviderUsage(*provider, model.UsageSourceAuditAssistant, 0, result, time.Since(started), nil)

	var explanation, text string
	decisionMode := assistantFieldDecisionMode(req.Context)
	if field == assistantFieldRules {
		explanation, text, err = parseGeneratedRules(result.Content, decisionMode)
	} else {
		explanation, text, err = parseGeneratedPrompt(result.Content)
	}
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "ASSISTANT_FIELD_INVALID", err.Error(), gin.H{
			"raw": truncateText(result.Content, 3000),
		})
		return
	}
	ok(c, gin.H{
		"field": field, "text": text, "explanation": explanation,
		"provider_id": provider.ID, "provider_name": provider.Name, "model": result.Model,
		"usage": assistantUsageView(provider, result),
	})
}

func assistantFieldContext(raw map[string]any) string {
	if len(raw) == 0 {
		return "（未提供）"
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "（未提供）"
	}
	return truncateText(string(encoded), 6000)
}

func assistantFieldDecisionMode(raw map[string]any) string {
	value, _ := raw["decision_mode"].(string)
	return strings.ToUpper(strings.TrimSpace(value))
}

func parseGeneratedRules(content, decisionMode string) (string, string, error) {
	reply, raw, err := extractRulesPayload(content)
	if err != nil {
		return "", "", err
	}
	var groups []any
	if err := json.Unmarshal(raw, &groups); err != nil {
		return "", "", fmt.Errorf("模型返回的规则不是 JSON 数组")
	}
	if len(groups) == 0 {
		if decisionMode == model.DecisionClassifyOnly {
			return "纯分类器成功审核固定 PASS，因此没有生成会改变结论的规则。", "[]", nil
		}
		return "", "", fmt.Errorf("AI 没有生成有效规则，请补充需求后重试")
	}
	if err := validateRulesJSON(raw); err != nil {
		return "", "", fmt.Errorf("AI 生成的规则不符合平台校验：%w", err)
	}
	if err := validateGeneratedRules(raw); err != nil {
		return "", "", err
	}
	normalized, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("规则 JSON 格式化失败")
	}
	if strings.TrimSpace(reply) == "" {
		reply = fmt.Sprintf("已生成 %d 组规则，请检查阈值与优先级后应用。", len(groups))
	}
	return reply, string(normalized), nil
}

func extractRulesPayload(content string) (string, json.RawMessage, error) {
	text := strings.TrimSpace(content)
	if jsonText := extractJSONObject(text); jsonText != "" {
		var parsed struct {
			Reply string          `json:"reply"`
			Rules json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal([]byte(jsonText), &parsed); err == nil && len(parsed.Rules) > 0 {
			return strings.TrimSpace(parsed.Reply), parsed.Rules, nil
		}
	}
	if jsonText := extractJSONArray(text); jsonText != "" {
		return "", json.RawMessage(jsonText), nil
	}
	return "", nil, fmt.Errorf("模型没有返回规则 JSON，请重试")
}

func extractJSONArray(value string) string {
	start := strings.Index(value, "[")
	end := strings.LastIndex(value, "]")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(value[start : end+1])
}

func parseGeneratedPrompt(content string) (string, string, error) {
	text := strings.TrimSpace(content)
	reply := ""
	prompt := ""
	parsedJSON := false
	jsonText := extractJSONObject(text)
	if jsonText != "" {
		var parsed struct {
			Reply  string `json:"reply"`
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(jsonText), &parsed); err == nil {
			parsedJSON = true
			reply = strings.TrimSpace(parsed.Reply)
			prompt = strings.TrimSpace(parsed.Prompt)
		}
	}
	if prompt == "" && !parsedJSON {
		prompt = stripMarkdownFence(text)
	}
	if prompt == "" {
		return "", "", fmt.Errorf("模型没有返回审核要求")
	}
	length := len([]rune(prompt))
	if length < 20 {
		return "", "", fmt.Errorf("AI 生成的审核要求过短，请补充审核目标与判定边界后重试")
	}
	if length > maxGeneratedPromptRunes {
		return "", "", fmt.Errorf("AI 生成的审核要求超过 %d 个字符，请精简需求后重试", maxGeneratedPromptRunes)
	}
	if strings.TrimSpace(reply) == "" {
		reply = "已优化审核要求，请检查后应用到表单。"
	}
	return reply, prompt, nil
}

func stripMarkdownFence(value string) string {
	text := strings.TrimSpace(value)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return text
	}
	lines = lines[1:]
	if last := strings.TrimSpace(lines[len(lines)-1]); last == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func validateGeneratedRules(raw []byte) error {
	var groups []struct {
		Name        string `json:"name"`
		Logic       string `json:"logic"`
		OnHitResult string `json:"on_hit_result"`
		Items       []struct {
			Field    string `json:"field"`
			Operator string `json:"operator"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return fmt.Errorf("规则必须是 JSON 数组")
	}
	if len(groups) > 30 {
		return fmt.Errorf("规则组不能超过 30 个")
	}
	for index, group := range groups {
		label := strings.TrimSpace(group.Name)
		if label == "" {
			label = fmt.Sprintf("第 %d 组", index+1)
		}
		if !validResult(group.OnHitResult) {
			return fmt.Errorf("规则「%s」的 on_hit_result 必须为 PASS/REJECT/REVIEW", label)
		}
		logic := strings.ToUpper(strings.TrimSpace(group.Logic))
		if logic == "" {
			logic = "AND"
		}
		if logic != "AND" && logic != "OR" {
			return fmt.Errorf("规则「%s」的 logic 仅支持 AND/OR", label)
		}
		if len(group.Items) == 0 {
			return fmt.Errorf("规则「%s」缺少 items 条件", label)
		}
		if len(group.Items) > 20 {
			return fmt.Errorf("规则「%s」的条件不能超过 20 个", label)
		}
		for itemIndex, item := range group.Items {
			field := strings.TrimSpace(item.Field)
			if !allowedAssistantRuleField(field) {
				return fmt.Errorf("规则「%s」第 %d 个条件使用了不支持的字段 %q", label, itemIndex+1, field)
			}
			operator := strings.ToLower(strings.TrimSpace(item.Operator))
			if !allowedAssistantRuleOperator(operator) {
				return fmt.Errorf("规则「%s」第 %d 个条件使用了不支持的运算符 %q", label, itemIndex+1, operator)
			}
		}
	}
	return nil
}

var assistantRuleFields = map[string]bool{
	"model.result": true, "model.score": true, "model.risk_level": true,
	"model.label": true, "model.labels": true, "model.summary": true,
	"model.category": true, "model.category_name": true, "model.category_confidence": true,
	"media.text": true, "media.text_len": true, "media.count": true,
	"media.image_count": true, "media.type": true,
	"task.source_type": true, "task.request_id": true, "task.biz_key": true, "task.source_ref": true,
}

var assistantRuleOperators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true,
	"between": true, "in": true, "not_in": true,
	"contains": true, "not_contains": true, "contains_any": true, "contains_all": true,
	"starts_with": true, "ends_with": true, "regex": true,
	"is_null": true, "is_not_null": true, "is_empty": true, "is_not_empty": true,
}

func allowedAssistantRuleField(field string) bool {
	if assistantRuleFields[field] {
		return true
	}
	return strings.HasPrefix(field, "ext.") || strings.HasPrefix(field, "input.")
}

func allowedAssistantRuleOperator(operator string) bool {
	return assistantRuleOperators[operator]
}

const rulesFieldAssistantSystemPrompt = `你是「智审图档 SmartDoc Vision」的审核规则助手。请根据用户要求与当前审核配置，生成可直接保存执行的规则组 JSON。

【规则结构】
规则组是一个 JSON 数组，每项结构：
{
  "name": "规则名称",
  "priority": 10,
  "logic": "AND 或 OR",
  "stop_on_hit": true,
  "on_hit_result": "PASS 或 REJECT 或 REVIEW",
  "items": [
    {"field": "model.risk_level", "operator": "eq", "value": "high"}
  ]
}

【可用字段】
model.result, model.score, model.risk_level, model.label, model.labels, model.summary,
model.category, model.category_name, model.category_confidence,
media.text, media.text_len, media.count, media.image_count, media.type,
task.source_type, task.request_id, task.biz_key, task.source_ref, ext.*, input.*

【可用运算符】
eq, ne, gt, gte, lt, lte, between, in, not_in,
contains, not_contains, contains_any, contains_all,
starts_with, ends_with, regex,
is_null, is_not_null, is_empty, is_not_empty

【要求】
1. priority 数字小的先执行；通常从 10 开始递增，并设置 stop_on_hit=true。
2. on_hit_result 只能是 PASS / REJECT / REVIEW。
3. 合规审核通常需要：高风险驳回、低风险通过、灰区转人工。
4. 分类审核请使用 model.category、model.category_confidence 和 model.result；分类编码必须来自当前配置的分类列表，低置信度转人工。
5. 纯分类器成功审核固定 PASS，规则不能把结果改成驳回；如果用户的规则不影响分类结论，可以返回空数组 []。
6. 不要生成 routes、审核提示词、接入方式或平台字段说明。
7. 用户提出的阈值、字段和结论必须原样体现；不要编造不存在的字段或运算符。
8. 只输出严格 JSON，不要 Markdown 代码块、注释或 JSON 以外的内容：
{"reply":"简短说明生成了什么规则","rules":[...]}`

const promptFieldAssistantSystemPrompt = `你是「智审图档 SmartDoc Vision」的审核提示词助手。请根据用户要求与当前审核配置，优化发送给模型的审核要求 Prompt。

【输出要求】
只输出严格 JSON，不要 Markdown 代码块或 JSON 以外的内容：
{"reply":"简短说明改了什么","prompt":"完整的审核要求"}

【Prompt 内容要求】
1. 使用简体中文，直接写给审核模型，明确审核对象、审核重点、通过标准、驳回红线和转人工边界。
2. 保留用户原有约束与当前配置中仍然有效的业务要求；用户要求优先。
3. 适合多文本 + 多图片审核，不描述视频/音频能力。
4. 不要写入接入方式、规则 JSON、任务号、API 地址、密钥或平台操作说明。
5. 不要使用 Markdown 标题、代码块或列表符号；使用自然段落，长度控制在 80~8000 字。
6. 无法确认或信息不足时，要求模型返回 REVIEW，不要臆测。
7. 分类审核应要求模型只使用配置中的分类编码，并返回分类置信度；纯分类器只要求返回分类类型。
8. 如果当前审核配置存在已生效的审核条约（上下文 treaty_count > 0），审核要求中必须包含占位符 {{AUDIT_TREATY}}，表示运行时注入条约内容；不要自行复述或编造条约条款。没有条约时不要输出该占位符。
9. 不要输出与 prompt 无关的解释；reply 只做一句话说明。`
