package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type apiClient struct {
	base  string
	http  *http.Client
	token string
	redis *redis.Client
}

type apiEnvelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

type project struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type provider struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type redisConnection struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Health string `json:"health"`
}

type audit struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type task struct {
	ID               uint64         `json:"id"`
	TaskNo           string         `json:"task_no"`
	ProjectID        uint64         `json:"project_id"`
	AuditID          uint64         `json:"audit_id"`
	BizKey           string         `json:"biz_key"`
	Status           string         `json:"status"`
	Result           string         `json:"result"`
	ResultDetail     map[string]any `json:"result_detail"`
	ErrorMessage     string         `json:"error_message"`
	DurationMS       int64          `json:"duration_ms"`
	ReviewNote       string         `json:"review_note"`
	ReviewReasonCode string         `json:"review_reason_code"`
}

type scriptExecution struct {
	ID         uint64          `json:"id"`
	TaskNo     string          `json:"task_no"`
	Status     string          `json:"status"`
	ErrorType  string          `json:"error_type"`
	Error      string          `json:"error_message"`
	ResultJSON json.RawMessage `json:"result_json"`
}

type taskDetail struct {
	Task   task             `json:"task"`
	Events []map[string]any `json:"events"`
}

func newAPIClient(base string, appRedis *redis.Client) *apiClient {
	return &apiClient{
		base:  strings.TrimRight(base, "/"),
		http:  &http.Client{Timeout: 60 * time.Second},
		redis: appRedis,
	}
}

func (c *apiClient) do(ctx context.Context, method, path string, body any, out any, auth bool) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var env apiEnvelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{Status: resp.StatusCode, Code: fmt.Sprint(env.Code), Message: env.Message}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

