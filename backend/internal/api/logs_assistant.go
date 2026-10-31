package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

type logsAssistantRequest struct {
	ProjectID  uint64             `json:"project_id"`
	ProviderID uint64             `json:"provider_id"`
	Messages   []assistantMessage `json:"messages"`
	Filters    *logsAssistantUI   `json:"filters"`
	Reasoning  string             `json:"reasoning"`
}

type logsAssistantUI struct {
	Operator   string   `json:"operator"`
	Actions    []string `json:"actions"`
	ObjectType string   `json:"object_type"`
	Result     string   `json:"result"`
	Keyword    string   `json:"keyword"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
}

type logsAssistantResult struct {
	Reply         string                `json:"reply"`
	Filter        logsAssistantUI       `json:"filter"`
	Total         int64                 `json:"total"`
	Tools         []string              `json:"tools"`
	Model         string                `json:"model"`
	Usage         assistantUsageSummary `json:"usage"`
	ContextTokens int                   `json:"context_tokens"`
	ContextWindow int                   `json:"context_window"`
	Compressed    bool                  `json:"compressed"`
	ToolLogs      []string              `json:"tool_logs,omitempty"`
}

type assistantUsageSummary struct {
	Requests     int `json:"requests"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type logsToolState struct {
	user       *model.User
	lastFilter auditLogFilter
	lastTotal  int64
	lastItems  []model.AuditLog
	toolLogs   []string
}

var auditActionLabels = map[string]string{
	"LOGIN":                       "登录",
	"LOGIN_FAILED":                "登录失败",
	"LOGIN_CAPTCHA_FAILED":        "滑块验证失败",
	"LOGIN_LOCKED":                "登录锁定",
	"LOGIN_MFA_CHALLENGE":         "两步验证挑战",
	"LOGIN_MFA_FAILED":            "两步验证失败",
	"LOGIN_MFA_ONLY":              "Authenticator 登录",
	"PASSWORD_CHANGE":             "修改密码",
	"MFA_ENABLE":                  "开启两步验证",
	"MFA_DISABLE":                 "关闭两步验证",
	"MFA_RESET":                   "重置两步验证",
	"PROJECT_CREATE":              "创建项目",
	"PROJECT_UPDATE":              "修改项目",
	"PROJECT_STATUS":              "项目状态变更",
	"PROJECT_DELETE":              "删除项目",
	"PROJECT_RESTORE":             "恢复项目",
	"PROJECT_PURGE":               "永久删除项目",
	"PROJECT_SECRET_UPDATE":       "更新项目密钥",
	"PROJECT_SECRET_DELETE":       "删除项目密钥",
	"PROVIDER_CREATE":             "创建模型接入",
	"PROVIDER_UPDATE":             "更新模型接入",
	"PROVIDER_DELETE":             "删除模型接入",
	"PROVIDER_TEST":               "测试模型连接",
	"AUDIT_CREATE":                "创建审核配置",
	"AUDIT_UPDATE":                "更新审核配置",
	"AUDIT_STATUS":                "审核启停",
	"AUDIT_DELETE":                "删除审核配置",
	"AUDIT_CLONE":                 "复制审核配置",
	"AUDIT_RESTORE":               "恢复审核配置",
	"AUDIT_PURGE":                 "永久删除审核配置",
	"TASK_SUBMIT":                 "提交审核任务",
	"TASK_COMPLETE":               "任务审核完成",
	"TASK_FAILED":                 "任务执行失败",
	"TASK_RETRY":                  "重试任务",
	"TASK_BATCH_RETRY":            "批量重试",
	"TASK_REVIEW":                 "人工复核",
	"TASK_DUPLICATE":              "重复任务去重",
	"REDIS_CONNECTION_CREATE":     "创建数据连接",
	"REDIS_CONNECTION_UPDATE":     "更新数据连接",
	"REDIS_CONNECTION_DELETE":     "删除数据连接",
	"REDIS_CONNECTION_TEST":       "测试数据连接",
	"CREDENTIAL_CREATE":           "创建接入凭证",
	"CREDENTIAL_UPDATE":           "更新接入凭证",
	"CREDENTIAL_ROTATE":           "轮换接入凭证",
	"CREDENTIAL_DELETE":           "删除接入凭证",
	"TRIGGER_UPDATE":              "更新触发方式",
	"TRIGGER_TEST":                "测试触发方式",
	"ROLE_CREATE":                 "创建角色",
	"ROLE_UPDATE":                 "更新角色",
	"ROLE_DELETE":                 "删除角色",
	"USER_CREATE":                 "创建账号",
	"USER_UPDATE":                 "更新账号",
	"SITE_BRANDING_UPDATE":        "更新站点品牌",
	"OPENAPI_SUBMIT":              "开放 API 提交",
	"NOTIFICATION_CHANNEL_CREATE": "创建通知渠道",
	"NOTIFICATION_CHANNEL_UPDATE": "更新通知渠道",
	"NOTIFICATION_CHANNEL_DELETE": "删除通知渠道",
	"NOTIFICATION_CHANNEL_TEST":   "测试通知渠道",
}

func auditActionLabel(action string) string {
	if label, ok := auditActionLabels[action]; ok {
		return label
	}
	return action
}

func auditActionCatalog() string {
	keys := make([]string, 0, len(auditActionLabels))
	for key := range auditActionLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+auditActionLabels[key])
	}
	return strings.Join(parts, ", ")
}

