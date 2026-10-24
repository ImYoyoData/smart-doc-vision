package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Calcium-Ion/moejs"
	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

type scriptAssistantRequest struct {
	ProjectID     uint64             `json:"project_id"`
	ProviderID    uint64             `json:"provider_id"`
	AuditID       uint64             `json:"audit_id"`
	CurrentScript string             `json:"current_script"`
	Messages      []assistantMessage `json:"messages"`
}

func (s *Server) scriptAssistantStream(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权使用脚本助手")
		return
	}
	var req scriptAssistantRequest
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
	if len(req.CurrentScript) > maxScriptBytes {
		badRequest(c, "当前脚本超过 64KB 限制")
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

	var audit *model.Audit
	if req.AuditID > 0 {
		var current model.Audit
		if err := s.db.Where("id = ? AND project_id = ? AND deleted_at IS NULL", req.AuditID, req.ProjectID).First(&current).Error; err != nil {
			notFound(c, "审核配置不存在")
			return
		}
		audit = &current
	}

	systemPrompt := scriptAssistantSystemPrompt
	systemPrompt += "\n\n【当前项目】\n项目编码：" + s.projectCode(req.ProjectID) + "\n"
	if audit != nil {
		var categories []model.AuditCategory
		_ = model.DecodeJSON(audit.Categories, &categories)
		var mediaTypes []string
		_ = model.DecodeJSON(audit.MediaTypes, &mediaTypes)
		summary, _ := json.Marshal(map[string]any{
			"code": audit.Code, "name": audit.Name, "decision_mode": audit.DecisionMode,
			"media_types": mediaTypes, "categories": categories,
		})
		systemPrompt += "【当前审核配置】\n" + truncateText(string(summary), 3000) + "\n"
	}
	if strings.TrimSpace(req.CurrentScript) != "" {
		systemPrompt += "\n【当前脚本】\n```js\n" + truncateText(req.CurrentScript, 12000) + "\n```\n"
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
		badRequest(c, "请输入需求描述")
		return
	}

	steps := make([]assistantStep, 0, 6)
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
		writeAssistantSSE(c, "step", step)
	}
	prepareAssistantSSE(c)
	emit(assistantStep{ID: "context", Label: "读取当前脚本", Status: "done",
		Detail: map[bool]string{true: "已读取审核配置", false: "未绑定审核配置"}[audit != nil]})
	emit(assistantStep{ID: "provider", Label: "选择通用大模型", Status: "done", Detail: provider.Name + " · " + provider.Model})
	emit(assistantStep{ID: "thinking", Label: "AI 正在生成脚本", Status: "running"})

	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	started := time.Now()
	result, chatErr := ai.Chat(ctx, ai.ChatRequest{
		Provider: *provider, APIKey: key, System: systemPrompt,
		Messages: messages, MaxTokens: 3200, Temperature: 0.2,
	})
	if chatErr != nil {
		emit(assistantStep{ID: "thinking", Label: "AI 正在生成脚本", Status: "error", Detail: chatErr.Error()})
		writeAssistantSSE(c, "error", gin.H{"code": "ASSISTANT_FAILED", "message": chatErr.Error()})
		return
	}
	emit(assistantStep{ID: "thinking", Label: "AI 正在生成脚本", Status: "done",
		Detail: fmt.Sprintf("%s · %.1fs", result.Model, time.Since(started).Seconds())})
	emit(assistantStep{ID: "check", Label: "检查脚本语法", Status: "running"})

	reply, script, scriptErr := parseScriptAssistantReply(result.Content)
	scriptValid := false
	scriptError := ""
	if script != "" {
		if len(script) > maxScriptBytes {
			scriptError = "脚本超过 64KB 限制"
		} else if _, err := moejs.CompileScript("script.js", script); err != nil {
			scriptError = err.Error()
		} else {
			scriptValid = true
		}
	}
	if script == "" {
		scriptError = ""
	}
	emit(assistantStep{ID: "check", Label: "检查脚本语法", Status: "done",
		Detail: map[bool]string{true: "语法通过", false: "未返回脚本"}[script == "" || scriptValid]})

	response := gin.H{
		"reply": reply, "script": script,
		"script_valid": scriptValid, "script_error": scriptError,
		"model": result.Model, "provider_id": provider.ID, "provider_name": provider.Name,
		"steps": steps,
	}
	if scriptErr != nil {
		response["parse_error"] = scriptErr.Error()
	}
	writeAssistantSSE(c, "reply", response)
	writeAssistantSSE(c, "done", gin.H{})
}

func parseScriptAssistantReply(content string) (string, string, error) {
	text := strings.TrimSpace(content)
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return text, "", nil
	}
	var parsed struct {
		Reply  string `json:"reply"`
		Script string `json:"script"`
	}
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return text, "", nil
	}
	reply := strings.TrimSpace(parsed.Reply)
	if reply == "" {
		reply = "已生成脚本，请检查后应用到编辑器。"
	}
	return reply, strings.TrimSpace(parsed.Script), nil
}

const scriptAssistantSystemPrompt = `你是「智审图档 SmartDoc Vision」的 JS 结果动作助手。你的任务是帮助管理员生成、修改或解释一个在审核结束后执行的 JavaScript 脚本。

【输出格式】
每一轮只输出一个严格 JSON 对象，不要输出 Markdown 代码块或 JSON 以外的内容：
{"reply":"给用户的 Markdown 说明","script":"完整脚本；只是解释或提问时可为空字符串"}

【脚本入口】
脚本必须是经典脚本，并定义：
function main(ctx) { ... }
脚本同步执行，不支持 setTimeout、setInterval、事件循环、异步宿主 API、import、require、eval、Function。

【可用 API】
ctx.task：id, task_no, biz_key, source_type, source_id, status, review_note, created_at, input（media/ext）
ctx.audit：id, project_id, name, code, description, decision_mode, media_types, categories, default_result, on_model_fail, dedupe_window_sec, status, version
ctx.audit.category(code) / ctx.audit.hasCategory(code)
ctx.project：id, code, name
ctx.result：PASS / REJECT / REVIEW
ctx.detail：risk_level, score, summary, reasons, labels, category, category_name, category_confidence
ctx.payload：标准结果 JSON
ctx.variables()：模板变量，带真实类型
ctx.get('audit.code')：按路径读取
ctx.secret(name)：读取项目密钥（不要在脚本中写明文密码）
ctx.log(...)：写执行日志
ctx.mysql.connect({host, port, user, password, database}) / ctx.pg.connect({...})
  返回 db.query(sql, params)、db.exec(sql, params)、db.begin()、db.close()
ctx.http.request({url, method, headers, body, timeout_ms})：
  返回 { status, ok, body, body_base64, bytes, headers }
ctx.crypto：base64Encode/base64Decode/hexEncode/sha256/hmacSha256/randomBytes/uuid

【限制】
- 不能读写文件、不能下载文件、不能启动进程、不能访问 socket/WebSocket、不能读取环境变量。
- 每次执行有项目配置的内存和时长上限。
- 数据库连接在脚本结束后由平台自动关闭。
- 返回普通 JS 对象/数组/字符串/数字，平台会转成 JSON 记录；不要返回循环引用。
- 失败、超时、内存超限会导致动作执行失败，请增加必要判断。

【风格】
- 优先使用参数化 SQL。
- 写操作尽量幂等（结合 ctx.task.task_no / biz_key）。
- 不确定业务字段时，先给出通用模板，并说明需要用户补充哪些字段。
- 不要编造 ctx 中不存在的 API。`
