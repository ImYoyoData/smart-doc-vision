package install

import (
	"testing"

	"smartdoc-vision/backend/internal/config"
)

func TestBuildConfigAppliesSiteBranding(t *testing.T) {
	cfg := BuildConfig(config.Config{}, Request{
		Site: Site{Name: "Acme Review", NameEn: "Acme Vision", Logo: "https://cdn.example.com/logo.png"},
	})
	if cfg.SiteName != "Acme Review" || cfg.SiteNameEn != "Acme Vision" {
		t.Fatalf("site names = %q / %q", cfg.SiteName, cfg.SiteNameEn)
	}
	if cfg.SiteLogo != "https://cdn.example.com/logo.png" {
		t.Fatalf("site logo = %q", cfg.SiteLogo)
	}
}

func TestBuildConfigFallsBackToPlatformBranding(t *testing.T) {
	cfg := BuildConfig(config.Config{}, Request{})
	if cfg.SiteName != "智审图档" || cfg.SiteNameEn != "SmartDoc Vision" {
		t.Fatalf("site names = %q / %q", cfg.SiteName, cfg.SiteNameEn)
	}
	if cfg.SiteLogo != "" {
		t.Fatalf("site logo = %q", cfg.SiteLogo)
	}
}

func TestRequestValidateSite(t *testing.T) {
	if err := (Request{Site: Site{Name: "Acme"}}).ValidateSite(); err != nil {
		t.Fatalf("valid branding rejected: %v", err)
	}
	if err := (Request{Site: Site{Name: string(make([]rune, 65))}}).ValidateSite(); err == nil {
		t.Fatal("overlong name was accepted")
	}
}