var logsAssistantSystemPrompt = `你是 SmartDoc Vision 的审计日志查询与分析助手。你可以调用只读工具查询审计日志，帮助管理员做模糊筛选、明细查询、统计分析和异常排查。

工作规则：
1. 先判断用户意图，再选择工具。不要凭空编造日志、数量或时间。
2. search_audit_logs 用于查明细，支持 keyword 模糊搜索、操作人 operator、动作 actions、对象 object_type、结果 result、日期 start/end 和 trace_id。
3. aggregate_audit_logs 用于统计分布或趋势，group_by 可选 action、operator、result、object_type、day、project。
4. get_audit_log_detail 用于查看单条日志的完整 detail；只有在用户明确要看详情或搜索返回了 trace_id/ID 时使用。
5. 当你已经确定用户最终要查看的日志范围时，在回答前调用一次 apply_log_filters，并传入最终应应用到页面表格的筛选条件；如果用户只是要统计概览、不需要筛选表格，可以不调用。
6. 如果用户没有说明时间范围，优先使用最近 30 天并在回答中注明；如果结果为空，明确说没有匹配日志并给出放宽条件的建议。
7. 回答使用中文，先给结论，再给关键数字、时间范围和有代表性的日志。不要展示大段原始 JSON。
8. 查询结果过多时提醒用户缩小范围；不要一次虚构或列出超过工具返回的数据。
9. 你只能读取和汇总日志，不能修改、删除、重试或执行任何业务操作。

可用动作代码：` + auditActionCatalog() + `

当前时间：`

func logsAssistantTools() []ai.ToolSpec {
	common := map[string]any{
		"operator":    map[string]any{"type": "string", "description": "操作人，模糊匹配"},
		"actions":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "动作代码数组，例如 LOGIN、TASK_SUBMIT"},
		"object_type": map[string]any{"type": "string", "description": "对象类型，例如 USER、AUDIT、TASK"},
		"result":      map[string]any{"type": "string", "enum": []string{"SUCCESS", "FAILED"}, "description": "执行结果"},
		"keyword":     map[string]any{"type": "string", "description": "模糊搜索操作人、动作、对象、trace_id 与详情"},
		"trace_id":    map[string]any{"type": "string", "description": "精确 trace_id"},
		"start":       map[string]any{"type": "string", "description": "开始日期，格式 YYYY-MM-DD"},
		"end":         map[string]any{"type": "string", "description": "结束日期，格式 YYYY-MM-DD"},
	}
	searchProps := map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "返回条数，默认 20"}}
	for key, value := range common {
		searchProps[key] = value
	}
	aggregateProps := map[string]any{
		"group_by": map[string]any{"type": "string", "enum": []string{"action", "operator", "result", "object_type", "day", "project"}, "description": "分组维度，默认 action"},
		"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "返回分组数，默认 20"},
	}
	for key, value := range common {
		aggregateProps[key] = value
	}
	return []ai.ToolSpec{
		{
			Name:        "search_audit_logs",
			Description: "按条件查询审计日志明细，返回总数和最近匹配记录（最多 50 条）。",
			Parameters:  map[string]any{"type": "object", "properties": searchProps},
		},
		{
			Name:        "aggregate_audit_logs",
			Description: "按动作、操作人、结果、对象类型、日期或项目统计审计日志数量。",
			Parameters:  map[string]any{"type": "object", "properties": aggregateProps},
		},
		{
			Name:        "get_audit_log_detail",
			Description: "查看一条或多条审计日志的完整详情；可按 ID 或 trace_id 查询。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":       map[string]any{"type": "integer", "description": "日志 ID"},
					"trace_id": map[string]any{"type": "string", "description": "trace_id"},
				},
			},
		},
		{
			Name:        "apply_log_filters",
			Description: "把最终筛选条件交回页面表格，供用户一键应用；不修改数据。只在用户需要查看/筛选明细时调用一次。",
			Parameters:  map[string]any{"type": "object", "properties": common},
		},
	}
}

