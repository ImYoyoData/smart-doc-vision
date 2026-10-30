package api

import (
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{2,31}$`)

type userInput struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       *string   `json:"email"`
	Password    string    `json:"password"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	ProjectIDs  *[]uint64 `json:"project_ids"`
	AuditIDs    *[]uint64 `json:"audit_ids"`
}

func normalizeEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", nil
	}
	if len(email) > 128 {
		return "", fmt.Errorf("邮箱地址不能超过 128 个字符")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", fmt.Errorf("邮箱地址格式不正确")
	}
	return email, nil
}

func (s *Server) listUsers(c *gin.Context) {
	if currentUser(c).Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可管理后台账号")
		return
	}
	var users []model.User
	if err := s.db.Order("id asc").Find(&users).Error; err != nil {
		serverError(c, err)
		return
	}
	items := make([]gin.H, 0, len(users))
	for i := range users {
		s.hydrateUser(&users[i])
		items = append(items, userView(users[i]))
	}
	ok(c, gin.H{"items": items, "total": len(items), "roles": s.roleDefinitionsList()})
}

func (s *Server) createUser(c *gin.Context) {
	operator := currentUser(c)
	if operator.Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可创建后台账号")
		return
	}
	var req userInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Role = strings.ToUpper(strings.TrimSpace(req.Role))
	email := ""
	if req.Email != nil {
		normalized, err := normalizeEmail(*req.Email)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		email = normalized
	}
	if !usernamePattern.MatchString(req.Username) {
		badRequest(c, "账号需以字母开头，3~32 位，仅支持字母、数字、点、下划线和连字符")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	if len([]rune(req.DisplayName)) > 64 {
		badRequest(c, "显示名称不能超过 64 个字符")
		return
	}
	if err := validatePassword(req.Password); err != nil {
		badRequest(c, err.Error())
		return
	}
	if !s.validRole(req.Role) {
		badRequest(c, "角色不合法")
		return
	}
	var projectIDs []uint64
	if req.ProjectIDs != nil {
		projectIDs = *req.ProjectIDs
	}
	if req.Role != model.RoleSuperAdmin && len(projectIDs) == 0 {
		badRequest(c, "非超级管理员至少需要授权一个项目")
		return
	}
	if err := s.validateProjectIDs(projectIDs); err != nil {
		badRequest(c, err.Error())
		return
	}
	auditIDs := []uint64(nil)
	if req.Role != model.RoleSuperAdmin && req.AuditIDs != nil {
		auditIDs = *req.AuditIDs
	}
	if err := s.validateAuditIDs(auditIDs, projectIDs); err != nil {
		badRequest(c, err.Error())
		return
	}
	var count int64
	_ = s.db.Model(&model.User{}).Where("username = ?", req.Username).Count(&count).Error
	if count > 0 {
		conflict(c, "USERNAME_CONFLICT", "该账号已存在")
		return
	}
	hash, err := security.HashPassword(req.Password)
	if err != nil {
		serverError(c, err)
		return
	}
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	if !oneOf(status, "ACTIVE", "DISABLED") {
		badRequest(c, "账号状态不合法")
		return
	}
	user := model.User{
		Username: req.Username, PasswordHash: hash, DisplayName: req.DisplayName,
		Email: email, Role: req.Role, Status: status,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := syncUserProjects(tx, user.ID, projectIDs); err != nil {
			return err
		}
		return syncUserAuditScopes(tx, user.ID, auditIDs)
	})
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "USERNAME_CONFLICT", "该账号已存在")
			return
		}
		serverError(c, err)
		return
	}
	s.hydrateUser(&user)
	s.writeAudit(c, nil, "USER_CREATE", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"username": user.Username, "role": user.Role, "project_ids": projectIDs, "audit_ids": auditIDs})
	created(c, userView(user))
}

