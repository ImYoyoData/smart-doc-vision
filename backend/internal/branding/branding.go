package branding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"smartdoc-vision/backend/internal/config"
)

const defaultFilePath = "data/branding.json"

// Settings is the runtime-overridable site branding. The .env values remain
// the defaults; this file lets a super administrator update branding without
// restarting the process.
type Settings struct {
	Name   string `json:"name"`
	NameEn string `json:"name_en"`
	Logo   string `json:"logo"`
}

var (
	mu      sync.RWMutex
	current *Settings
	path    = defaultFilePath
)

func FilePath() string {
	mu.RLock()
	defer mu.RUnlock()
	return path
}

// Load applies an existing runtime branding file to cfg. A missing file is
// normal: the .env values remain in effect.
func Load(cfg *config.Config) error {
	mu.Lock()
	defer mu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取站点品牌配置失败: %w", err)
	}
	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("解析站点品牌配置失败: %w", err)
	}
	settings, err = normalize(settings)
	if err != nil {
		return err
	}
	current = &settings
	apply(cfg, settings)
	return nil
}

// Get returns the effective runtime branding, falling back to cfg.
func Get(cfg config.Config) Settings {
	mu.RLock()
	defer mu.RUnlock()
	if current != nil {
		return *current
	}
	return Settings{Name: cfg.SiteName, NameEn: cfg.SiteNameEn, Logo: cfg.SiteLogo}
}

// Save validates, persists, and applies the new branding immediately.
func Save(cfg *config.Config, input Settings) (Settings, error) {
	settings, err := normalize(input)
	if err != nil {
		return Settings{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Settings{}, fmt.Errorf("创建品牌配置目录失败: %w", err)
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("生成品牌配置失败: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".branding-*.json")
	if err != nil {
		return Settings{}, fmt.Errorf("创建品牌配置临时文件失败: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return Settings{}, fmt.Errorf("设置品牌配置权限失败: %w", err)
	}
	if _, err := temp.Write(raw); err != nil {
		cleanup()
		return Settings{}, fmt.Errorf("写入品牌配置失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return Settings{}, fmt.Errorf("关闭品牌配置失败: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return Settings{}, fmt.Errorf("保存品牌配置失败: %w", err)
	}
	current = &settings
	apply(cfg, settings)
	return settings, nil
}

func normalize(settings Settings) (Settings, error) {
	settings.Name = strings.TrimSpace(settings.Name)
	settings.NameEn = strings.TrimSpace(settings.NameEn)
	settings.Logo = strings.TrimSpace(settings.Logo)
	if settings.Name == "" {
		settings.Name = "智审图档"
	}
	if settings.NameEn == "" {
		settings.NameEn = "SmartDoc Vision"
	}
	if len([]rune(settings.Name)) > 64 {
		return Settings{}, fmt.Errorf("网站名称不能超过 64 个字符")
	}
	if len([]rune(settings.NameEn)) > 64 {
		return Settings{}, fmt.Errorf("英文名称不能超过 64 个字符")
	}
	if len(settings.Logo) > 2*1024*1024 {
		return Settings{}, fmt.Errorf("Logo 地址过长")
	}
	return settings, nil
}

func apply(cfg *config.Config, settings Settings) {
	if cfg == nil {
		return
	}
	cfg.SiteName = settings.Name
	cfg.SiteNameEn = settings.NameEn
	cfg.SiteLogo = settings.Logo
}