func (s *Server) logsAssistant(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "log:read") {
		forbidden(c, "当前角色无权查询审计日志")
		return
	}
	var req logsAssistantRequest
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
		badRequest(c, "请输入查询需求")
		return
	}
	if len(req.Messages) > 100 {
		req.Messages = req.Messages[len(req.Messages)-100:]
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
		badRequest(c, "请输入查询需求")
		return
	}

	system := logsAssistantSystemPrompt + time.Now().Format("2006-01-02 15:04:05") +
		"\n当前用户：" + user.Username + "（" + user.Role + "）"
	if req.Filters != nil {
		if raw, err := json.Marshal(req.Filters); err == nil {
			system += "\n当前页面筛选：" + string(raw)
		}
	}
	system += "\n日志范围：当前账号有权查看的全部项目；登录与账号操作属于全局日志。"

	providerConfig := model.ProviderConfig{}
	_ = model.DecodeJSON(provider.ConfigJSON, &providerConfig)
	contextWindow := providerConfig.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 128000
	}

	state := &logsToolState{user: user}
	tools := logsAssistantTools()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	var final *ai.ChatResult
	usage := assistantUsageSummary{}
	modelName := provider.Model
	compressed := false
	contextTokens := 0
	if estimateMessagesTokens(messages) > contextWindow*3/4 {
		if summary, compressUsage, err := s.compressLogsContext(ctx, provider, key, messages); err == nil {
			messages = append([]ai.ChatMessage{{Role: "assistant", Content: "【历史对话摘要】\n" + summary}}, lastMessages(messages, 4)...)
			compressed = true
			usage.Requests += compressUsage.Requests
			usage.InputTokens += compressUsage.InputTokens
			usage.OutputTokens += compressUsage.OutputTokens
			usage.TotalTokens += compressUsage.TotalTokens
		} else {
			messages = lastMessages(messages, 8)
			compressed = true
		}
	}
	for turn := 0; turn < 6; turn++ {
		started := time.Now()
		result, chatErr := ai.Chat(ctx, ai.ChatRequest{
			Provider: *provider, APIKey: key, System: system,
			Messages: messages, Tools: tools, MaxTokens: 4096, Temperature: 0.2,
			Reasoning: req.Reasoning,
		})
		if chatErr != nil {
			s.recordProviderUsage(*provider, model.UsageSourceLogsAssistant, 0, nil, time.Since(started), chatErr)
			if isContextLimitError(chatErr) && !compressed {
				if summary, compressUsage, err := s.compressLogsContext(ctx, provider, key, messages); err == nil {
					messages = append([]ai.ChatMessage{{Role: "assistant", Content: "【历史对话摘要】\n" + summary}}, lastMessages(messages, 4)...)
					compressed = true
					usage.Requests += compressUsage.Requests
					usage.InputTokens += compressUsage.InputTokens
					usage.OutputTokens += compressUsage.OutputTokens
					usage.TotalTokens += compressUsage.TotalTokens
					continue
				}
			}
			if isToolUnsupportedError(chatErr) {
				fail(c, http.StatusUnprocessableEntity, "ASSISTANT_TOOLS_UNSUPPORTED",
					"当前模型不支持工具调用，请到「模型接入」启用支持 function calling 的通用大模型", nil)
				return
			}
			serverError(c, chatErr)
			return
		}
		modelName = result.Model
		contextTokens = result.Usage.InputTokens + result.Usage.OutputTokens
		s.recordProviderUsage(*provider, model.UsageSourceLogsAssistant, 0, result, time.Since(started), nil)
		usage.Requests++
		usage.InputTokens += result.Usage.InputTokens
		usage.OutputTokens += result.Usage.OutputTokens
		usage.TotalTokens += result.Usage.TotalTokens
		if len(result.ToolCalls) == 0 {
			final = result
			break
		}
		messages = append(messages, ai.ChatMessage{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
		for index := range result.ToolCalls {
			call := result.ToolCalls[index]
			id := call.ID
			if id == "" {
				id = fmt.Sprintf("log-tool-%d-%d", turn, index)
			}
			output := s.executeLogsTool(ctx, state, call)
			messages = append(messages, ai.ChatMessage{Role: "tool", ToolCallID: id, Content: output})
		}
	}
	if final == nil {
		final = &ai.ChatResult{Content: "查询轮次已达上限。请缩小时间范围或指定操作类型后再试。", Model: modelName}
	}
	reply := strings.TrimSpace(final.Content)
	if reply == "" {
		reply = "已完成日志查询。"
	}
	filter := logsAssistantUI{
		Operator:   state.lastFilter.Operator,
		Actions:    state.lastFilter.Actions,
		ObjectType: state.lastFilter.ObjectType,
		Result:     state.lastFilter.Result,
		Keyword:    state.lastFilter.Keyword,
		Start:      state.lastFilter.Start,
		End:        state.lastFilter.End,
	}
	if contextTokens == 0 {
		contextTokens = estimateMessagesTokens(messages)
	}
	ok(c, logsAssistantResult{
		Reply: reply, Filter: filter, Total: state.lastTotal,
		Tools: state.toolLogs, Model: modelName, Usage: usage,
		ContextTokens: contextTokens, ContextWindow: contextWindow, Compressed: compressed,
	})
}

func estimateMessagesTokens(messages []ai.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content)) + 4
	}
	return total
}

