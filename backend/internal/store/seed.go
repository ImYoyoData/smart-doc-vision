package store

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

type Seeder struct {
	DB     *gorm.DB
	Cipher *security.Cipher
	Cfg    config.Config
}

func (s *Seeder) Run() error {
	if err := s.ensureRoles(); err != nil {
		return err
	}
	admin, err := s.ensureAdmin()
	if err != nil {
		return err
	}
	project, err := s.ensureDemoProject(admin.ID)
	if err != nil {
		return err
	}
	provider, err := s.ensureGenericProvider(project.ID)
	if err != nil {
		return err
	}
	if err := s.ensureJEVProvider(project.ID); err != nil {
		return err
	}
	if err := s.ensureDemoAudit(project.ID, provider, admin.ID); err != nil {
		return err
	}
	if err := s.ensureClassificationAudit(project.ID, admin.ID); err != nil {
		return err
	}
	if err := s.ensureClassifierAudit(project.ID, admin.ID); err != nil {
		return err
	}
	if err := s.ensureDefaultAPICredentials(); err != nil {
		return err
	}
	return s.ensureAuditTriggers()
}

func (s *Seeder) ensureRoles() error {
	for _, role := range catalog.Roles() {
		var existing model.Role
		err := s.DB.Where("code = ?", role.Code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			record := model.Role{
				Code: role.Code, Name: role.Name, Description: role.Description,
				Permissions: model.JSONBytes(role.Permissions), Menus: model.JSONBytes(role.Menus),
				Builtin: true,
			}
			if err := s.DB.Create(&record).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		// Built-in role definitions follow the application catalog on upgrade.
		if err := s.DB.Model(&model.Role{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"name": role.Name, "description": role.Description,
			"permissions": model.JSONBytes(role.Permissions), "menus": model.JSONBytes(role.Menus),
			"builtin": true,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) ensureDefaultAPICredentials() error {
	var projects []model.Project
	if err := s.DB.Find(&projects).Error; err != nil {
		return err
	}
	for _, project := range projects {
		if project.AppKey == "" || project.AppSecretEnc == "" {
			continue
		}
		var count int64
		if err := s.DB.Model(&model.APICredential{}).Where("app_key = ?", project.AppKey).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		var auditIDs []uint64
		_ = s.DB.Model(&model.Audit{}).
			Where("project_id = ? AND deleted_at IS NULL", project.ID).
			Pluck("id", &auditIDs).Error
		credential := model.APICredential{
			ProjectID: project.ID, Name: "默认接入凭证", AppKey: project.AppKey,
			SecretEnc: project.AppSecretEnc, AuditIDs: model.JSONBytes(auditIDs),
			Status: model.CredentialActive, CreatedBy: project.OwnerID,
		}
		if err := s.DB.Create(&credential).Error; err != nil {
			return err
		}
		_ = s.DB.Model(&model.Project{}).Where("id = ?", project.ID).
			Updates(map[string]any{"app_key": "", "app_secret_enc": ""}).Error
		log.Printf("migrated project app key into credential: %s", credential.AppKey)
	}
	return nil
}

func (s *Seeder) ensureAdmin() (*model.User, error) {
	var user model.User
	err := s.DB.Where("username = ?", s.Cfg.AdminUsername).First(&user).Error
	if err == nil {
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	hash, err := security.HashPassword(s.Cfg.AdminPassword)
	if err != nil {
		return nil, err
	}
	user = model.User{
		Username:     s.Cfg.AdminUsername,
		PasswordHash: hash,
		DisplayName:  "系统管理员",
		Role:         model.RoleSuperAdmin,
		Status:       "ACTIVE",
	}
	if err := s.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	log.Printf("seeded admin user: %s", user.Username)
	return &user, nil
}

func (s *Seeder) ensureDemoProject(ownerID uint64) (*model.Project, error) {
	var project model.Project
	err := s.DB.Where("code = ?", "smartdoc-demo").First(&project).Error
	if err == nil {
		return &project, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	project = model.Project{
		Name:          "智审图档 · 演示空间",
		Code:          "smartdoc-demo",
		Env:           "dev",
		Status:        model.ProjectActive,
		Description:   "SmartDoc Vision 初始演示项目，可直接体验模型审核闭环。",
		OwnerID:       ownerID,
		RetentionDays: 30,
		CreatedBy:     ownerID,
	}
	if err := s.DB.Create(&project).Error; err != nil {
		return nil, err
	}
	log.Printf("seeded demo project: %s", project.Code)
	return &project, nil
}

func (s *Seeder) ensureGenericProvider(projectID uint64) (*model.AIProvider, error) {
	var provider model.AIProvider
	err := s.DB.Where("project_id = ? AND name = ?", projectID, "通用大模型").First(&provider).Error
	if err == nil {
		return &provider, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	keyEnc, err := s.Cipher.Encrypt(s.Cfg.DefaultAIAPIKey)
	if err != nil {
		return nil, err
	}
	status := model.ProviderActive
	if strings.TrimSpace(s.Cfg.DefaultAIAPIKey) == "" {
		status = model.ProviderDisabled
	}
	provider = model.AIProvider{
		ProjectID:  projectID,
		Name:       "通用大模型",
		Kind:       model.ProviderOpenAI,
		BaseURL:    s.Cfg.DefaultAIBaseURL,
		Model:      s.Cfg.DefaultAIModel,
		APIKeyEnc:  keyEnc,
		Prompt:     "",
		ConfigJSON: catalog.DefaultProviderConfig(model.ProviderOpenAI, s.Cfg.DefaultAISupportsVision),
		Status:     status,
	}
	if err := s.DB.Create(&provider).Error; err != nil {
		return nil, err
	}
	log.Printf("seeded AI provider: %s (%s)", provider.Name, provider.Model)
	return &provider, nil
}

func (s *Seeder) ensureJEVProvider(projectID uint64) error {
	var count int64
	if err := s.DB.Model(&model.AIProvider{}).
		Where("project_id = ? AND kind = ?", projectID, model.ProviderJEV).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	provider := model.AIProvider{
		ProjectID:  projectID,
		Name:       "JEV 决策模型",
		Kind:       model.ProviderJEV,
		BaseURL:    s.Cfg.JEVBaseURL,
		Model:      s.Cfg.JEVModel,
		Prompt:     "使用 TypeSafe Jev System One 进行结构化裁决，输出 choice/noul 概率，不生成自由文本。",
		ConfigJSON: catalog.DefaultProviderConfig(model.ProviderJEV, true),
		Status:     model.ProviderDisabled,
	}
	if err := s.DB.Create(&provider).Error; err != nil {
		return err
	}
	log.Printf("seeded JEV provider placeholder: %s", provider.Name)
	return nil
}

func (s *Seeder) ensureDemoAudit(projectID uint64, provider *model.AIProvider, adminID uint64) error {
	var count int64
	if err := s.DB.Model(&model.Audit{}).
		Where("project_id = ? AND code = ?", projectID, "archive_review").
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	audit := model.Audit{
		ProjectID:       projectID,
		Name:            "档案图档智能审核",
		Code:            "archive_review",
		Description:     "对档案文本和扫描件进行内容安全、完整性与敏感信息审核，灰区自动转人工复核。",
		MediaTypes:      catalog.MediaTypesJSON([]string{"text", "image"}),
		ProviderID:      provider.ID,
		ReviewPrompt:    catalog.DefaultReviewPrompt,
		Rules:           catalog.RulesPayload("standard"),
		Routes:          catalog.DefaultRoutes(),
		DefaultResult:   model.ResultReview,
		OnModelFail:     model.ResultReview,
		DedupeWindowSec: 86400,
		Status:          model.AuditActive,
		Version:         1,
		CreatedBy:       adminID,
	}
	if err := s.DB.Create(&audit).Error; err != nil {
		return err
	}
	trigger := model.TriggerSource{
		ProjectID:  projectID,
		AuditID:    audit.ID,
		Type:       model.TriggerOpenAPI,
		Name:       "后台/开放 API 提交",
		ConfigJSON: model.JSONBytes(map[string]any{"enabled": true}),
		Priority:   40,
		Status:     "ACTIVE",
	}
	if err := s.DB.Create(&trigger).Error; err != nil {
		return err
	}
	log.Printf("seeded demo audit: %s", audit.Code)
	return nil
}

func (s *Seeder) ensureClassificationAudit(projectID uint64, adminID uint64) error {
	var count int64
	if err := s.DB.Model(&model.Audit{}).
		Where("project_id = ? AND code = ?", projectID, "archive_classify").
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var jevProvider model.AIProvider
	if err := s.DB.Where("project_id = ? AND kind = ?", projectID, model.ProviderJEV).
		Order("id asc").First(&jevProvider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("skip classification demo audit: no JEV provider configured")
			return nil
		}
		return err
	}
	audit := model.Audit{
		ProjectID:       projectID,
		Name:            "档案类型分类审核（JEV 示例）",
		Code:            "archive_classify",
		Description:     "使用 JEV choice 决策做档案分类，并按分类映射 PASS / REJECT / REVIEW；低置信度自动转人工。",
		MediaTypes:      catalog.MediaTypesJSON([]string{"text", "image"}),
		DecisionMode:    model.DecisionClassify,
		Categories:      model.JSONBytes(catalog.ExampleClassificationCategories()),
		ProviderID:      jevProvider.ID,
		ReviewPrompt:    catalog.DefaultClassifyPrompt,
		Rules:           catalog.ClassificationRulesPayload(0.60),
		Routes:          catalog.DefaultRoutes(),
		DefaultResult:   model.ResultReview,
		OnModelFail:     model.ResultReview,
		DedupeWindowSec: 86400,
		Status:          model.AuditActive,
		Version:         1,
		CreatedBy:       adminID,
	}
	if err := s.DB.Create(&audit).Error; err != nil {
		return err
	}
	log.Printf("seeded classification demo audit: %s", audit.Code)
	return nil
}

func (s *Seeder) ensureClassifierAudit(projectID uint64, adminID uint64) error {
	var count int64
	if err := s.DB.Model(&model.Audit{}).
		Where("project_id = ? AND code = ?", projectID, "archive_classifier").
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var provider model.AIProvider
	if err := s.DB.Where("project_id = ? AND status = ?", projectID, model.ProviderActive).
		Order("CASE WHEN kind = 'JEV' THEN 0 ELSE 1 END, id asc").First(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("skip pure classifier demo audit: no active provider configured")
			return nil
		}
		return err
	}
	categories := catalog.ExampleClassificationCategories()
	for i := range categories {
		categories[i].Result = model.ResultPass
	}
	categories = catalog.EnsureClassifierUnknown(categories)
	audit := model.Audit{
		ProjectID:       projectID,
		Name:            "档案类型分类器（纯分类示例）",
		Code:            "archive_classifier",
		Description:     "只返回档案类型，不做驳回判定；成功审核结论固定 PASS，类型写入 category / category_name。",
		MediaTypes:      catalog.MediaTypesJSON([]string{"text", "image"}),
		DecisionMode:    model.DecisionClassifyOnly,
		Categories:      model.JSONBytes(categories),
		ProviderID:      provider.ID,
		ReviewPrompt:    catalog.DefaultClassifyPrompt + "\n\n这是纯分类器任务：只返回最匹配的分类类型和置信度，不需要判定通过或驳回，平台会把成功分类的结论固定为 PASS。",
		Rules:           model.JSONBytes([]any{}),
		Routes:          catalog.DefaultRoutes(),
		DefaultResult:   model.ResultPass,
		OnModelFail:     model.ResultReview,
		DedupeWindowSec: 86400,
		Status:          model.AuditActive,
		Version:         1,
		CreatedBy:       adminID,
	}
	if err := s.DB.Create(&audit).Error; err != nil {
		return err
	}
	log.Printf("seeded pure classifier demo audit: %s", audit.Code)
	return nil
}

func (s *Seeder) ensureAuditTriggers() error {
	var audits []model.Audit
	if err := s.DB.Where("deleted_at IS NULL").Find(&audits).Error; err != nil {
		return err
	}
	var projects []model.Project
	if err := s.DB.Find(&projects).Error; err != nil {
		return err
	}
	projectMap := map[uint64]model.Project{}
	for _, p := range projects {
		projectMap[p.ID] = p
	}
	for _, audit := range audits {
		project, ok := projectMap[audit.ProjectID]
		if !ok {
			continue
		}
		defaults := []struct {
			Type   string
			Name   string
			Status string
			Config any
		}{
			{model.TriggerOpenAPI, "开放 API", "ACTIVE", map[string]any{"sync_wait_ms": 3000}},
			{model.TriggerRedis, "Redis Stream", "DISABLED", map[string]any{
				"stream": "sdv:" + project.Code + ":audit:" + audit.Code,
				"group":  "sdv-cg-" + fmt.Sprint(audit.ID), "batch_size": 16,
			}},
			{model.TriggerWebSocket, "WebSocket", "DISABLED", map[string]any{}},
			{model.TriggerPolling, "HTTP 定时轮询", "DISABLED", map[string]any{
				"url": "", "method": "GET", "interval_sec": 60,
				"items_path": "items", "cursor_path": "next_cursor", "cursor_param": "cursor",
				"allow_private": false,
			}},
		}
		for _, item := range defaults {
			var count int64
			_ = s.DB.Model(&model.TriggerSource{}).Where("audit_id = ? AND type = ?", audit.ID, item.Type).Count(&count).Error
			if count > 0 {
				continue
			}
			ts := model.TriggerSource{
				ProjectID: project.ID, AuditID: audit.ID, Type: item.Type, Name: item.Name,
				Status: item.Status, ConfigJSON: model.JSONBytes(item.Config), Priority: 10,
			}
			_ = s.DB.Create(&ts).Error
		}
	}
	return nil
}
