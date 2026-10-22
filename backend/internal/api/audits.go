package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
)

var (
	auditCodePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)
	categoryCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
)

type auditInput struct {
	ProjectID       uint64                `json:"project_id"`
	Name            string                `json:"name"`
	Code            string                `json:"code"`
	Description     string                `json:"description"`
	MediaTypes      []string              `json:"media_types"`
	DecisionMode    string                `json:"decision_mode"`
	Categories      []model.AuditCategory `json:"categories"`
	ProviderID      uint64                `json:"provider_id"`
	ReviewPrompt    string                `json:"review_prompt"`
	Strictness      string                `json:"strictness"`
	Rules           json.RawMessage       `json:"rules"`
	Routes          json.RawMessage       `json:"routes"`
	DefaultResult   string                `json:"default_result"`
	OnModelFail     string                `json:"on_model_fail"`
	DedupeWindowSec *int                  `json:"dedupe_window_sec"`
	Status          string                `json:"status"`

	CallbackEnabled        *bool             `json:"callback_enabled"`
	CallbackURL            string            `json:"callback_url"`
	CallbackSecret         string            `json:"callback_secret"`
	CallbackResults        []string          `json:"callback_results"`
	CallbackHeaders        map[string]string `json:"callback_headers"`
	CallbackIncludeContent *bool             `json:"callback_include_content"`
}

func (s *Server) listAudits(c *gin.Context) {
	q := s.db.Model(&model.Audit{}).Where("deleted_at IS NULL")
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
	if status := c.Query("status"); status != "" {
		q = q.Where("status IN ?", splitList(status))
	}
	if mode := strings.ToUpper(strings.TrimSpace(c.Query("decision_mode"))); mode != "" {
		q = q.Where("decision_mode = ?", mode)
	}
	if provider := c.Query("provider_id"); provider != "" {
		if id, okID := parseUint(provider); okID {
			q = q.Where("provider_id = ?", id)
		}
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("(name LIKE ? OR code LIKE ?)", like, like)
	}
	page, pageSize := pagination(c)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var audits []model.Audit
	if err := q.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&audits).Error; err != nil {
		serverError(c, err)
		return
	}
	s.populateAuditStats(audits)
	ok(c, gin.H{"items": audits, "total": total, "page": page, "page_size": pageSize, "has_more": int64(page*pageSize) < total})
}

func (s *Server) getAudit(c *gin.Context) {
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	list := []model.Audit{*audit}
	s.populateAuditStats(list)
	*audit = list[0]
	var triggers []model.TriggerSource
	_ = s.db.Where("audit_id = ?", audit.ID).Order("priority asc").Find(&triggers).Error
	for i := range triggers {
		triggers[i].SecretSet = triggers[i].SecretEnc != ""
	}
	ok(c, gin.H{"audit": audit, "triggers": triggers})
}

func (s *Server) createAudit(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权创建审核")
		return
	}
	var req auditInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if err := validateAuditInput(&req, true); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.Code == "" {
		badRequest(c, "请填写审核编码")
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
	var provider model.AIProvider
	if err := s.db.Where("id = ? AND project_id = ?", req.ProviderID, req.ProjectID).First(&provider).Error; err != nil {
		badRequest(c, "所选模型供应商不属于当前项目")
		return
	}
	var count int64
	_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND code = ?", req.ProjectID, req.Code).Count(&count).Error
	if count > 0 {
		conflict(c, "CODE_CONFLICT", "当前项目下审核编码已存在（含回收站）")
		return
	}
	storedRules := req.Rules
	if len(storedRules) == 0 || string(storedRules) == "null" {
		switch req.DecisionMode {
		case model.DecisionClassify:
			storedRules = json.RawMessage(catalog.ClassificationRulesPayload(0.60))
		case model.DecisionClassifyOnly:
			storedRules = json.RawMessage("[]")
		default:
			storedRules = json.RawMessage(catalog.RulesPayload(req.Strictness))
		}
	}
	storedRoutes := req.Routes
	if len(storedRoutes) == 0 || string(storedRoutes) == "null" {
		storedRoutes = json.RawMessage(catalog.DefaultRoutes())
	}
	status := req.Status
	if status == "" {
		status = model.AuditActive
	}
	audit := model.Audit{
		ProjectID: req.ProjectID, Name: req.Name, Code: req.Code, Description: req.Description,
		MediaTypes: model.JSONBytes(req.MediaTypes), DecisionMode: req.DecisionMode,
		Categories: model.JSONBytes(req.Categories), ProviderID: provider.ID,
		ReviewPrompt: req.ReviewPrompt, Rules: model.JSONText(storedRules), Routes: model.JSONText(storedRoutes),
		DefaultResult: defaultResult(req.DefaultResult), OnModelFail: defaultResult(req.OnModelFail),
		DedupeWindowSec: dedupeWindow(req.DedupeWindowSec), Status: status, Version: 1, CreatedBy: user.ID,
	}
	if err := applyCallbackConfig(s, &audit, &req, true); err != nil {
		badRequest(c, err.Error())
		return
	}
	if audit.ReviewPrompt == "" {
		audit.ReviewPrompt = catalog.DefaultReviewPrompt
	}
	if err := s.db.Create(&audit).Error; err != nil {
		serverError(c, err)
		return
	}
	_ = createDefaultTriggers(s.db, project, audit)
	s.writeAudit(c, &audit.ProjectID, "AUDIT_CREATE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS",
		map[string]any{"name": audit.Name, "code": audit.Code, "provider_id": provider.ID})
	created(c, s.auditView(audit, provider.Name, provider.Kind))
}