func lastMessages(messages []ai.ChatMessage, count int) []ai.ChatMessage {
	if count <= 0 || len(messages) <= count {
		return messages
	}
	return messages[len(messages)-count:]
}

func (s *Server) compressLogsContext(ctx context.Context, provider *model.AIProvider, key string, messages []ai.ChatMessage) (string, assistantUsageSummary, error) {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Role+": "+truncateText(message.Content, 4000))
	}
	prompt := "请把以下审计日志助手的对话压缩成简洁摘要，保留用户目标、已确认结论、筛选条件、尚未解决的问题和重要数据；不要编造内容。" + "\n\n" + strings.Join(parts, "\n")
	started := time.Now()
	result, err := ai.Chat(ctx, ai.ChatRequest{
		Provider: *provider, APIKey: key,
		System:    "你是对话上下文压缩助手。只输出摘要正文。",
		Messages:  []ai.ChatMessage{{Role: "user", Content: truncateText(prompt, 24000)}},
		MaxTokens: 1000, Temperature: 0.2,
	})
	usage := assistantUsageSummary{}
	if err != nil {
		s.recordProviderUsage(*provider, model.UsageSourceLogsAssistant, 0, nil, time.Since(started), err)
		return "", usage, err
	}
	s.recordProviderUsage(*provider, model.UsageSourceLogsAssistant, 0, result, time.Since(started), nil)
	usage = assistantUsageSummary{
		Requests: 1, InputTokens: result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens, TotalTokens: result.Usage.TotalTokens,
	}
	return strings.TrimSpace(result.Content), usage, nil
}

