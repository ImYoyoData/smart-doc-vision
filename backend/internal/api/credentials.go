package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

type credentialInput struct {
	Name     string   `json:"name"`
	AuditIDs []uint64 `json:"audit_ids"`
	Status   string   `json:"status"`
}

type credentialScopeView struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status string `json:"status"`
	Exists bool   `json:"exists"`
}

type credentialRow struct {
	ID              uint64                `json:"id"`
	ProjectID       uint64                `json:"project_id"`
	Name            string                `json:"name"`
	AppKey          string                `json:"app_key"`
	AppSecretMasked string                `json:"app_secret_masked"`
	SecretSet       bool                  `json:"secret_set"`
	Status          string                `json:"status"`
	AuditIDs        []uint64              `json:"audit_ids"`
	Scopes          []credentialScopeView `json:"scopes"`
	LastUsedAt      *time.Time            `json:"last_used_at"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

func (s *Server) listCredentials(c *gin.Context) {
	if !canAdmin(currentUser(c).Role) {
		forbidden(c, "当前角色无权查看项目凭证")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	var rows []model.APICredential
	if err := s.db.Where("project_id = ?", project.ID).Order("id asc").Find(&rows).Error; err != nil {
		serverError(c, err)
		return
	}
	views := make([]credentialRow, 0, len(rows))
	for _, row := range rows {
		views = append(views, s.credentialRow(row))
	}
	ok(c, gin.H{"items": views, "total": len(views)})
}

func (s *Server) createCredential(c *gin.Context) {
	user := currentUser(c)
	if !canAdmin(user.Role) {
		forbidden(c, "当前角色无权创建项目凭证")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	var req credentialInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if err := s.validateCredentialInput(project.ID, &req, true); err != nil {
		badRequest(c, err.Error())
		return
	}
	appKey, secret, err := s.generateCredentialSecrets()
	if err != nil {
		serverError(c, err)
		return
	}
	enc, err := s.cipher.Encrypt(secret)
	if err != nil {
		serverError(c, err)
		return
	}
	status := req.Status
	if status == "" {
		status = model.CredentialActive
	}
	row := model.APICredential{
		ProjectID: project.ID, Name: req.Name, AppKey: appKey, SecretEnc: enc,
		AuditIDs: model.JSONBytes(req.AuditIDs), Status: status, CreatedBy: user.ID,
	}
	if err := s.db.Create(&row).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "CREDENTIAL_CREATE", "CREDENTIAL", fmt.Sprint(row.ID), "SUCCESS",
		map[string]any{"name": row.Name, "audit_ids": req.AuditIDs})
	view := s.credentialRow(row)
	ok(c, gin.H{"credential": view, "app_secret": secret, "warning": "AppSecret 仅显示一次，请立即保存。"})
}

func (s *Server) updateCredential(c *gin.Context) {
	user := currentUser(c)
	if !canAdmin(user.Role) {
		forbidden(c, "当前角色无权修改项目凭证")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	credential, err := s.findCredential(project.ID, c.Param("credentialId"))
	if err != nil {
		return
	}
	var req credentialInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if err := s.validateCredentialInput(project.ID, &req, false); err != nil {
		badRequest(c, err.Error())
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		credential.Name = name
	}
	if req.AuditIDs != nil {
		credential.AuditIDs = model.JSONBytes(req.AuditIDs)
	}
	if oneOf(req.Status, model.CredentialActive, model.CredentialDisabled) {
		credential.Status = req.Status
	}
	if err := s.db.Save(credential).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "CREDENTIAL_UPDATE", "CREDENTIAL", fmt.Sprint(credential.ID), "SUCCESS",
		map[string]any{"name": credential.Name, "status": credential.Status})
	ok(c, s.credentialRow(*credential))
}

func (s *Server) rotateCredential(c *gin.Context) {
	user := currentUser(c)
	if !canAdmin(user.Role) {
		forbidden(c, "当前角色无权轮换项目凭证")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	credential, err := s.findCredential(project.ID, c.Param("credentialId"))
	if err != nil {
		return
	}
	secret, err := security.RandomSecret(32)
	if err != nil {
		serverError(c, err)
		return
	}
	enc, err := s.cipher.Encrypt(secret)
	if err != nil {
		serverError(c, err)
		return
	}
	if err := s.db.Model(credential).Update("secret_enc", enc).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "CREDENTIAL_ROTATE", "CREDENTIAL", fmt.Sprint(credential.ID), "SUCCESS", nil)
	ok(c, gin.H{"id": credential.ID, "app_key": credential.AppKey, "app_secret": secret, "warning": "新 AppSecret 仅显示一次，旧密钥立即失效。"})
}

func (s *Server) deleteCredential(c *gin.Context) {
	user := currentUser(c)
	if !canAdmin(user.Role) {
		forbidden(c, "当前角色无权删除项目凭证")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	credential, err := s.findCredential(project.ID, c.Param("credentialId"))
	if err != nil {
		return
	}
	if err := s.db.Delete(credential).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "CREDENTIAL_DELETE", "CREDENTIAL", fmt.Sprint(credential.ID), "SUCCESS",
		map[string]any{"name": credential.Name})
	ok(c, gin.H{"deleted": true})
}

func (s *Server) findCredential(projectID uint64, rawID string) (*model.APICredential, error) {
	id, okID := parseUint(rawID)
	if !okID {
		return nil, fmt.Errorf("credential id invalid")
	}
	var credential model.APICredential
	if err := s.db.Where("id = ? AND project_id = ?", id, projectID).First(&credential).Error; err != nil {
		return nil, err
	}
	return &credential, nil
}

func (s *Server) validateCredentialInput(projectID uint64, req *credentialInput, creating bool) error {
	req.Name = strings.TrimSpace(req.Name)
	if creating && req.Name == "" {
		return fmt.Errorf("请填写凭证名称")
	}
	if req.Name != "" && len([]rune(req.Name)) > 64 {
		return fmt.Errorf("凭证名称不能超过 64 个字符")
	}
	if creating && len(req.AuditIDs) == 0 {
		return fmt.Errorf("请至少选择一个允许访问的审核配置")
	}
	if req.Status != "" && !oneOf(req.Status, model.CredentialActive, model.CredentialDisabled) {
		return fmt.Errorf("凭证状态不合法")
	}
	if req.AuditIDs != nil {
		for _, id := range req.AuditIDs {
			var count int64
			if err := s.db.Model(&model.Audit{}).Where("id = ? AND project_id = ? AND deleted_at IS NULL", id, projectID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("审核配置 #%d 不存在或已删除", id)
			}
		}
	}
	return nil
}

func (s *Server) credentialRow(row model.APICredential) credentialRow {
	secret, _ := s.cipher.Decrypt(row.SecretEnc)
	var ids []uint64
	_ = model.DecodeJSON(row.AuditIDs, &ids)
	scopes := make([]credentialScopeView, 0, len(ids))
	for _, id := range ids {
		var audit model.Audit
		if err := s.db.First(&audit, id).Error; err != nil {
			scopes = append(scopes, credentialScopeView{ID: id, Exists: false})
			continue
		}
		scopes = append(scopes, credentialScopeView{
			ID:   audit.ID,
			Name: audit.Name, Code: audit.Code, Status: audit.Status,
			Exists: audit.DeletedAt == nil,
		})
	}
	return credentialRow{
		ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, AppKey: row.AppKey,
		AppSecretMasked: security.MaskSecret(secret), SecretSet: secret != "",
		Status: row.Status, AuditIDs: ids, Scopes: scopes, LastUsedAt: row.LastUsedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (s *Server) generateCredentialSecrets() (string, string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		raw, err := security.RandomHex(12)
		if err != nil {
			return "", "", err
		}
		appKey := "sdv_" + raw
		var count int64
		if err := s.db.Model(&model.APICredential{}).Where("app_key = ?", appKey).Count(&count).Error; err != nil {
			return "", "", err
		}
		if count > 0 {
			continue
		}
		secret, err := security.RandomSecret(32)
		if err != nil {
			return "", "", err
		}
		return appKey, secret, nil
	}
	return "", "", fmt.Errorf("生成 AppKey 失败，请重试")
}

func credentialAuditIDs(credential model.APICredential) []uint64 {
	var ids []uint64
	_ = model.DecodeJSON(credential.AuditIDs, &ids)
	return ids
}

func credentialAllowsAudit(credential model.APICredential, auditID uint64) bool {
	for _, id := range credentialAuditIDs(credential) {
		if id == auditID {
			return true
		}
	}
	return false
}