func (s *Server) updateAudit(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权修改审核")
		return
	}
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	var req auditInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		if len([]rune(name)) < 2 || len([]rune(name)) > 64 {
			badRequest(c, "审核名称需为 2~64 个字符")
			return
		}
		audit.Name = name
	}
	if req.Description != "" {
		audit.Description = req.Description
	}
	if req.ReviewPrompt != "" {
		audit.ReviewPrompt = req.ReviewPrompt
	}
	if len(req.MediaTypes) > 0 {
		if err := validateMediaTypes(req.MediaTypes); err != nil {
			badRequest(c, err.Error())
			return
		}
		audit.MediaTypes = model.JSONBytes(req.MediaTypes)
	}
	if req.DecisionMode != "" || req.Categories != nil {
		mode := audit.DecisionMode
		if req.DecisionMode != "" {
			mode = normalizeDecisionMode(req.DecisionMode)
		}
		categories := req.Categories
		if categories == nil {
			_ = model.DecodeJSON(audit.Categories, &categories)
		}
		if err := validateDecisionConfig(mode, categories); err != nil {
			badRequest(c, err.Error())
			return
		}
		audit.DecisionMode = mode
		audit.Categories = model.JSONBytes(categories)
		if len(req.Rules) == 0 && req.Strictness == "" {
			switch mode {
			case model.DecisionClassify:
				audit.Rules = catalog.ClassificationRulesPayload(0.60)
			case model.DecisionClassifyOnly:
				audit.Rules = model.JSONBytes([]any{})
			default:
				audit.Rules = catalog.RulesPayload("standard")
			}
		}
	}
	if req.ProviderID != 0 && req.ProviderID != audit.ProviderID {
		var provider model.AIProvider
		if err := s.db.Where("id = ? AND project_id = ?", req.ProviderID, audit.ProjectID).First(&provider).Error; err != nil {
			badRequest(c, "所选模型供应商不属于当前项目")
			return
		}
		audit.ProviderID = provider.ID
	}
	if len(req.Rules) > 0 && string(req.Rules) != "null" {
		if err := validateRulesJSON(req.Rules); err != nil {
			badRequest(c, err.Error())
			return
		}
		audit.Rules = model.JSONText(req.Rules)
	} else if req.Strictness != "" {
		audit.Rules = catalog.RulesPayload(req.Strictness)
	}
	if len(req.Routes) > 0 && string(req.Routes) != "null" {
		if err := validateRoutesJSON(req.Routes); err != nil {
			badRequest(c, err.Error())
			return
		}
		audit.Routes = model.JSONText(req.Routes)
	}
	if req.DefaultResult != "" {
		if !validResult(req.DefaultResult) {
			badRequest(c, "默认结论需为 PASS/REJECT/REVIEW")
			return
		}
		audit.DefaultResult = req.DefaultResult
	}
	if req.OnModelFail != "" {
		if !validResult(req.OnModelFail) {
			badRequest(c, "模型失败兜底需为 PASS/REJECT/REVIEW")
			return
		}
		audit.OnModelFail = req.OnModelFail
	}
	if req.DedupeWindowSec != nil {
		if *req.DedupeWindowSec < 0 || *req.DedupeWindowSec > 604800 {
			badRequest(c, "去重窗口需在 0~604800 秒之间")
			return
		}
		audit.DedupeWindowSec = *req.DedupeWindowSec
	}
	if oneOf(req.Status, model.AuditActive, model.AuditPaused, model.AuditDraft) {
		audit.Status = req.Status
	}
	if err := applyCallbackConfig(s, audit, &req, false); err != nil {
		badRequest(c, err.Error())
		return
	}
	audit.Version++
	if err := s.db.Save(audit).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &audit.ProjectID, "AUDIT_UPDATE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS",
		map[string]any{"name": audit.Name, "version": audit.Version})
	list := []model.Audit{*audit}
	s.populateAuditStats(list)
	*audit = list[0]
	ok(c, audit)
}

