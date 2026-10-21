package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	triggerpkg "smartdoc-vision/backend/internal/trigger"
)

type triggerInput struct {
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
	Secret   string         `json:"secret"`
	Status   string         `json:"status"`
	Priority *int           `json:"priority"`
}

func (s *Server) updateTrigger(c *gin.Context) {
	user := currentUser(c)
	if !canManage(user.Role) {
		forbidden(c, "当前角色无权配置触发源")
		return
	}
	trigger, err := s.findTrigger(c)
	if err != nil {
		return
	}
	var req triggerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		trigger.Name = name
	}
	if req.Priority != nil {
		trigger.Priority = *req.Priority
	}
	if req.Status != "" {
		if !oneOf(req.Status, "ACTIVE", "DISABLED", "PAUSED") {
			badRequest(c, "触发源状态不合法")
			return
		}
		trigger.Status = req.Status
	}
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			badRequest(c, "config 格式错误")
			return
		}
		trigger.ConfigJSON = configBytes
	}
	if err := s.validateTrigger(trigger, req.Status != "" && req.Status == "ACTIVE"); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.Secret != "" {
		enc, err := s.cipher.Encrypt(req.Secret)
		if err != nil {
			serverError(c, err)
			return
		}
		trigger.SecretEnc = enc
	}
	trigger.SecretSet = trigger.SecretEnc != ""
	if err := s.db.Save(trigger).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &trigger.ProjectID, "TRIGGER_UPDATE", "TRIGGER", fmt.Sprint(trigger.ID), "SUCCESS",
		map[string]any{"audit_id": trigger.AuditID, "type": trigger.Type, "status": trigger.Status})
	ok(c, trigger)
}

func (s *Server) testTrigger(c *gin.Context) {
	user := currentUser(c)
	if !canManage(user.Role) {
		forbidden(c, "当前角色无权测试触发源")
		return
	}
	trigger, err := s.findTrigger(c)
	if err != nil {
		return
	}
	project, audit, err := s.triggerContext(trigger)
	if err != nil {
		serverError(c, err)
		return
	}
	result := gin.H{"type": trigger.Type, "ok": true}
	switch trigger.Type {
	case model.TriggerOpenAPI:
		result["submit_path"] = fmt.Sprintf("/open/v1/projects/%s/audits/%s/submit", project.Code, audit.Code)
		result["message"] = "开放 API 触发已就绪，使用项目凭证进行 HMAC 签名后提交"
	case model.TriggerWebSocket:
		result["connect_path"] = fmt.Sprintf("/open/v1/ws/projects/%s/audits/%s", project.Code, audit.Code)
		result["message"] = "WebSocket 接入地址已生成；启用触发源后可连接"
	case model.TriggerRedis:
		var cfg triggerpkg.RedisConfig
		_ = model.DecodeJSON(trigger.ConfigJSON, &cfg)
		if cfg.Stream == "" || cfg.Group == "" {
			fail(c, http.StatusUnprocessableEntity, "INVALID_CONFIG", "Redis Stream 与消费组不能为空", result)
			return
		}
		if err := s.redis.XGroupCreateMkStream(c.Request.Context(), cfg.Stream, cfg.Group, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			fail(c, http.StatusUnprocessableEntity, "REDIS_UNAVAILABLE", err.Error(), result)
			return
		}
		result["stream"] = cfg.Stream
		result["group"] = cfg.Group
		result["message"] = "Stream 与消费组已准备就绪"
	case model.TriggerPolling:
		var cfg triggerpkg.PollingConfig
		_ = model.DecodeJSON(trigger.ConfigJSON, &cfg)
		if cfg.URL == "" {
			fail(c, http.StatusUnprocessableEntity, "INVALID_CONFIG", "轮询 URL 不能为空", result)
			return
		}
		secret, _ := s.cipher.Decrypt(trigger.SecretEnc)
		page, fetchErr := triggerpkg.FetchPollingPage(c.Request.Context(), cfg, secret, trigger.CursorValue)
		if fetchErr != nil {
			fail(c, http.StatusUnprocessableEntity, "CONNECTION_TEST_FAILED", fetchErr.Error(), result)
			return
		}
		result["items"] = page.RawCount
		result["next_cursor"] = page.NextCursor
		result["message"] = fmt.Sprintf("轮询成功，读取到 %d 条任务数据", page.RawCount)
	}
	s.writeAudit(c, &trigger.ProjectID, "TRIGGER_TEST", "TRIGGER", fmt.Sprint(trigger.ID), "SUCCESS",
		map[string]any{"type": trigger.Type})
	ok(c, result)
}

