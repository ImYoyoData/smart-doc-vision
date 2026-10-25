package api

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/model"
)

var customRolePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,31}$`)

func (s *Server) refreshRoleCache() error {
	var records []model.Role
	if err := s.db.Order("builtin desc, id asc").Find(&records).Error; err != nil {
		return err
	}
	definitions := make([]catalog.RoleDefinition, 0, len(records))
	for _, record := range records {
		var permissions, menus []string
		if err := model.DecodeJSON(record.Permissions, &permissions); err != nil {
			continue
		}
		if err := model.DecodeJSON(record.Menus, &menus); err != nil {
			continue
		}
		definitions = append(definitions, catalog.RoleDefinition{
			Code: record.Code, Name: record.Name, Description: record.Description,
			Permissions: permissions, Menus: menus, Builtin: record.Builtin,
		})
	}
	if len(definitions) == 0 {
		definitions = catalog.Roles()
	}
	cache := make(map[string]catalog.RoleDefinition, len(definitions))
	for _, definition := range definitions {
		cache[definition.Code] = definition
	}
	s.rolesMu.Lock()
	s.roles = cache
	s.rolesList = definitions
	s.rolesMu.Unlock()
	return nil
}

func (s *Server) roleDefinition(code string) (catalog.RoleDefinition, bool) {
	s.rolesMu.RLock()
	definition, ok := s.roles[code]
	s.rolesMu.RUnlock()
	return definition, ok
}

func (s *Server) roleDefinitionsList() []catalog.RoleDefinition {
	s.rolesMu.RLock()
	defer s.rolesMu.RUnlock()
	out := make([]catalog.RoleDefinition, len(s.rolesList))
	copy(out, s.rolesList)
	return out
}

func (s *Server) validRole(code string) bool {
	_, ok := s.roleDefinition(code)
	return ok
}

func (s *Server) hasPermission(role, permission string) bool {
	if role == model.RoleSuperAdmin {
		return true
	}
	definition, ok := s.roleDefinition(role)
	if !ok {
		return false
	}
	for _, item := range definition.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func (s *Server) roleDefinitions(c *gin.Context) {
	ok(c, gin.H{
		"items":           s.roleDefinitionsList(),
		"all_permissions": catalog.AllPermissions(),
		"all_menus":       catalog.AllMenus(),
	})
}

type roleInput struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Menus       []string `json:"menus"`
}

func (s *Server) createRole(c *gin.Context) {
	if currentUser(c).Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可管理角色")
		return
	}
	var req roleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if !customRolePattern.MatchString(req.Code) {
		badRequest(c, "角色编码需以大写字母开头，3~32 位，仅支持大写字母、数字和下划线")
		return
	}
	if definition, exists := s.roleDefinition(req.Code); exists {
		_ = definition
		conflict(c, "ROLE_EXISTS", "角色编码已存在")
		return
	}
	if err := validateRoleInput(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	record := model.Role{
		Code: req.Code, Name: req.Name, Description: req.Description,
		Permissions: model.JSONBytes(req.Permissions), Menus: model.JSONBytes(req.Menus),
	}
	if err := s.db.Create(&record).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "ROLE_EXISTS", "角色编码已存在")
			return
		}
		serverError(c, err)
		return
	}
	if err := s.refreshRoleCache(); err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "ROLE_CREATE", "ROLE", req.Code, "SUCCESS",
		map[string]any{"name": req.Name, "permissions": req.Permissions, "menus": req.Menus})
	created(c, gin.H{"role": record})
}

func (s *Server) updateRole(c *gin.Context) {
	if currentUser(c).Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可管理角色")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	var record model.Role
	if err := s.db.Where("code = ?", code).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notFound(c, "角色不存在")
			return
		}
		serverError(c, err)
		return
	}
	if record.Builtin {
		badRequest(c, "内置角色不可修改，可新建自定义角色")
		return
	}
	var req roleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if err := validateRoleInput(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.db.Model(&model.Role{}).Where("id = ?", record.ID).Updates(map[string]any{
		"name": req.Name, "description": req.Description,
		"permissions": model.JSONBytes(req.Permissions), "menus": model.JSONBytes(req.Menus),
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := s.refreshRoleCache(); err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "ROLE_UPDATE", "ROLE", code, "SUCCESS",
		map[string]any{"name": req.Name, "permissions": req.Permissions, "menus": req.Menus})
	ok(c, gin.H{"updated": true})
}

func (s *Server) deleteRole(c *gin.Context) {
	if currentUser(c).Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可管理角色")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	var record model.Role
	if err := s.db.Where("code = ?", code).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notFound(c, "角色不存在")
			return
		}
		serverError(c, err)
		return
	}
	if record.Builtin {
		badRequest(c, "内置角色不可删除")
		return
	}
	var users int64
	if err := s.db.Model(&model.User{}).Where("role = ?", code).Count(&users).Error; err != nil {
		serverError(c, err)
		return
	}
	if users > 0 {
		conflict(c, "ROLE_IN_USE", fmt.Sprintf("仍有 %d 个账号使用该角色，请先调整账号角色", users))
		return
	}
	if err := s.db.Delete(&model.Role{}, record.ID).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := s.refreshRoleCache(); err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "ROLE_DELETE", "ROLE", code, "SUCCESS", nil)
	ok(c, gin.H{"deleted": true, "code": code})
}

func validateRoleInput(req *roleInput) error {
	if req.Name == "" {
		return errors.New("请填写角色名称")
	}
	if len([]rune(req.Name)) > 64 {
		return errors.New("角色名称不能超过 64 个字符")
	}
	if len(req.Permissions) == 0 {
		return errors.New("至少选择一项操作权限")
	}
	if len(req.Menus) == 0 {
		return errors.New("至少选择一个可见菜单")
	}
	allowedPermissions := map[string]bool{}
	for _, permission := range catalog.AllPermissions() {
		allowedPermissions[permission] = true
	}
	for _, permission := range req.Permissions {
		if !allowedPermissions[permission] {
			return fmt.Errorf("权限 %s 不合法", permission)
		}
	}
	allowedMenus := map[string]bool{}
	for _, menu := range catalog.AllMenus() {
		allowedMenus[menu] = true
	}
	for _, menu := range req.Menus {
		if !allowedMenus[menu] {
			return fmt.Errorf("菜单 %s 不合法", menu)
		}
	}
	return nil
}