func login(ctx context.Context, c *apiClient, te *testEnv) error {
	var challenge struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/admin/v1/auth/captcha", nil, &challenge, false); err != nil {
		return fmt.Errorf("captcha: %w", err)
	}
	raw, err := c.redis.Get(ctx, "sdv:auth:captcha:"+challenge.ID).Result()
	if err != nil {
		return fmt.Errorf("captcha answer: %w", err)
	}
	var answer struct {
		X int `json:"x"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return err
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	payload := map[string]any{
		"username": te.username, "password": te.password,
		"captcha_id": challenge.ID, "captcha_x": answer.X,
	}
	if err := c.do(ctx, http.MethodPost, "/api/admin/v1/auth/login", payload, &result, false); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if result.AccessToken == "" {
		return fmt.Errorf("login returned empty token")
	}
	c.token = result.AccessToken
	return nil
}

func (c *apiClient) createProject(ctx context.Context, name, code string) (*project, error) {
	return c.createProjectWithOptions(ctx, name, code, nil)
}

func (c *apiClient) createProjectWithOptions(ctx context.Context, name, code string, options map[string]any) (*project, error) {
	payload := map[string]any{
		"name": name, "code": code, "env": "test", "description": "QA100 automated test project",
		"retention_days": 7, "max_tasks_per_day": 0, "max_concurrent_tasks": 8,
		"js_max_memory_mb": 64, "js_timeout_ms": 5000, "js_max_concurrency": 4,
	}
	for key, value := range options {
		payload[key] = value
	}
	var p project
	err := c.do(ctx, http.MethodPost, "/api/admin/v1/projects", payload, &p, true)
	return &p, err
}

func (c *apiClient) getProject(ctx context.Context, id uint64) (*project, error) {
	var p project
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/admin/v1/projects/%d", id), nil, &p, true)
	return &p, err
}

func (c *apiClient) createProvider(ctx context.Context, projectID uint64, name, baseURL, model string) (*provider, error) {
	return c.createProviderWithConfig(ctx, projectID, name, baseURL, model, map[string]any{
		"supports_vision": false, "max_tokens": 1024, "temperature": 0.1, "max_images": 1,
	}, 1)
}

func (c *apiClient) createProviderWithConfig(ctx context.Context, projectID uint64, name, baseURL, model string, config map[string]any, priority int) (*provider, error) {
	var p provider
	err := c.do(ctx, http.MethodPost, "/api/admin/v1/providers", map[string]any{
		"project_id": projectID, "name": name, "kind": "OPENAI_COMPATIBLE",
		"base_url": baseURL, "model": model, "api_key": "qa100-test-key",
		"priority": priority, "status": "ACTIVE", "config": config,
	}, &p, true)
	return &p, err
}

func (c *apiClient) testProvider(ctx context.Context, id uint64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/admin/v1/providers/%d/test", id), map[string]any{}, nil, true)
}

func (c *apiClient) createRedisConnection(ctx context.Context, projectID uint64, name, addr string) (*redisConnection, error) {
	var conn redisConnection
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/admin/v1/projects/%d/redis-connections", projectID), map[string]any{
		"name": name, "addr": addr, "password": "", "status": "ACTIVE",
		"auto_resume_triggers": false, "tls": false,
	}, &conn, true)
	return &conn, err
}

func (c *apiClient) testRedisConnection(ctx context.Context, id uint64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/admin/v1/redis-connections/%d/test", id), map[string]any{}, nil, true)
}

func (c *apiClient) createAudit(ctx context.Context, payload map[string]any) (*audit, error) {
	var a audit
	err := c.do(ctx, http.MethodPost, "/api/admin/v1/audits", payload, &a, true)
	return &a, err
}

func (c *apiClient) updateAuditStatus(ctx context.Context, id uint64, status string) error {
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/admin/v1/audits/%d/status", id), map[string]any{"status": status}, nil, true)
}

func (c *apiClient) getAudit(ctx context.Context, id uint64) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/admin/v1/audits/%d", id), nil, &out, true)
	return out, err
}

func (c *apiClient) updateAudit(ctx context.Context, id uint64, payload map[string]any) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/admin/v1/audits/%d", id), payload, nil, true)
}

func (c *apiClient) submitTask(ctx context.Context, projectID, auditID uint64, bizKey, text string, ext map[string]any, media []map[string]any) (*task, bool, error) {
	payload := map[string]any{
		"project_id": projectID, "audit_id": auditID,
		"biz_key": bizKey, "text": text, "ext": ext,
	}
	if media != nil {
		payload["media"] = media
		delete(payload, "text")
	}
	var result struct {
		Task       task `json:"task"`
		Duplicated bool `json:"duplicated"`
	}
	err := c.do(ctx, http.MethodPost, "/api/admin/v1/tasks", payload, &result, true)
	return &result.Task, result.Duplicated, err
}

func (c *apiClient) getTask(ctx context.Context, id uint64) (*task, []map[string]any, error) {
	var detail taskDetail
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/admin/v1/tasks/%d", id), nil, &detail, true); err != nil {
		return nil, nil, err
	}
	return &detail.Task, detail.Events, nil
}

func (c *apiClient) reviewTask(ctx context.Context, id uint64, conclusion, reason, remark string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/admin/v1/tasks/%d/review", id), map[string]any{
		"conclusion": conclusion, "reason_code": reason, "remark": remark,
	}, nil, true)
}

func (c *apiClient) getScriptExecution(ctx context.Context, id uint64) (*scriptExecution, error) {
	var item scriptExecution
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/admin/v1/script-executions/%d", id), nil, &item, true)
	return &item, err
}

func (c *apiClient) listScriptExecutions(ctx context.Context, auditID uint64, taskNo string) ([]scriptExecution, error) {
	var out struct {
		Items []scriptExecution `json:"items"`
		Total int               `json:"total"`
	}
	path := fmt.Sprintf("/api/admin/v1/audits/%d/script-executions?task_no=%s", auditID, url.QueryEscape(taskNo))
	err := c.do(ctx, http.MethodGet, path, nil, &out, true)
	return out.Items, err
}

func (c *apiClient) reviewSummary(ctx context.Context, projectID uint64) (map[string]any, error) {
	var out map[string]any
	path := "/api/admin/v1/reviews/summary"
	if projectID != 0 {
		path += fmt.Sprintf("?project_id=%d", projectID)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out, true)
	return out, err
}
