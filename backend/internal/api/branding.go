package api

import (
	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/branding"
)

func (s *Server) getSystemBranding(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权查看站点品牌")
		return
	}
	ok(c, gin.H{
		"name":    branding.Get(s.cfg).Name,
		"name_en": branding.Get(s.cfg).NameEn,
		"logo":    branding.Get(s.cfg).Logo,
	})
}

func (s *Server) updateSystemBranding(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权修改站点品牌")
		return
	}
	var input branding.Settings
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	saved, err := branding.Save(&s.cfg, input)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	s.writeAudit(c, nil, "SITE_BRANDING_UPDATE", "SYSTEM", "branding", "SUCCESS",
		map[string]any{"name": saved.Name, "name_en": saved.NameEn, "has_logo": saved.Logo != ""})
	ok(c, saved)
}
