package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/config"
	appversion "smartdoc-vision/backend/internal/version"
)

const maxSiteLogoBytes = 10 << 20

var siteLogoExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	".ico":  true,
	".avif": true,
}

func siteInfoPayload(cfg config.Config) gin.H {
	logo := strings.TrimSpace(cfg.SiteLogo)
	switch {
	case isRemoteSiteLogo(logo):
		// SITE_LOGO may point at an object URL or an inline data URL.
	case logo != "":
		if _, ok := siteLogoFile(logo); ok {
			logo = "/api/site/logo"
		} else {
			logo = ""
		}
	}
	return gin.H{
		"name":    cfg.SiteName,
		"name_en": cfg.SiteNameEn,
		"logo":    logo,
		"version": appversion.Version,
	}
}

func isRemoteSiteLogo(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:image/")
}

func siteLogoFile(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || isRemoteSiteLogo(value) {
		return "", false
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", false
	}
	stat, err := os.Stat(path)
	if err != nil || stat.IsDir() || stat.Size() > maxSiteLogoBytes {
		return "", false
	}
	if !siteLogoExtensions[strings.ToLower(filepath.Ext(path))] {
		return "", false
	}
	return path, true
}

func serveSiteLogo(cfg config.Config, c *gin.Context) {
	path, ok := siteLogoFile(cfg.SiteLogo)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), stat.ModTime(), file)
}