func isContextLimitError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "context") && !strings.Contains(lower, "token") && !strings.Contains(lower, "上下文") {
		return false
	}
	return strings.Contains(lower, "length") || strings.Contains(lower, "too long") ||
		strings.Contains(lower, "too many") || strings.Contains(lower, "maximum") ||
		strings.Contains(lower, "exceed") || strings.Contains(lower, "limit")
}

func (s *Server) executeLogsTool(ctx context.Context, state *logsToolState, call ai.ToolCall) string {
	switch call.Name {
	case "search_audit_logs":
		return s.executeLogsSearch(ctx, state, call.Arguments)
	case "aggregate_audit_logs":
		return s.executeLogsAggregate(ctx, state, call.Arguments)
	case "get_audit_log_detail":
		return s.executeLogsDetail(ctx, state, call.Arguments)
	case "apply_log_filters":
		return s.executeLogsApplyFilters(ctx, state, call.Arguments)
	default:
		return "工具错误：未知工具 " + call.Name
	}
}

func parseLogsFilter(args map[string]any) auditLogFilter {
	filter := auditLogFilter{}
	filter.Operator = stringArg(args["operator"])
	filter.ObjectType = stringArg(args["object_type"])
	filter.Result = stringArg(args["result"])
	filter.Keyword = stringArg(args["keyword"])
	filter.TraceID = stringArg(args["trace_id"])
	filter.Start = stringArg(args["start"])
	filter.End = stringArg(args["end"])
	if raw, ok := args["actions"].([]any); ok {
		for _, item := range raw {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" {
				filter.Actions = append(filter.Actions, value)
			}
		}
	}
	return filter
}

func (s *Server) executeLogsSearch(ctx context.Context, state *logsToolState, raw string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "工具错误：参数不是合法 JSON: " + err.Error()
	}
	filter := parseLogsFilter(args)
	limit := intArg(args["limit"], 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	if _, err := parseAuditDate(filter.Start); err != nil {
		return "工具错误：start 日期格式应为 YYYY-MM-DD"
	}
	if _, err := parseAuditDate(filter.End); err != nil {
		return "工具错误：end 日期格式应为 YYYY-MM-DD"
	}
	q := s.auditLogQuery(state.user, filter)
	var total int64
	if err := q.WithContext(ctx).Count(&total).Error; err != nil {
		return "工具错误：查询失败: " + err.Error()
	}
	var items []model.AuditLog
	if err := q.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&items).Error; err != nil {
		return "工具错误：读取失败: " + err.Error()
	}
	state.lastTotal = total
	state.lastItems = items
	state.toolLogs = append(state.toolLogs, "search_audit_logs")
	return logsToolJSON(map[string]any{
		"total": total, "returned": len(items), "filters": logsFilterView(filter),
		"items":     logsToolItems(items),
		"truncated": total > int64(len(items)),
	})
}

func (s *Server) executeLogsAggregate(ctx context.Context, state *logsToolState, raw string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "工具错误：参数不是合法 JSON: " + err.Error()
	}
	filter := parseLogsFilter(args)
	if _, err := parseAuditDate(filter.Start); err != nil {
		return "工具错误：start 日期格式应为 YYYY-MM-DD"
	}
	if _, err := parseAuditDate(filter.End); err != nil {
		return "工具错误：end 日期格式应为 YYYY-MM-DD"
	}
	groupBy := strings.ToLower(stringArg(args["group_by"]))
	if groupBy == "" {
		groupBy = "action"
	}
	limit := intArg(args["limit"], 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	groupExpr := ""
	orderExpr := "group_count desc"
	switch groupBy {
	case "action":
		groupExpr = "action"
	case "operator":
		groupExpr = "operator"
	case "result":
		groupExpr = "result"
	case "object_type":
		groupExpr = "object_type"
	case "project":
		groupExpr = "COALESCE(CAST(project_id AS CHAR), 'GLOBAL')"
	case "day":
		groupExpr = "DATE_FORMAT(created_at, '%Y-%m-%d')"
		orderExpr = "group_key asc"
	default:
		return "工具错误：group_by 只能是 action、operator、result、object_type、day、project"
	}
	q := s.auditLogQuery(state.user, filter)
	var total int64
	if err := q.WithContext(ctx).Count(&total).Error; err != nil {
		return "工具错误：统计失败: " + err.Error()
	}
	type logsGroupRow struct {
		GroupKey   string `gorm:"column:group_key"`
		GroupCount int64  `gorm:"column:group_count"`
	}
	rows := make([]logsGroupRow, 0)
	groupQuery := s.auditLogQuery(state.user, filter)
	if err := groupQuery.WithContext(ctx).
		Select(groupExpr + " AS group_key, COUNT(*) AS group_count").
		Group(groupExpr).Order(orderExpr).Limit(limit).Scan(&rows).Error; err != nil {
		return "工具错误：分组失败: " + err.Error()
	}
	state.lastTotal = total
	state.toolLogs = append(state.toolLogs, "aggregate_audit_logs")
	converted := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		key := row.GroupKey
		if groupBy == "action" {
			key = auditActionLabel(key)
		}
		converted = append(converted, map[string]any{"key": key, "count": row.GroupCount})
	}
	return logsToolJSON(map[string]any{
		"total": total, "group_by": groupBy, "groups": converted,
		"filters": logsFilterView(filter), "truncated": len(rows) >= limit,
	})
}