func (s *Server) updateAuditStatus(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权变更审核状态")
		return
	}
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !oneOf(req.Status, model.AuditActive, model.AuditPaused, model.AuditDraft) {
		badRequest(c, "状态需为 ACTIVE / PAUSED / DRAFT")
		return
	}
	if err := s.db.Model(audit).Update("status", req.Status).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &audit.ProjectID, "AUDIT_STATUS", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS", map[string]any{"status": req.Status})
	ok(c, gin.H{"status": req.Status})
}

func (s *Server) deleteAudit(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权删除审核")
		return
	}
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	now := time.Now()
	if err := s.db.Model(&model.Audit{}).Where("id = ?", audit.ID).Updates(map[string]any{
		"status": model.AuditDeleted, "deleted_at": now, "deleted_with_project": false,
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	_ = s.db.Model(&model.TriggerSource{}).Where("audit_id = ?", audit.ID).Update("status", "PAUSED").Error
	s.writeAudit(c, &audit.ProjectID, "AUDIT_DELETE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS", map[string]any{"name": audit.Name})
	ok(c, gin.H{"deleted": true, "expire_at": now.AddDate(0, 0, 30)})
}

func (s *Server) restoreAudit(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权恢复审核")
		return
	}
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "审核 ID 无效")
		return
	}
	var audit model.Audit
	if err := s.db.First(&audit, id).Error; err != nil {
		notFound(c, "审核配置不存在")
		return
	}
	if audit.DeletedAt == nil {
		badRequest(c, "审核未在回收站中")
		return
	}
	if !s.requireProjectAccess(c, audit.ProjectID) {
		return
	}
	var conflictCount int64
	_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND code = ? AND id <> ? AND deleted_at IS NULL", audit.ProjectID, audit.Code, audit.ID).Count(&conflictCount).Error
	if conflictCount > 0 {
		conflict(c, "CODE_CONFLICT", "当前项目下已有同编码审核")
		return
	}
	var providerCount int64
	_ = s.db.Model(&model.AIProvider{}).Where("id = ?", audit.ProviderID).Count(&providerCount).Error
	if providerCount == 0 {
		fail(c, http.StatusUnprocessableEntity, "DEPENDENCY_MISSING", "审核依赖的模型供应商已被删除，请先重新配置模型", nil)
		return
	}
	if err := s.db.Model(&model.Audit{}).Where("id = ?", audit.ID).Updates(map[string]any{
		"status": model.AuditActive, "deleted_at": nil, "deleted_with_project": false,
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	_ = s.db.Model(&model.TriggerSource{}).Where("audit_id = ?", audit.ID).Update("status", "ACTIVE").Error
	s.writeAudit(c, &audit.ProjectID, "AUDIT_RESTORE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS", nil)
	ok(c, gin.H{"restored": true})
}

func (s *Server) cloneAudit(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权复制审核")
		return
	}
	audit, err := s.findAudit(c)
	if err != nil {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请填写新审核名称和编码")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if !auditCodePattern.MatchString(req.Code) {
		badRequest(c, "审核编码格式不合法")
		return
	}
	var count int64
	_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND code = ?", audit.ProjectID, req.Code).Count(&count).Error
	if count > 0 {
		conflict(c, "CODE_CONFLICT", "当前项目下审核编码已存在")
		return
	}
	clone := *audit
	clone.ID = 0
	clone.Name = req.Name
	clone.Code = req.Code
	clone.Status = model.AuditDraft
	clone.Version = 1
	clone.CreatedBy = user.ID
	clone.DeletedAt = nil
	clone.DeletedWithProject = false
	if err := s.db.Create(&clone).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &clone.ProjectID, "AUDIT_CLONE", "AUDIT", fmt.Sprint(clone.ID), "SUCCESS",
		map[string]any{"source_id": audit.ID, "name": clone.Name, "code": clone.Code})
	created(c, clone)
}