func (s *Server) updateUser(c *gin.Context) {
	operator := currentUser(c)
	if operator.Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可管理后台账号")
		return
	}
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "用户 ID 无效")
		return
	}
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	var req userInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	newRole := user.Role
	if strings.TrimSpace(req.Role) != "" {
		newRole = strings.ToUpper(strings.TrimSpace(req.Role))
		if !s.validRole(newRole) {
			badRequest(c, "角色不合法")
			return
		}
	}
	newStatus := user.Status
	if strings.TrimSpace(req.Status) != "" {
		newStatus = strings.ToUpper(strings.TrimSpace(req.Status))
		if !oneOf(newStatus, "ACTIVE", "DISABLED", "LOCKED") {
			badRequest(c, "账号状态不合法")
			return
		}
	}
	if user.ID == operator.ID && newStatus != "ACTIVE" {
		badRequest(c, "不能停用当前登录账号")
		return
	}
	if user.Role == model.RoleSuperAdmin && (newRole != model.RoleSuperAdmin || newStatus != "ACTIVE") {
		var superCount int64
		_ = s.db.Model(&model.User{}).Where("role = ? AND status = ?", model.RoleSuperAdmin, "ACTIVE").Count(&superCount).Error
		if superCount <= 1 {
			badRequest(c, "系统至少需要保留一个启用中的超级管理员")
			return
		}
	}
	var projectIDs []uint64
	if req.ProjectIDs != nil {
		projectIDs = *req.ProjectIDs
		if err := s.validateProjectIDs(projectIDs); err != nil {
			badRequest(c, err.Error())
			return
		}
		if newRole != model.RoleSuperAdmin && len(projectIDs) == 0 {
			badRequest(c, "非超级管理员至少需要授权一个项目")
			return
		}
	} else {
		_ = s.db.Model(&model.ProjectMember{}).Where("user_id = ?", user.ID).Pluck("project_id", &projectIDs).Error
	}
	auditIDs := []uint64(nil)
	if newRole != model.RoleSuperAdmin {
		if req.AuditIDs != nil {
			auditIDs = *req.AuditIDs
		} else {
			_ = s.db.Model(&model.UserAuditScope{}).Where("user_id = ?", user.ID).
				Order("audit_id asc").Pluck("audit_id", &auditIDs).Error
		}
	}
	if err := s.validateAuditIDs(auditIDs, projectIDs); err != nil {
		badRequest(c, err.Error())
		return
	}
	if name := strings.TrimSpace(req.DisplayName); name != "" {
		if len([]rune(name)) > 64 {
			badRequest(c, "显示名称不能超过 64 个字符")
			return
		}
		user.DisplayName = name
	}
	if req.Email != nil {
		email, err := normalizeEmail(*req.Email)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		user.Email = email
	}
	if req.Password != "" {
		if err := validatePassword(req.Password); err != nil {
			badRequest(c, err.Error())
			return
		}
		hash, err := security.HashPassword(req.Password)
		if err != nil {
			serverError(c, err)
			return
		}
		user.PasswordHash = hash
		user.TokenVersion++
	}
	if user.Role != newRole || user.Status != newStatus {
		user.TokenVersion++
	}
	user.Role = newRole
	user.Status = newStatus
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		if req.ProjectIDs != nil {
			if err := syncUserProjects(tx, user.ID, projectIDs); err != nil {
				return err
			}
		}
		if req.AuditIDs != nil || newRole == model.RoleSuperAdmin {
			return syncUserAuditScopes(tx, user.ID, auditIDs)
		}
		return nil
	})
	if err != nil {
		serverError(c, err)
		return
	}
	s.hydrateUser(&user)
	s.writeAudit(c, nil, "USER_UPDATE", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"username": user.Username, "role": user.Role, "status": user.Status, "project_ids": projectIDs, "audit_ids": auditIDs})
	ok(c, userView(user))
}

func (s *Server) changePassword(c *gin.Context) {
	user := currentUser(c)
	if user.ID == 0 {
		fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录", nil)
		return
	}
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入当前密码和新密码")
		return
	}
	if !security.CheckPassword(user.PasswordHash, req.OldPassword) {
		badRequest(c, "当前密码不正确")
		return
	}
	if req.OldPassword == req.NewPassword {
		badRequest(c, "新密码不能与当前密码相同")
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		badRequest(c, err.Error())
		return
	}
	hash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		serverError(c, err)
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_hash": hash,
		"token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "PASSWORD_CHANGE", "USER", fmt.Sprint(user.ID), "SUCCESS", nil)
	ok(c, gin.H{"changed": true})
}

func (s *Server) updateProfile(c *gin.Context) {
	user := currentUser(c)
	if user.ID == 0 {
		fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录", nil)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).Update("email", email).Error; err != nil {
		serverError(c, err)
		return
	}
	user.Email = email
	s.writeAudit(c, nil, "PROFILE_UPDATE", "USER", fmt.Sprint(user.ID), "SUCCESS", map[string]any{"email": email})
	ok(c, gin.H{"user": userView(*user)})
}

func (s *Server) validateProjectIDs(projectIDs []uint64) error {
	if len(projectIDs) == 0 {
		return nil
	}
	var count int64
	if err := s.db.Model(&model.Project{}).Where("id IN ? AND deleted_at IS NULL", projectIDs).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(projectIDs) {
		return fmt.Errorf("存在无效或已删除的项目授权")
	}
	return nil
}

func (s *Server) validateAuditIDs(auditIDs, projectIDs []uint64) error {
	if len(auditIDs) == 0 {
		return nil
	}
	unique := make(map[uint64]bool, len(auditIDs))
	ids := make([]uint64, 0, len(auditIDs))
	for _, id := range auditIDs {
		if id == 0 || unique[id] {
			continue
		}
		unique[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	var audits []model.Audit
	if err := s.db.Where("id IN ? AND deleted_at IS NULL", ids).Find(&audits).Error; err != nil {
		return err
	}
	if len(audits) != len(ids) {
		return fmt.Errorf("存在无效或已删除的审核配置")
	}
	if len(projectIDs) > 0 {
		allowed := make(map[uint64]bool, len(projectIDs))
		for _, id := range projectIDs {
			allowed[id] = true
		}
		for _, audit := range audits {
			if !allowed[audit.ProjectID] {
				return fmt.Errorf("审核配置「%s」不在账号的项目范围内", audit.Name)
			}
		}
	}
	return nil
}

func syncUserAuditScopes(tx *gorm.DB, userID uint64, auditIDs []uint64) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.UserAuditScope{}).Error; err != nil {
		return err
	}
	seen := make(map[uint64]bool, len(auditIDs))
	for _, auditID := range auditIDs {
		if auditID == 0 || seen[auditID] {
			continue
		}
		seen[auditID] = true
		if err := tx.Create(&model.UserAuditScope{UserID: userID, AuditID: auditID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func syncUserProjects(tx *gorm.DB, userID uint64, projectIDs []uint64) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.ProjectMember{}).Error; err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		member := model.ProjectMember{UserID: userID, ProjectID: projectID}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
	}
	return nil
}

func validatePassword(password string) error {
	if len([]rune(password)) < 10 {
		return fmt.Errorf("密码至少需要 10 位")
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("密码必须同时包含字母和数字")
	}
	return nil
}
