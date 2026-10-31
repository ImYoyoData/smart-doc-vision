package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/config"
)

func TestSiteInfoPayloadUsesConfiguredBranding(t *testing.T) {
	cfg := config.Config{
		SiteName:   "Acme Review",
		SiteNameEn: "Acme Vision",
		SiteLogo:   "https://cdn.example.com/logo.png",
	}
	payload := siteInfoPayload(cfg)
	if payload["name"] != "Acme Review" || payload["name_en"] != "Acme Vision" {
		t.Fatalf("unexpected names: %#v", payload)
	}
	if payload["logo"] != "https://cdn.example.com/logo.png" {
		t.Fatalf("unexpected remote logo: %#v", payload["logo"])
	}
}

func TestSiteLogoFileAndServe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logo.png")
	content := []byte("\x89PNG\r\n\x1a\nbrand")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SiteName: "Acme", SiteNameEn: "Acme Vision", SiteLogo: path}
	if payload := siteInfoPayload(cfg); payload["logo"] != "/api/site/logo" {
		t.Fatalf("local logo was not routed: %#v", payload["logo"])
	}

	recorder := serveLogoRequest(t, cfg)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Body.Len() != len(content) {
		t.Fatalf("body length = %d", recorder.Body.Len())
	}
}

func serveLogoRequest(t *testing.T, cfg config.Config) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/site/logo", func(c *gin.Context) { serveSiteLogo(cfg, c) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/site/logo", nil))
	return recorder
}

func TestServeSiteLogoRejectsMissingFile(t *testing.T) {
	cfg := config.Config{SiteLogo: filepath.Join(t.TempDir(), "missing.png")}
	if path, ok := siteLogoFile(cfg.SiteLogo); ok {
		t.Fatalf("missing file accepted: %q", path)
	}
	recorder := serveLogoRequest(t, cfg)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}