func (s *Server) findAudit(c *gin.Context) (*model.Audit, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "审核 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var audit model.Audit
	if err := s.db.First(&audit, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			notFound(c, "审核配置不存在")
		} else {
			serverError(c, err)
		}
		return nil, err
	}
	if !s.requireProjectAccess(c, audit.ProjectID) {
		return nil, fmt.Errorf("audit out of scope")
	}
	return &audit, nil
}

func (s *Server) populateAuditStats(audits []model.Audit) {
	providerCache := map[uint64]model.AIProvider{}
	projectCache := map[uint64]string{}
	for i := range audits {
		a := &audits[i]
		provider, okCache := providerCache[a.ProviderID]
		if !okCache {
			_ = s.db.First(&provider, a.ProviderID).Error
			providerCache[a.ProviderID] = provider
		}
		a.ProviderName = provider.Name
		a.ProviderKind = provider.Kind
		if _, okProject := projectCache[a.ProjectID]; !okProject {
			var p model.Project
			_ = s.db.First(&p, a.ProjectID).Error
			projectCache[a.ProjectID] = p.Name
		}
		a.ProjectName = projectCache[a.ProjectID]
		a.CallbackSecretSet = a.CallbackSecretEnc != ""
		var triggerTypes []string
		_ = s.db.Model(&model.TriggerSource{}).Where("audit_id = ? AND status = ?", a.ID, "ACTIVE").
			Order("priority asc").Pluck("type", &triggerTypes).Error
		a.ActiveTriggers = triggerTypes
		_ = s.db.Model(&model.Task{}).Where("audit_id = ? AND created_at >= ?", a.ID, startOfToday()).Count(&a.TodayTasks).Error
	}
}

func createDefaultTriggers(db *gorm.DB, project model.Project, audit model.Audit) error {
	defaults := []model.TriggerSource{
		{
			ProjectID: audit.ProjectID, AuditID: audit.ID, Type: model.TriggerOpenAPI,
			Name: "开放 API", Priority: 10, Status: "ACTIVE",
			ConfigJSON: model.JSONBytes(map[string]any{"sync_wait_ms": 3000}),
		},
		{
			ProjectID: audit.ProjectID, AuditID: audit.ID, Type: model.TriggerRedis,
			Name: "Redis Stream", Priority: 20, Status: "DISABLED",
			ConfigJSON: model.JSONBytes(map[string]any{
				"stream":     "sdv:" + project.Code + ":audit:" + audit.Code,
				"group":      fmt.Sprintf("sdv-cg-%d", audit.ID),
				"batch_size": 16,
			}),
		},
		{
			ProjectID: audit.ProjectID, AuditID: audit.ID, Type: model.TriggerWebSocket,
			Name: "WebSocket", Priority: 30, Status: "DISABLED",
			ConfigJSON: model.JSONBytes(map[string]any{}),
		},
		{
			ProjectID: audit.ProjectID, AuditID: audit.ID, Type: model.TriggerPolling,
			Name: "HTTP 定时轮询", Priority: 40, Status: "DISABLED",
			ConfigJSON: model.JSONBytes(map[string]any{
				"url": "", "method": "GET", "interval_sec": 60,
				"items_path": "items", "cursor_path": "next_cursor",
				"cursor_param": "cursor", "allow_private": false,
			}),
		},
	}
	for _, trigger := range defaults {
		var count int64
		if err := db.Model(&model.TriggerSource{}).Where("audit_id = ? AND type = ?", audit.ID, trigger.Type).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&trigger).Error; err != nil && !strings.Contains(err.Error(), "Duplicate entry") {
				return err
			}
		}
	}
	return nil
}

