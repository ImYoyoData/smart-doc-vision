package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/redisconn"
)

type routeConfig struct {
	ExecMode string        `json:"exec_mode"`
	OnError  string        `json:"on_error"`
	Actions  []routeAction `json:"actions"`
}

type routeAction struct {
	Type         string            `json:"type"`
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers"`
	Params       map[string]string `json:"params"`
	Body         string            `json:"body"`
	TimeoutMS    int               `json:"timeout_ms"`
	Command      string            `json:"command"`
	Target       string            `json:"target"`
	Payload      string            `json:"payload"`
	ConnectionID uint64            `json:"connection_id"`
	DB           int               `json:"db"`
	Remark       string            `json:"remark"`
	Script       string            `json:"script"`
	MemoryMB     int               `json:"memory_mb"`
}

// actionError carries the failure category and script execution id back to the
// task's route results.
type actionError struct {
	Type        string
	Err         error
	ExecutionID uint64
}

func (e *actionError) Error() string { return e.Err.Error() }
func (e *actionError) Unwrap() error { return e.Err }

type routeRender struct {
	Task        model.Task
	Audit       model.Audit
	Project     model.Project
	Input       model.TaskInput
	FinalResult string
	Detail      map[string]any
}

// ExecuteRoutes runs the configured result actions for a task after a manual
// review. The manual conclusion is authoritative; action failures are recorded
// but do not change it back to REVIEW.
func (e *Engine) ExecuteRoutes(taskID uint64) {
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return
	}
	var audit model.Audit
	if err := e.db.First(&audit, task.AuditID).Error; err != nil {
		return
	}
	var input model.TaskInput
	_ = model.DecodeJSON(task.InputJSON, &input)
	var detail map[string]any
	_ = model.DecodeJSON(task.ResultJSON, &detail)
	if detail == nil {
		detail = map[string]any{}
	}
	results, _ := e.runRoutes(audit, task, task.Result, input, detail)
	if results == nil {
		return
	}
	detail["routes"] = results
	_ = e.db.Model(&model.Task{}).Where("id = ?", task.ID).
		Update("result_json", model.JSONBytes(detail)).Error
}

func (e *Engine) runRoutes(audit model.Audit, task model.Task, finalResult string, input model.TaskInput, detail map[string]any) ([]map[string]any, string) {
	var routes map[string]routeConfig
	if len(audit.Routes) == 0 || model.DecodeJSON(audit.Routes, &routes) != nil {
		return nil, ""
	}
	route, ok := routes[finalResult]
	if !ok || len(route.Actions) == 0 {
		return nil, ""
	}
	var project model.Project
	_ = e.db.First(&project, task.ProjectID).Error
	render := routeRender{Task: task, Audit: audit, Project: project, Input: input, FinalResult: finalResult, Detail: detail}
	results := make([]map[string]any, 0, len(route.Actions))
	override := ""
	for index, action := range route.Actions {
		start := time.Now()
		err := e.executeRouteAction(render, action, index)
		entry := map[string]any{
			"index": index, "type": action.Type, "ok": err == nil,
			"started_at":  start.UTC().Format(time.RFC3339),
			"duration_ms": time.Since(start).Milliseconds(),
		}
		if action.Type == "js" {
			entry["remark"] = action.Remark
		}
		if err != nil {
			entry["error"] = truncate(err.Error(), 500)
			var actionErr *actionError
			if errors.As(err, &actionErr) {
				entry["error_type"] = actionErr.Type
				if actionErr.ExecutionID > 0 {
					entry["execution_id"] = actionErr.ExecutionID
				}
			}
		}
		results = append(results, entry)
		if err == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(route.OnError)) {
		case "to_review":
			override = model.ResultReview
			return results, override
		case "stop":
			return results, override
		default:
			// continue with the remaining actions
		}
	}
	return results, override
}

func (e *Engine) executeRouteAction(render routeRender, action routeAction, index int) error {
	switch strings.ToLower(strings.TrimSpace(action.Type)) {
	case "http":
		return e.executeHTTPAction(render, action)
	case "websocket":
		return e.executeWebSocketAction(render, action)
	case "redis":
		return e.executeRedisAction(render, action)
	case "js":
		return e.executeJSAction(render, action, index)
	default:
		return fmt.Errorf("不支持的动作类型 %q", action.Type)
	}
}

func routePayload(render routeRender) map[string]any {
	return map[string]any{
		"event": "audit.result_action", "task_no": render.Task.TaskNo,
		"project_code": render.Project.Code, "audit_code": render.Audit.Code,
		"biz_key": render.Task.BizKey, "result": render.FinalResult,
		"summary": render.Detail["summary"], "reasons": render.Detail["reasons"],
		"labels": render.Detail["labels"], "risk_level": render.Detail["risk_level"],
		"score": render.Detail["score"], "category": render.Detail["category"],
		"category_name":       render.Detail["category_name"],
		"category_confidence": render.Detail["category_confidence"],
		"ext":                 render.Input.Ext,
	}
}

func (e *Engine) executeHTTPAction(render routeRender, action routeAction) error {
	target := renderString(action.URL, render)
	if target == "" {
		return fmt.Errorf("HTTP 动作缺少 url")
	}
	if len(action.Params) > 0 {
		u, err := url.Parse(target)
		if err != nil {
			return fmt.Errorf("HTTP 动作 url 无效: %w", err)
		}
		query := u.Query()
		for key, value := range action.Params {
			if strings.TrimSpace(key) != "" {
				query.Set(key, renderString(value, render))
			}
		}
		u.RawQuery = query.Encode()
		target = u.String()
	}
	if !e.cfg.AllowPrivate {
		if err := media.ValidateOutboundURL(target); err != nil {
			return fmt.Errorf("HTTP 动作地址未通过 SSRF 校验: %w", err)
		}
	}
	method := strings.ToUpper(strings.TrimSpace(action.Method))
	if method == "" {
		method = http.MethodPost
	}
	body := renderString(action.Body, render)
	if strings.TrimSpace(body) == "" {
		raw, _ := json.Marshal(routePayload(render))
		body = string(raw)
	}
	timeout := time.Duration(action.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(e.ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range action.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, renderString(value, render))
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "SmartDoc-Vision-Route/1.0")
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return nil
}

func validateWebSocketURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("WebSocket 地址无效: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return fmt.Errorf("WebSocket 动作仅支持 ws/wss 地址")
	}
	return media.ValidateOutboundURL(u.String())
}

func isWebSocketURL(target string) bool {
	lower := strings.ToLower(strings.TrimSpace(target))
	return strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://")
}

func sendWebSocket(ctx context.Context, target string, headers map[string]string, payload string, timeout time.Duration) error {
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	reqHeader := http.Header{}
	reqHeader.Set("User-Agent", "SmartDoc-Vision-Route/1.0")
	for key, value := range headers {
		if strings.TrimSpace(key) != "" {
			reqHeader.Set(key, value)
		}
	}
	conn, resp, err := dialer.DialContext(ctx, target, reqHeader)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("WebSocket 握手失败 HTTP %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		return fmt.Errorf("WebSocket 发送失败: %w", err)
	}
	return nil
}

func (e *Engine) executeWebSocketAction(render routeRender, action routeAction) error {
	target := renderString(action.URL, render)
	if target == "" {
		return fmt.Errorf("WebSocket 动作缺少 url")
	}
	if !e.cfg.AllowPrivate {
		if err := validateWebSocketURL(target); err != nil {
			return fmt.Errorf("WebSocket 动作地址未通过 SSRF 校验: %w", err)
		}
	} else if !isWebSocketURL(target) {
		return fmt.Errorf("WebSocket 动作仅支持 ws/wss 地址")
	}
	payload := renderString(action.Payload, render)
	if strings.TrimSpace(payload) == "" {
		payload = renderString(action.Body, render)
	}
	if strings.TrimSpace(payload) == "" {
		raw, _ := json.Marshal(routePayload(render))
		payload = string(raw)
	}
	headers := map[string]string{}
	for key, value := range action.Headers {
		if strings.TrimSpace(key) != "" {
			headers[key] = renderString(value, render)
		}
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	timeout := time.Duration(action.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(e.ctx, timeout)
	defer cancel()
	return sendWebSocket(ctx, target, headers, payload, timeout)
}

func (e *Engine) executeRedisAction(render routeRender, action routeAction) error {
	if action.ConnectionID == 0 {
		return fmt.Errorf("Redis 动作必须选择项目 Redis 连接")
	}
	var connection model.RedisConnection
	if err := e.db.Where("id = ? AND project_id = ?", action.ConnectionID, render.Project.ID).First(&connection).Error; err != nil {
		return fmt.Errorf("Redis 连接不存在或不属于当前项目")
	}
	if connection.Status != model.RedisConnectionActive {
		return fmt.Errorf("Redis 连接「%s」已停用", connection.Name)
	}
	if !connection.LastTestOK || connection.Health != model.RedisHealthOK {
		return fmt.Errorf("Redis 连接「%s」未通过连接测试", connection.Name)
	}
	password := ""
	if connection.PasswordEnc != "" {
		decrypted, err := e.cipher.Decrypt(connection.PasswordEnc)
		if err != nil {
			return fmt.Errorf("Redis 连接密码解密失败")
		}
		password = decrypted
	}
	if action.DB < 0 || action.DB > 255 {
		return fmt.Errorf("Redis DB 编号需在 0~255 之间")
	}
	target := renderString(action.Target, render)
	if target == "" {
		return fmt.Errorf("Redis 动作缺少 target")
	}
	payload := renderString(action.Payload, render)
	if strings.TrimSpace(payload) == "" {
		raw, _ := json.Marshal(routePayload(render))
		payload = string(raw)
	}
	client := redisconn.New(connection, password, action.DB)
	defer client.Close()
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(action.Command)) {
	case "xadd":
		return client.XAdd(ctx, &redis.XAddArgs{Stream: target, Values: map[string]any{"data": payload}}).Err()
	case "set":
		return client.Set(ctx, target, payload, 0).Err()
	default:
		return client.Publish(ctx, target, payload).Err()
	}
}

func detailText(detail map[string]any, key string) string {
	if value, ok := detail[key]; ok && value != nil {
		return fmt.Sprint(value)
	}
	return ""
}

func renderString(input string, render routeRender) string {
	replacer := strings.NewReplacer(
		"${task_no}", render.Task.TaskNo,
		"${task_id}", fmt.Sprint(render.Task.ID),
		"${biz_key}", render.Task.BizKey,
		"${source_type}", render.Task.SourceType,
		"${project_code}", render.Project.Code,
		"${project_id}", fmt.Sprint(render.Project.ID),
		"${audit_code}", render.Audit.Code,
		"${audit_id}", fmt.Sprint(render.Audit.ID),
		"${result}", render.FinalResult,
		"${summary}", detailText(render.Detail, "summary"),
		"${reasons}", detailText(render.Detail, "reasons"),
		"${labels}", detailText(render.Detail, "labels"),
		"${risk_level}", detailText(render.Detail, "risk_level"),
		"${score}", detailText(render.Detail, "score"),
		"${category}", detailText(render.Detail, "category"),
		"${category_name}", detailText(render.Detail, "category_name"),
		"${category_confidence}", detailText(render.Detail, "category_confidence"),
		"${review_note}", render.Task.ReviewNote,
	)
	return replacer.Replace(input)
}