func (s *Server) executeLogsApplyFilters(ctx context.Context, state *logsToolState, raw string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "工具错误：参数不是合法 JSON: " + err.Error()
	}
	filter := parseLogsFilter(args)
	if _, err := parseAuditDate(filter.Start); err != nil {
		return "工具错误：start 日期格式应为 YYYY-MM-DD"
	}
	if _, err := parseAuditDate(filter.End); err != nil {
		return "工具错误：end 日期格式应为 YYYY-MM-DD"
	}
	state.lastFilter = filter
	state.toolLogs = append(state.toolLogs, "apply_log_filters")
	return "筛选条件已准备好，用户可以在前端应用到日志表格。"
}

func (s *Server) executeLogsDetail(ctx context.Context, state *logsToolState, raw string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return "工具错误：参数不是合法 JSON: " + err.Error()
	}
	filter := auditLogFilter{}
	id := uint64(intArg(args["id"], 0))
	traceID := stringArg(args["trace_id"])
	if id == 0 && traceID == "" {
		return "工具错误：请提供 id 或 trace_id"
	}
	q := s.auditLogQuery(state.user, filter)
	if id != 0 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("trace_id = ?", traceID)
	}
	var items []model.AuditLog
	if err := q.WithContext(ctx).Order("created_at desc").Limit(20).Find(&items).Error; err != nil {
		return "工具错误：读取失败: " + err.Error()
	}
	if len(items) == 0 {
		return "没有找到匹配的日志。"
	}
	state.toolLogs = append(state.toolLogs, "get_audit_log_detail")
	return logsToolJSON(map[string]any{"returned": len(items), "items": logsToolItems(items)})
}

func logsToolItems(items []model.AuditLog) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id": item.ID, "time": item.CreatedAt.Format(time.RFC3339),
			"project_id": item.ProjectID, "operator": item.Operator, "ip": item.OperatorIP,
			"action": item.Action, "action_label": auditActionLabel(item.Action),
			"object_type": item.ObjectType, "object_id": item.ObjectID,
			"result": item.Result, "trace_id": item.TraceID,
			"detail": truncateText(string(item.DetailJSON), 1200),
		})
	}
	return out
}

func logsFilterView(filter auditLogFilter) map[string]any {
	return map[string]any{
		"operator": filter.Operator, "actions": filter.Actions,
		"object_type": filter.ObjectType, "result": filter.Result,
		"keyword": filter.Keyword, "trace_id": filter.TraceID,
		"start": filter.Start, "end": filter.End,
	}
}

func logsToolJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "工具错误：序列化结果失败"
	}
	return string(raw)
}

func stringArg(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intArg(value any, fallback int) int {
	switch item := value.(type) {
	case float64:
		return int(item)
	case int:
		return item
	case json.Number:
		number, err := item.Int64()
		if err == nil {
			return int(number)
		}
	}
	return fallback
}
