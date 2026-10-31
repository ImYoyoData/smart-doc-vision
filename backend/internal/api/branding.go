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
	settings := branding.Get(s.cfg)
	ok(c, gin.H{
		"name":      settings.Name,
		"name_en":   settings.NameEn,
		"logo":      settings.Logo,
		"watermark": settings.Watermark,
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
	// Older clients submit only name/logo; keep the current watermark then.
	if input.Watermark == nil {
		input.Watermark = branding.Get(s.cfg).Watermark
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

func (s *Server) getSystemWatermark(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权查看水印设置")
		return
	}
	settings := branding.Get(s.cfg)
	ok(c, settings.Watermark)
}

func (s *Server) updateSystemWatermark(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权修改水印设置")
		return
	}
	var input branding.WatermarkSettings
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	saved, err := branding.SaveWatermark(&s.cfg, input)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	s.writeAudit(c, nil, "SITE_WATERMARK_UPDATE", "SYSTEM", "watermark", "SUCCESS",
		map[string]any{"enabled": saved.Watermark.Enabled, "font_size": saved.Watermark.FontSize,
			"opacity": saved.Watermark.Opacity, "density": saved.Watermark.Density})
	ok(c, saved.Watermark)
}