func (s *Server) auditView(audit model.Audit, providerName, providerKind string) model.Audit {
	audit.ProviderName = providerName
	audit.ProviderKind = providerKind
	audit.CallbackSecretSet = audit.CallbackSecretEnc != ""
	return audit
}

func applyCallbackConfig(s *Server, audit *model.Audit, req *auditInput, creating bool) error {
	if req.CallbackEnabled != nil {
		audit.CallbackEnabled = *req.CallbackEnabled
	}
	if url := strings.TrimSpace(req.CallbackURL); url != "" {
		audit.CallbackURL = url
	}
	if audit.CallbackEnabled {
		if audit.CallbackURL == "" {
			return fmt.Errorf("启用回调时必须填写回调地址")
		}
		if err := validateCallbackURL(audit.CallbackURL, s.cfg.AllowPrivate); err != nil {
			return err
		}
	}
	if req.CallbackResults != nil {
		results := make([]string, 0, len(req.CallbackResults))
		for _, r := range req.CallbackResults {
			r = strings.ToUpper(strings.TrimSpace(r))
			if r != model.ResultPass && r != model.ResultReject {
				return fmt.Errorf("回调结果类型 %q 不合法（仅支持 PASS/REJECT）", r)
			}
			results = append(results, r)
		}
		audit.CallbackResults = model.JSONBytes(results)
	} else if creating {
		audit.CallbackResults = model.JSONBytes([]string{model.ResultReject})
	}
	if req.CallbackHeaders != nil {
		if len(req.CallbackHeaders) > 20 {
			return fmt.Errorf("回调 Header 最多 20 个")
		}
		for k := range req.CallbackHeaders {
			if strings.TrimSpace(k) == "" {
				return fmt.Errorf("回调 Header 名称不能为空")
			}
		}
		audit.CallbackHeaders = model.JSONBytes(req.CallbackHeaders)
	}
	if req.CallbackIncludeContent != nil {
		audit.CallbackIncludeContent = *req.CallbackIncludeContent
	} else if creating {
		audit.CallbackIncludeContent = true
	}
	if req.CallbackSecret != "" {
		enc, err := s.cipher.Encrypt(req.CallbackSecret)
		if err != nil {
			return err
		}
		audit.CallbackSecretEnc = enc
	}
	var configuredResults []string
	_ = model.DecodeJSON(audit.CallbackResults, &configuredResults)
	if audit.CallbackEnabled && len(configuredResults) == 0 {
		return fmt.Errorf("启用回调时至少选择一个回调结论")
	}
	audit.CallbackSecretSet = audit.CallbackSecretEnc != ""
	return nil
}

func validateAuditInput(req *auditInput, creating bool) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if len([]rune(req.Name)) < 2 || len([]rune(req.Name)) > 64 {
		return fmt.Errorf("审核名称需为 2~64 个字符")
	}
	if req.Code != "" && !auditCodePattern.MatchString(req.Code) {
		return fmt.Errorf("审核编码需以小写字母开头，仅含小写字母、数字、下划线")
	}
	if req.ProjectID == 0 {
		return fmt.Errorf("请选择项目")
	}
	if req.ProviderID == 0 {
		return fmt.Errorf("请选择模型供应商")
	}
	if err := validateMediaTypes(req.MediaTypes); err != nil {
		return err
	}
	req.DecisionMode = normalizeDecisionMode(req.DecisionMode)
	if creating && req.DecisionMode == model.DecisionClassifyOnly {
		req.Categories = catalog.EnsureClassifierUnknown(req.Categories)
	}
	if err := validateDecisionConfig(req.DecisionMode, req.Categories); err != nil {
		return err
	}
	if req.DefaultResult != "" && !validResult(req.DefaultResult) {
		return fmt.Errorf("默认结论需为 PASS/REJECT/REVIEW")
	}
	if req.OnModelFail != "" && !validResult(req.OnModelFail) {
		return fmt.Errorf("模型失败兜底需为 PASS/REJECT/REVIEW")
	}
	if req.DedupeWindowSec != nil && (*req.DedupeWindowSec < 0 || *req.DedupeWindowSec > 604800) {
		return fmt.Errorf("去重窗口需在 0~604800 秒之间")
	}
	if len(req.Rules) > 0 && string(req.Rules) != "null" {
		if err := validateRulesJSON(req.Rules); err != nil {
			return err
		}
	}
	if len(req.Routes) > 0 && string(req.Routes) != "null" {
		if err := validateRoutesJSON(req.Routes); err != nil {
			return err
		}
	}
	if req.Status != "" && !oneOf(req.Status, model.AuditActive, model.AuditPaused, model.AuditDraft) {
		return fmt.Errorf("状态取值不合法")
	}
	return nil
}