func (s *Server) integrationGuide(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	var project model.Project
	if err := s.db.First(&project, audit.ProjectID).Error; err != nil {
		notFound(c, "项目不存在")
		return
	}
	var triggers []model.TriggerSource
	_ = s.db.Where("audit_id = ?", audit.ID).Order("priority asc").Find(&triggers).Error
	triggerMap := map[string]model.TriggerSource{}
	for _, t := range triggers {
		triggerMap[t.Type] = t
	}
	var credentials []model.APICredential
	_ = s.db.Where("project_id = ? AND status = ?", project.ID, model.CredentialActive).Order("id asc").Find(&credentials).Error
	eligible := make([]model.APICredential, 0, len(credentials))
	for _, credential := range credentials {
		if credentialAllowsAudit(credential, audit.ID) {
			eligible = append(eligible, credential)
		}
	}
	var selected *model.APICredential
	if requestedID, okID := parseUint(c.Query("credential_id")); okID {
		for i := range eligible {
			if eligible[i].ID == requestedID {
				selected = &eligible[i]
				break
			}
		}
	}
	if selected == nil && len(eligible) > 0 {
		selected = &eligible[0]
	}
	credentialViews := make([]gin.H, 0, len(eligible))
	for _, credential := range eligible {
		secret, _ := s.cipher.Decrypt(credential.SecretEnc)
		credentialViews = append(credentialViews, gin.H{
			"id": credential.ID, "name": credential.Name, "app_key": credential.AppKey,
			"secret_masked": security.MaskSecret(secret), "secret_set": secret != "",
		})
	}
	var selectedView any
	secret := ""
	appKey := ""
	if selected != nil {
		secret, _ = s.cipher.Decrypt(selected.SecretEnc)
		appKey = selected.AppKey
		selectedView = gin.H{
			"id": selected.ID, "name": selected.Name, "app_key": selected.AppKey,
			"secret_masked": security.MaskSecret(secret), "secret_set": secret != "",
		}
	}

	baseURL := requestBaseURL(c)
	submitPath := fmt.Sprintf("/open/v1/projects/%s/audits/%s/submit", project.Code, audit.Code)
	body := map[string]any{
		"biz_key": "archive:2026:0001",
		"media": []map[string]any{
			{"type": "text", "name": "page-1.txt", "content": "第一页档案文本 / OCR 结果"},
			{"type": "text", "name": "page-2.txt", "content": "第二页档案文本 / OCR 结果"},
			{"type": "image", "name": "page-1.jpg", "url": "https://your-domain.example.com/archive/page-1.jpg"},
			{"type": "image", "name": "page-2.jpg", "url": "https://your-domain.example.com/archive/page-2.jpg"},
		},
		"ext": map[string]any{"scene": "archive_review"},
	}
	bodyJSON, _ := json.Marshal(body)
	sampleHeaders := map[string]string{}
	sampleCurl := ""
	wsPath := fmt.Sprintf("/open/v1/ws/projects/%s/audits/%s", project.Code, audit.Code)
	wsCanonical := ""
	wsSignedURL := ""
	if selected != nil {
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		nonce := uuid.NewString()
		canonical := "POST\n" + submitPath + "\n" + timestamp + "\n" + nonce + "\n" + security.BodySHA256(bodyJSON)
		signature := security.SignHMAC(secret, canonical)
		sampleHeaders = map[string]string{
			"Content-Type": "application/json",
			"X-Api-Key":    appKey,
			"X-Timestamp":  timestamp,
			"X-Nonce":      nonce,
			"X-Signature":  signature,
		}
		sampleCurl = fmt.Sprintf(`curl -X POST '%s%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Api-Key: %s' \
  -H 'X-Timestamp: %s' \
  -H 'X-Nonce: %s' \
  -H 'X-Signature: %s' \
  -d '%s'`, baseURL, submitPath, appKey, timestamp, nonce, signature, string(bodyJSON))
		wsTimestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		wsNonce := uuid.NewString()
		wsCanonical = "GET\n" + wsPath + "\n" + wsTimestamp + "\n" + wsNonce + "\n" + security.BodySHA256(nil)
		wsSign := security.SignHMAC(secret, wsCanonical)
		wsQuery := url.Values{}
		wsQuery.Set("app_key", appKey)
		wsQuery.Set("timestamp", wsTimestamp)
		wsQuery.Set("nonce", wsNonce)
		wsQuery.Set("sign", wsSign)
		wsSignedURL = wsPath + "?" + wsQuery.Encode()
	}

	redisCfg := triggerpkg.RedisConfig{}
	if t, okT := triggerMap[model.TriggerRedis]; okT {
		_ = model.DecodeJSON(t.ConfigJSON, &redisCfg)
	}
	if redisCfg.Stream == "" {
		redisCfg.Stream = "sdv:" + project.Code + ":audit:" + audit.Code
	}
	if redisCfg.Group == "" {
		redisCfg.Group = fmt.Sprintf("sdv-cg-%d", audit.ID)
	}
	if redisCfg.BatchSize == 0 {
		redisCfg.BatchSize = 16
	}
	var callbackResults []string
	_ = model.DecodeJSON(audit.CallbackResults, &callbackResults)
	if len(callbackResults) == 0 {
		callbackResults = []string{model.ResultReject}
	}
	callbackHeaders := map[string]string{}
	_ = model.DecodeJSON(audit.CallbackHeaders, &callbackHeaders)

	triggerViews := make([]gin.H, 0, len(triggers))
	for _, t := range triggers {
		t.SecretSet = t.SecretEnc != ""
		triggerViews = append(triggerViews, gin.H{
			"id": t.ID, "type": t.Type, "name": t.Name, "status": t.Status,
			"priority": t.Priority, "config": t.ConfigJSON, "secret_set": t.SecretSet,
			"last_error": t.LastError, "last_consume_at": t.LastConsumeAt,
			"last_run_at": t.LastRunAt, "cursor": t.CursorValue,
		})
	}
	ok(c, gin.H{
		"audit": gin.H{
			"id": audit.ID, "name": audit.Name, "code": audit.Code, "status": audit.Status,
			"decision_mode": audit.DecisionMode, "categories": audit.Categories,
		},
		"project": gin.H{"id": project.ID, "name": project.Name, "code": project.Code},
		"credentials": gin.H{
			"eligible": credentialViews, "selected": selectedView, "manage_path": "/credentials",
			"scope": "凭证必须显式授权当前审核配置；审核删除或凭证停用后立即无法访问。",
		},
		"input_contract": gin.H{
			"media_types":         []string{"text", "image"},
			"supports_multi_text": true, "supports_multi_image": true, "supports_video": false,
			"max_images": 8,
			"note":       "当前版本仅支持多文本 + 多图片审核，不支持视频、音频。通用大模型需开启图片理解；JEV 原生支持多模态。",
		},
		"docs_path": "/help",
		"openapi": gin.H{
			"enabled":           triggerMap[model.TriggerOpenAPI].Status == "ACTIVE",
			"base_url":          baseURL,
			"path":              submitPath,
			"method":            "POST",
			"headers":           sampleHeaders,
			"signing_available": selected != nil,
			"canonical":         "HTTPMethod + \\n + Path + \\n + Timestamp + \\n + Nonce + \\n + SHA256HEX(body)",
			"sample_body":       body,
			"sample_body_raw":   string(bodyJSON),
			"sample_curl":       sampleCurl,
			"body_sha256":       security.BodySHA256(bodyJSON),
			"signing_note":      "签名有效期 5 分钟，Nonce 10 分钟内不可重复；X-Signature 为 HMAC-SHA256(AppSecret, CanonicalString) 的小写十六进制。",
			"query_task_path":   fmt.Sprintf("/open/v1/projects/%s/tasks/{task_no}", project.Code),
		},
		"redis": gin.H{
			"enabled":       triggerMap[model.TriggerRedis].Status == "ACTIVE",
			"stream":        redisCfg.Stream,
			"group":         redisCfg.Group,
			"batch_size":    redisCfg.BatchSize,
			"message_field": "data",
			"example":       body,
			"xadd_command":  fmt.Sprintf("XADD %s * data '<上面的 JSON 字符串>'", redisCfg.Stream),
			"note":          "平台使用当前 Redis DB 3 作为 Stream；PEL 超时消息由平台消费者继续处理。",
		},
		"websocket": gin.H{
			"enabled":           triggerMap[model.TriggerWebSocket].Status == "ACTIVE",
			"path":              wsPath,
			"signing_available": selected != nil,
			"signed_url":        wsSignedURL,
			"query":             []string{"app_key", "timestamp", "nonce", "sign"},
			"canonical":         wsCanonical,
			"submit_frame": map[string]any{
				"type": "submit",
				"data": body,
			},
			"ack_frame": map[string]any{"type": "ack", "data": map[string]any{"task_id": 123, "task_no": "SDV...", "status": "QUEUED"}},
		},
		"polling": gin.H{
			"enabled": triggerMap[model.TriggerPolling].Status == "ACTIVE",
			"config":  triggerMap[model.TriggerPolling].ConfigJSON,
			"response_contract": gin.H{
				"items":       "TaskInput 数组或嵌套在 items_path 指定的位置",
				"next_cursor": "由 cursor_path 指定，缺省为 next_cursor",
				"item":        map[string]any{"biz_key": "archive:2026:0001", "media": []map[string]any{{"type": "text", "content": "..."}}},
			},
			"note": "首次带空 cursor 请求；之后带上次响应中的 cursor。仅处理 items 中的任务对象。",
		},
		"callback": gin.H{
			"enabled":         audit.CallbackEnabled,
			"url":             audit.CallbackURL,
			"results":         callbackResults,
			"include_content": audit.CallbackIncludeContent,
			"secret_set":      audit.CallbackSecretEnc != "",
			"headers":         callbackHeaders,
			"signature":       "X-SmartDoc-Signature: sha256=HMAC-SHA256(CallbackSecret, raw_body)",
			"sample_payload": map[string]any{
				"event": "audit.completed", "task_no": "SDV...", "result": "REJECT",
				"category": "contract", "category_name": "合同协议", "category_confidence": 0.92,
				"summary": "命中文档完整性风险", "labels": []string{"内容安全"},
				"reasons": []string{"关键页缺失"}, "score": 0.93,
				"media": []any{map[string]any{"type": "image", "url": "https://..."}},
				"ext":   map[string]any{"scene": "archive_review"},
			},
		},
		"permissions": gin.H{
			"credential_scope": "AppKey / AppSecret 属于项目级凭证，可在「接入凭证」页面为每条凭证勾选允许访问的审核配置",
			"access":           "仅能访问该凭证已授权、且审核配置存在并启用的审核；审核删除、凭证停用或未授权时返回 403",
			"admin":            "项目管理员可在接入凭证页创建、轮换、停用、删除凭证；触发方式开关仍受审核配置权限控制",
		},
		"triggers": triggerViews,
	})
}

