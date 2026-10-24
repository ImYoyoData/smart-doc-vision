package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/usage"
)

type providerView struct {
	model.AIProvider
	APIKeyMasked string        `json:"api_key_masked"`
	HasAPIKey    bool          `json:"has_api_key"`
	Usage        usage.Summary `json:"usage"`
}

type providerInput struct {
	ProjectID uint64          `json:"project_id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	BaseURL   string          `json:"base_url"`
	Model     string          `json:"model"`
	APIKey    string          `json:"api_key"`
	Prompt    string          `json:"prompt"`
	Config    json.RawMessage `json:"config"`
	Status    string          `json:"status"`
}

func (s *Server) listProviders(c *gin.Context) {
	q := s.db.Model(&model.AIProvider{})
	if pid := c.Query("project_id"); pid != "" {
		id, okID := parseUint(pid)
		if !okID {
			badRequest(c, "project_id 无效")
			return
		}
		if !s.requireProjectAccess(c, id) {
			return
		}
		q = q.Where("project_id = ?", id)
	} else {
		q = s.applyProjectScope(q, currentUser(c))
	}
	if kind := c.Query("kind"); kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var providers []model.AIProvider
	if err := q.Order("id asc").Find(&providers).Error; err != nil {
		serverError(c, err)
		return
	}
	views := make([]providerView, 0, len(providers))
	ids := make([]uint64, 0, len(providers))
	for _, p := range providers {
		ids = append(ids, p.ID)
	}
	summaries, err := usage.Summaries(s.db, ids)
	if err != nil {
		serverError(c, err)
		return
	}
	for _, p := range providers {
		view := s.toProviderView(p)
		view.Usage = summaries[p.ID]
		views = append(views, view)
	}
	ok(c, gin.H{"items": views, "total": len(views)})
}

// providerUsage returns recent request details plus lifetime totals for one
// model. JEV only reports input tokens because its output is not billed.
func (s *Server) providerUsage(c *gin.Context) {
	provider, err := s.findProvider(c)
	if err != nil {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if parsed, okLimit := parseUint(raw); okLimit && parsed > 0 {
			limit = int(parsed)
		}
	}
	rows, err := usage.Recent(s.db, provider.ID, limit)
	if err != nil {
		serverError(c, err)
		return
	}
	summaries, err := usage.Summaries(s.db, []uint64{provider.ID})
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{
		"items":   rows,
		"total":   len(rows),
		"summary": summaries[provider.ID],
		"kind":    provider.Kind,
		"model":   provider.Model,
	})
}

func (s *Server) getProvider(c *gin.Context) {
	provider, err := s.findProvider(c)
	if err != nil {
		return
	}
	ok(c, s.toProviderView(*provider))
}

func (s *Server) createProvider(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "provider:write") {
		forbidden(c, "当前角色无权配置模型")
		return
	}
	var req providerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if req.ProjectID == 0 {
		badRequest(c, "请选择项目")
		return
	}
	var project model.Project
	if err := s.db.Where("id = ? AND deleted_at IS NULL", req.ProjectID).First(&project).Error; err != nil {
		notFound(c, "项目不存在或已删除")
		return
	}
	if !s.requireProjectAccess(c, project.ID) {
		return
	}
	if err := validateProviderInput(&req, true); err != nil {
		badRequest(c, err.Error())
		return
	}
	var count int64
	_ = s.db.Model(&model.AIProvider{}).Where("project_id = ? AND name = ?", req.ProjectID, req.Name).Count(&count).Error
	if count > 0 {
		conflict(c, "NAME_CONFLICT", "当前项目下已存在同名模型配置")
		return
	}
	keyEnc, err := s.cipher.Encrypt(req.APIKey)
	if err != nil {
		serverError(c, err)
		return
	}
	config := req.Config
	if len(config) == 0 || string(config) == "null" {
		config = json.RawMessage(catalog.DefaultProviderConfig(req.Kind, false))
	} else if !json.Valid(config) {
		badRequest(c, "config 必须是合法 JSON")
		return
	}
	status := req.Status
	if status == "" {
		if strings.TrimSpace(req.APIKey) != "" {
			status = model.ProviderActive
		} else {
			status = model.ProviderDisabled
		}
	}
	provider := model.AIProvider{
		ProjectID: req.ProjectID, Name: req.Name, Kind: req.Kind,
		BaseURL: strings.TrimRight(req.BaseURL, "/"), Model: req.Model,
		APIKeyEnc: keyEnc, Prompt: req.Prompt, ConfigJSON: model.JSONText(config), Status: status,
	}
	if err := s.db.Create(&provider).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &provider.ProjectID, "PROVIDER_CREATE", "PROVIDER", fmt.Sprint(provider.ID), "SUCCESS",
		map[string]any{"name": provider.Name, "kind": provider.Kind, "model": provider.Model})
	created(c, s.toProviderView(provider))
}

func (s *Server) updateProvider(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "provider:write") {
		forbidden(c, "当前角色无权配置模型")
		return
	}
	provider, err := s.findProvider(c)
	if err != nil {
		return
	}
	var req providerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		var count int64
		_ = s.db.Model(&model.AIProvider{}).Where("project_id = ? AND name = ? AND id <> ?", provider.ProjectID, name, provider.ID).Count(&count).Error
		if count > 0 {
			conflict(c, "NAME_CONFLICT", "当前项目下已存在同名模型配置")
			return
		}
		provider.Name = name
	}
	if req.Kind != "" && req.Kind != provider.Kind {
		badRequest(c, "模型类型创建后不可变更，请新建供应商")
		return
	}
	if req.BaseURL != "" {
		if err := validateBaseURL(req.BaseURL); err != nil {
			badRequest(c, err.Error())
			return
		}
		provider.BaseURL = strings.TrimRight(req.BaseURL, "/")
	}
	if req.Model != "" {
		provider.Model = req.Model
	}
	provider.Prompt = req.Prompt
	if req.APIKey != "" {
		enc, err := s.cipher.Encrypt(req.APIKey)
		if err != nil {
			serverError(c, err)
			return
		}
		provider.APIKeyEnc = enc
	}
	if len(req.Config) > 0 && string(req.Config) != "null" {
		if !json.Valid(req.Config) {
			badRequest(c, "config 必须是合法 JSON")
			return
		}
		provider.ConfigJSON = model.JSONText(req.Config)
	}
	if oneOf(req.Status, model.ProviderActive, model.ProviderDisabled) {
		provider.Status = req.Status
	}
	if err := s.db.Save(provider).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &provider.ProjectID, "PROVIDER_UPDATE", "PROVIDER", fmt.Sprint(provider.ID), "SUCCESS",
		map[string]any{"name": provider.Name, "model": provider.Model, "api_key_changed": req.APIKey != ""})
	ok(c, s.toProviderView(*provider))
}

func (s *Server) deleteProvider(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "provider:write") {
		forbidden(c, "当前角色无权删除模型配置")
		return
	}
	provider, err := s.findProvider(c)
	if err != nil {
		return
	}
	var count int64
	_ = s.db.Model(&model.Audit{}).Where("provider_id = ? AND deleted_at IS NULL", provider.ID).Count(&count).Error
	if count > 0 {
		conflict(c, "PROVIDER_IN_USE", fmt.Sprintf("该供应商正被 %d 条审核配置使用，请先切换审核所用模型", count))
		return
	}
	if err := s.db.Delete(provider).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &provider.ProjectID, "PROVIDER_DELETE", "PROVIDER", fmt.Sprint(provider.ID), "SUCCESS",
		map[string]any{"name": provider.Name, "kind": provider.Kind})
	ok(c, gin.H{"deleted": true})
}

func (s *Server) testProvider(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "provider:write") {
		forbidden(c, "当前角色无权测试模型连接")
		return
	}
	provider, err := s.findProvider(c)
	if err != nil {
		return
	}
	key, err := s.cipher.Decrypt(provider.APIKeyEnc)
	if err != nil {
		serverError(c, fmt.Errorf("密钥解密失败: %w", err))
		return
	}
	adapter, err := ai.AdapterFor(provider.Kind)
	if err != nil {
		serverError(c, err)
		return
	}
	// Connection tests must not reuse a fixed gateway session ID, because some
	// OpenCode-compatible gateways reject an already-seen session.
	testProvider := ai.WithRandomSessionHeader(*provider)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	result, testErr := adapter.Test(ctx, ai.Request{Provider: testProvider, APIKey: key})
	updates := map[string]any{
		"last_test_at":  time.Now(),
		"last_test_ok":  result.OK,
		"last_test_ms":  result.LatencyMS,
		"last_test_msg": truncateText(result.Message, 500),
	}
	if result.OK {
		updates["breaker_until"] = nil
		updates["breaker_reason"] = ""
		updates["failure_count"] = 0
	}
	message := result.Message
	if message == "" && testErr != nil {
		message = testErr.Error()
	}
	updates["last_test_msg"] = truncateText(message, 500)
	if err := s.db.Model(&model.AIProvider{}).Where("id = ?", provider.ID).Updates(updates).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &provider.ProjectID, "PROVIDER_TEST", "PROVIDER", fmt.Sprint(provider.ID), pickResult(result.OK), map[string]any{
		"kind": provider.Kind, "ok": result.OK, "latency_ms": result.LatencyMS, "message": truncateText(message, 200),
	})
	if testErr != nil {
		result.Message = message
		fail(c, http.StatusUnprocessableEntity, "CONNECTION_TEST_FAILED", message, result)
		return
	}
	ok(c, result)
}

func (s *Server) findProvider(c *gin.Context) (*model.AIProvider, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "供应商 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var provider model.AIProvider
	if err := s.db.First(&provider, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			notFound(c, "模型供应商不存在")
		} else {
			serverError(c, err)
		}
		return nil, err
	}
	if !s.requireProjectAccess(c, provider.ProjectID) {
		return nil, fmt.Errorf("provider out of scope")
	}
	return &provider, nil
}

func (s *Server) toProviderView(p model.AIProvider) providerView {
	key, _ := s.cipher.Decrypt(p.APIKeyEnc)
	return providerView{
		AIProvider:   p,
		APIKeyMasked: security.MaskSecret(key),
		HasAPIKey:    key != "",
	}
}

func validateProviderInput(req *providerInput, creating bool) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.ToUpper(strings.TrimSpace(req.Kind))
	switch req.Kind {
	case "OPENAI":
		req.Kind = model.ProviderOpenAI
	case "CLAUDE":
		req.Kind = model.ProviderAnthropic
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.Model = strings.TrimSpace(req.Model)
	if req.Name == "" || len([]rune(req.Name)) > 64 {
		return fmt.Errorf("请填写供应商名称（不超过 64 字）")
	}
	if !oneOf(req.Kind, model.ProviderOpenAI, model.ProviderAnthropic, model.ProviderJEV) {
		return fmt.Errorf("供应商类型需为 OPENAI_COMPATIBLE、ANTHROPIC 或 JEV")
	}
	if req.BaseURL == "" {
		req.BaseURL, req.Model = catalog.ProviderDefaults(req.Kind)
	}
	if err := validateBaseURL(req.BaseURL); err != nil {
		return err
	}
	if req.Model == "" {
		_, defaultModel := catalog.ProviderDefaults(req.Kind)
		req.Model = defaultModel
	}
	if req.Status != "" && !oneOf(req.Status, model.ProviderActive, model.ProviderDisabled) {
		return fmt.Errorf("状态取值不合法")
	}
	return nil
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("BaseURL 格式错误")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("BaseURL 必须以 http:// 或 https:// 开头")
	}
	if u.Host == "" {
		return fmt.Errorf("BaseURL 缺少主机名")
	}
	return nil
}

func pickResult(ok bool) string {
	if ok {
		return "SUCCESS"
	}
	return "FAILED"
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