func validateMediaTypes(types []string) error {
	if len(types) == 0 {
		return fmt.Errorf("至少选择一种媒体类型")
	}
	seen := map[string]bool{}
	for _, t := range types {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "text" && t != "image" {
			return fmt.Errorf("媒体类型 %q 不合法；当前版本仅支持 text 和 image（不支持视频/音频）", t)
		}
		if seen[t] {
			return fmt.Errorf("媒体类型重复: %s", t)
		}
		seen[t] = true
	}
	return nil
}

func normalizeDecisionMode(mode string) string {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == model.DecisionClassify || mode == model.DecisionClassifyOnly {
		return mode
	}
	return model.DecisionReview
}

func validateDecisionConfig(mode string, categories []model.AuditCategory) error {
	mode = normalizeDecisionMode(mode)
	if mode == model.DecisionReview {
		if len(categories) == 0 {
			return nil
		}
	}
	if mode == model.DecisionClassify && len(categories) < 2 {
		return fmt.Errorf("分类审核至少需要 2 个分类")
	}
	if mode == model.DecisionClassifyOnly && len(categories) < 1 {
		return fmt.Errorf("纯分类器至少需要 1 个分类类型")
	}
	maxCategories := 20
	if mode == model.DecisionClassifyOnly {
		maxCategories = 200
	}
	if len(categories) > maxCategories {
		return fmt.Errorf("分类数量不能超过 %d 个", maxCategories)
	}
	seen := map[string]bool{}
	for i := range categories {
		category := &categories[i]
		category.Code = strings.ToLower(strings.TrimSpace(category.Code))
		category.Name = strings.TrimSpace(category.Name)
		category.Description = strings.TrimSpace(category.Description)
		category.Result = strings.ToUpper(strings.TrimSpace(category.Result))
		if !categoryCodePattern.MatchString(category.Code) {
			return fmt.Errorf("第 %d 个分类编码需以小写字母开头，仅含小写字母、数字、下划线", i+1)
		}
		if seen[category.Code] {
			return fmt.Errorf("分类编码 %q 重复", category.Code)
		}
		seen[category.Code] = true
		if len([]rune(category.Name)) < 1 || len([]rune(category.Name)) > 32 {
			return fmt.Errorf("分类 %q 的名称需为 1~32 个字符", category.Code)
		}
		if len([]rune(category.Description)) > 255 {
			return fmt.Errorf("分类 %q 的说明不能超过 255 个字符", category.Code)
		}
		if mode == model.DecisionClassifyOnly {
			category.Result = model.ResultPass
		} else if !validResult(category.Result) {
			return fmt.Errorf("分类 %q 的映射结论必须为 PASS/REJECT/REVIEW", category.Code)
		}
	}
	return nil
}

