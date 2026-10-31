package branding

import (
	"path/filepath"
	"testing"

	"smartdoc-vision/backend/internal/config"
)

func TestSaveLoadBranding(t *testing.T) {
	oldPath := path
	oldCurrent := current
	defer func() {
		path = oldPath
		current = oldCurrent
	}()
	path = filepath.Join(t.TempDir(), "branding.json")
	current = nil

	cfg := config.Config{SiteName: "Env Name", SiteNameEn: "Env Vision"}
	saved, err := Save(&cfg, Settings{Name: "Acme 审核云", NameEn: "Acme Review"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Acme 审核云" || cfg.SiteName != saved.Name {
		t.Fatalf("settings not applied: %#v / %#v", saved, cfg)
	}

	loaded := config.Config{SiteName: "Env Name", SiteNameEn: "Env Vision"}
	if err := Load(&loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.SiteName != "Acme 审核云" || loaded.SiteNameEn != "Acme Review" {
		t.Fatalf("branding not loaded: %#v", loaded)
	}
}

func TestSaveBrandingRejectsOverlongName(t *testing.T) {
	cfg := config.Config{}
	if _, err := Save(&cfg, Settings{Name: string(make([]rune, 65))}); err == nil {
		t.Fatal("overlong name accepted")
	}
}
