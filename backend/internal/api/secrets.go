package api

import (
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

var secretNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{1,63}$`)

type projectSecretInput struct {
	Value string `json:"value"`
}

func (s *Server) listProjectSecrets(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权查看项目密钥")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	var secrets []model.ProjectSecret
	if err := s.db.Where("project_id = ?", project.ID).Order("name asc").Find(&secrets).Error; err != nil {
		serverError(c, err)
		return
	}
	items := make([]gin.H, 0, len(secrets))
	for _, secret := range secrets {
		items = append(items, gin.H{
			"id": secret.ID, "name": secret.Name,
			"created_at": secret.CreatedAt, "updated_at": secret.UpdatedAt,
		})
	}
	ok(c, gin.H{"items": items, "total": len(items)})
}

func (s *Server) putProjectSecret(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权配置项目密钥")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	name := strings.TrimSpace(c.Param("name"))
	if !secretNamePattern.MatchString(name) {
		badRequest(c, "密钥名称需以字母开头，仅含字母、数字、下划线，长度 2~64")
		return
	}
	var req projectSecretInput
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Value) == "" {
		badRequest(c, "请填写密钥值")
		return
	}
	encrypted, err := s.cipher.Encrypt(req.Value)
	if err != nil {
		serverError(c, err)
		return
	}
	var secret model.ProjectSecret
	err = s.db.Where("project_id = ? AND name = ?", project.ID, name).First(&secret).Error
	if err != nil {
		secret = model.ProjectSecret{ProjectID: project.ID, Name: name, ValueEnc: encrypted}
		if err := s.db.Create(&secret).Error; err != nil {
			serverError(c, err)
			return
		}
	} else {
		secret.ValueEnc = encrypted
		if err := s.db.Save(&secret).Error; err != nil {
			serverError(c, err)
			return
		}
	}
	s.writeAudit(c, &project.ID, "PROJECT_SECRET_UPDATE", "SECRET", name, "SUCCESS",
		map[string]any{"project_id": project.ID, "name": name})
	ok(c, gin.H{"name": name, "updated": true})
}

func (s *Server) deleteProjectSecret(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权删除项目密钥")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	name := strings.TrimSpace(c.Param("name"))
	if err := s.db.Where("project_id = ? AND name = ?", project.ID, name).Delete(&model.ProjectSecret{}).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "PROJECT_SECRET_DELETE", "SECRET", name, "SUCCESS",
		map[string]any{"project_id": project.ID, "name": name})
	ok(c, gin.H{"deleted": true})
}