func validateRulesJSON(raw []byte) error {
	var groups []struct {
		Name        string `json:"name"`
		OnHitResult string `json:"on_hit_result"`
		Items       []any  `json:"items"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return fmt.Errorf("规则必须是 JSON 数组")
	}
	for _, g := range groups {
		if !validResult(g.OnHitResult) {
			return fmt.Errorf("规则 %q 的 on_hit_result 必须为 PASS/REJECT/REVIEW", g.Name)
		}
	}
	return nil
}

func validResult(v string) bool {
	v = strings.ToUpper(strings.TrimSpace(v))
	return v == model.ResultPass || v == model.ResultReject || v == model.ResultReview
}

type routeActionInput struct {
	Type      string            `json:"type"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Params    map[string]string `json:"params"`
	Body      string            `json:"body"`
	TimeoutMS int               `json:"timeout_ms"`
	Command   string            `json:"command"`
	Target    string            `json:"target"`
	Payload   string            `json:"payload"`
	DSNEnv    string            `json:"dsn_env"`
	Query     string            `json:"query"`
	Args      []any             `json:"args"`
}

type resultRouteInput struct {
	ExecMode string             `json:"exec_mode"`
	OnError  string             `json:"on_error"`
	Actions  []routeActionInput `json:"actions"`
}

func validateRoutesJSON(raw []byte) error {
	var routes map[string]resultRouteInput
	if err := json.Unmarshal(raw, &routes); err != nil {
		return fmt.Errorf("结果动作路由必须是 JSON 对象")
	}
	for result, route := range routes {
		if !validResult(result) {
			return fmt.Errorf("结果动作路由的键 %q 不合法，仅支持 PASS/REJECT/REVIEW", result)
		}
		if mode := strings.ToLower(strings.TrimSpace(route.ExecMode)); mode != "" && mode != "sequential" {
			return fmt.Errorf("%s 的 exec_mode 仅支持 sequential", result)
		}
		if onError := strings.ToLower(strings.TrimSpace(route.OnError)); onError != "" && onError != "continue" && onError != "stop" && onError != "to_review" {
			return fmt.Errorf("%s 的 on_error 仅支持 continue/stop/to_review", result)
		}
		for index, action := range route.Actions {
			label := fmt.Sprintf("%s 的第 %d 个动作", result, index+1)
			if action.TimeoutMS < 0 || action.TimeoutMS > 30000 {
				return fmt.Errorf("%s timeout_ms 需在 0~30000 之间", label)
			}
			switch strings.ToLower(strings.TrimSpace(action.Type)) {
			case "http":
				if strings.TrimSpace(action.URL) == "" {
					return fmt.Errorf("%s缺少 url", label)
				}
				if method := strings.ToUpper(strings.TrimSpace(action.Method)); method != "" &&
					method != http.MethodGet && method != http.MethodPost && method != http.MethodPut &&
					method != http.MethodPatch && method != http.MethodDelete && method != http.MethodHead {
					return fmt.Errorf("%s的 method 不合法", label)
				}
			case "websocket":
				if strings.TrimSpace(action.URL) == "" {
					return fmt.Errorf("%s缺少 url", label)
				}
				scheme := strings.ToLower(strings.TrimSpace(action.URL))
				if !strings.HasPrefix(scheme, "ws://") && !strings.HasPrefix(scheme, "wss://") &&
					!strings.HasPrefix(scheme, "${") {
					return fmt.Errorf("%s的 url 必须以 ws:// 或 wss:// 开头", label)
				}
			case "redis":
				if strings.TrimSpace(action.Target) == "" {
					return fmt.Errorf("%s缺少 target", label)
				}
				switch strings.ToLower(strings.TrimSpace(action.Command)) {
				case "", "publish", "set", "xadd":
				default:
					return fmt.Errorf("%s的 command 仅支持 publish/set/xadd", label)
				}
			case "mysql":
				if strings.TrimSpace(action.DSNEnv) == "" {
					return fmt.Errorf("%s缺少 dsn_env", label)
				}
				if strings.TrimSpace(action.Query) == "" {
					return fmt.Errorf("%s缺少 query", label)
				}
			default:
				return fmt.Errorf("%s的类型 %q 不合法，仅支持 http/websocket/redis/mysql", label, action.Type)
			}
		}
	}
	return nil
}

func validateCallbackURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("回调地址格式错误")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	case "ws", "wss":
		if u.Host == "" {
			return fmt.Errorf("回调地址缺少主机名")
		}
		if allowPrivate {
			return nil
		}
		u.Scheme = map[string]string{"ws": "http", "wss": "https"}[strings.ToLower(u.Scheme)]
	default:
		return fmt.Errorf("回调地址必须以 http://、https://、ws:// 或 wss:// 开头")
	}
	if u.Host == "" {
		return fmt.Errorf("回调地址缺少主机名")
	}
	if !allowPrivate {
		if err := media.ValidateOutboundURL(u.String()); err != nil {
			return fmt.Errorf("回调地址未通过 SSRF 校验: %w", err)
		}
	}
	return nil
}

func defaultResult(v string) string {
	if validResult(v) {
		return strings.ToUpper(strings.TrimSpace(v))
	}
	return model.ResultReview
}

func dedupeWindow(v *int) int {
	if v == nil {
		return 86400
	}
	return *v
}
