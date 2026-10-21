package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
)

type routeConfig struct {
	ExecMode string        `json:"exec_mode"`
	OnError  string        `json:"on_error"`
	Actions  []routeAction `json:"actions"`
}

type routeAction struct {
	Type      string            `json:"type"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	TimeoutMS int               `json:"timeout_ms"`
	Command   string            `json:"command"`
	Target    string            `json:"target"`
	Payload   string            `json:"payload"`
	DSNEnv    string            `json:"dsn_env"`
	Query     string            `json:"query"`
	Args      []any             `json:"args"`
}

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
		err := e.executeRouteAction(render, action)
		entry := map[string]any{
			"index": index, "type": action.Type, "ok": err == nil,
			"duration_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			entry["error"] = truncate(err.Error(), 500)
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

func (e *Engine) executeRouteAction(render routeRender, action routeAction) error {
	switch strings.ToLower(strings.TrimSpace(action.Type)) {
	case "http":
		return e.executeHTTPAction(render, action)
	case "redis":
		return e.executeRedisAction(render, action)
	case "mysql":
		return e.executeMySQLAction(render, action)
	default:
		return fmt.Errorf("不支持的动作类型 %q", action.Type)
	}
}

func (e *Engine) executeHTTPAction(render routeRender, action routeAction) error {
	target := renderString(action.URL, render)
	if target == "" {
		return fmt.Errorf("HTTP 动作缺少 url")
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
		payload := map[string]any{
			"event": "audit.result_action", "task_no": render.Task.TaskNo,
			"project_code": render.Project.Code, "audit_code": render.Audit.Code,
			"biz_key": render.Task.BizKey, "result": render.FinalResult,
			"summary": render.Detail["summary"], "reasons": render.Detail["reasons"],
			"labels": render.Detail["labels"], "ext": render.Input.Ext,
		}
		raw, _ := json.Marshal(payload)
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

func (e *Engine) executeRedisAction(render routeRender, action routeAction) error {
	if e.redis == nil {
		return fmt.Errorf("Redis 未配置")
	}
	target := renderString(action.Target, render)
	if target == "" {
		return fmt.Errorf("Redis 动作缺少 target")
	}
	payload := renderString(action.Payload, render)
	if strings.TrimSpace(payload) == "" {
		raw, _ := json.Marshal(map[string]any{
			"task_no": render.Task.TaskNo, "biz_key": render.Task.BizKey,
			"project_code": render.Project.Code, "audit_code": render.Audit.Code,
			"result": render.FinalResult, "summary": render.Detail["summary"],
			"reasons": render.Detail["reasons"], "labels": render.Detail["labels"],
		})
		payload = string(raw)
	}
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(action.Command)) {
	case "xadd":
		return e.redis.XAdd(ctx, &redis.XAddArgs{Stream: target, Values: map[string]any{"data": payload}}).Err()
	case "set":
		return e.redis.Set(ctx, target, payload, 0).Err()
	default:
		return e.redis.Publish(ctx, target, payload).Err()
	}
}

func (e *Engine) executeMySQLAction(render routeRender, action routeAction) error {
	dsn := strings.TrimSpace(os.Getenv(strings.TrimSpace(action.DSNEnv)))
	if dsn == "" {
		return fmt.Errorf("MySQL 动作缺少环境变量 %s 或对应 DSN 为空", action.DSNEnv)
	}
	query := strings.TrimSpace(renderString(action.Query, render))
	if query == "" {
		return fmt.Errorf("MySQL 动作缺少 query")
	}
	args := make([]any, 0, len(action.Args))
	for _, value := range action.Args {
		switch v := value.(type) {
		case string:
			args = append(args, renderString(v, render))
		default:
			args = append(args, v)
		}
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(e.ctx, 8*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, query, args...)
	return err
}

func renderString(input string, render routeRender) string {
	replacer := strings.NewReplacer(
		"${task_no}", render.Task.TaskNo,
		"${task_id}", fmt.Sprint(render.Task.ID),
		"${biz_key}", render.Task.BizKey,
		"${project_code}", render.Project.Code,
		"${project_id}", fmt.Sprint(render.Project.ID),
		"${audit_code}", render.Audit.Code,
		"${audit_id}", fmt.Sprint(render.Audit.ID),
		"${result}", render.FinalResult,
		"${summary}", fmt.Sprint(render.Detail["summary"]),
	)
	return replacer.Replace(input)
}