func (s *Server) findTrigger(c *gin.Context) (*model.TriggerSource, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "触发源 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var trigger model.TriggerSource
	if err := s.db.First(&trigger, id).Error; err != nil {
		notFound(c, "触发源不存在")
		return nil, err
	}
	if !s.requireProjectAccess(c, trigger.ProjectID) {
		return nil, fmt.Errorf("trigger out of scope")
	}
	trigger.SecretSet = trigger.SecretEnc != ""
	return &trigger, nil
}

func (s *Server) triggerContext(trigger *model.TriggerSource) (*model.Project, *model.Audit, error) {
	var project model.Project
	if err := s.db.First(&project, trigger.ProjectID).Error; err != nil {
		return nil, nil, err
	}
	var audit model.Audit
	if err := s.db.First(&audit, trigger.AuditID).Error; err != nil {
		return nil, nil, err
	}
	return &project, &audit, nil
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := c.GetHeader("X-Forwarded-Proto"); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := c.Request.Host
	if forwarded := c.GetHeader("X-Forwarded-Host"); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host
}

func (s *Server) validateTrigger(trigger *model.TriggerSource, enabling bool) error {
	switch trigger.Type {
	case model.TriggerRedis:
		var c triggerpkg.RedisConfig
		_ = model.DecodeJSON(trigger.ConfigJSON, &c)
		if enabling && (strings.TrimSpace(c.Stream) == "" || strings.TrimSpace(c.Group) == "") {
			return fmt.Errorf("启用 Redis 触发前必须填写 Stream 与消费组")
		}
		if c.BatchSize < 0 || c.BatchSize > 32 {
			return fmt.Errorf("Redis 批量大小需在 1~32 之间")
		}
	case model.TriggerPolling:
		var c triggerpkg.PollingConfig
		_ = model.DecodeJSON(trigger.ConfigJSON, &c)
		if enabling && strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("启用轮询前必须填写 URL")
		}
		if c.URL != "" {
			if err := validateBaseURL(c.URL); err != nil {
				return fmt.Errorf("轮询 URL 格式错误")
			}
		}
		if c.IntervalSec < 10 || c.IntervalSec > 86400 {
			return fmt.Errorf("轮询间隔需在 10~86400 秒之间")
		}
	case model.TriggerOpenAPI, model.TriggerWebSocket:
		// No extra fields.
	default:
		return fmt.Errorf("不支持的触发源类型")
	}
	return nil
}
