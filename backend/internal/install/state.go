package install

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/config"
)

const markerFile = ".install.txt"

type State string

const (
	StateInstalled     State = "installed"
	StateNeedsInstall  State = "needs_install"
	StateConfigMissing State = "config_missing"
)

// ErrAlreadyInstalled is returned when a second install attempt races the
// installation marker.
var ErrAlreadyInstalled = errors.New("系统已经安装，不能重复执行安装流程")

type Marker struct {
	Path        string    `json:"path"`
	InstalledAt time.Time `json:"installed_at"`
	Version     string    `json:"version"`
	Storage     string    `json:"storage,omitempty"`
	Admin       string    `json:"admin,omitempty"`
	Source      string    `json:"source,omitempty"`
}

func Detect(cfg config.Config) State {
	_, hasMarker := FindMarker()
	if hasMarker {
		if cfg.Configured {
			return StateInstalled
		}
		return StateConfigMissing
	}
	if cfg.Configured {
		return StateInstalled
	}
	return StateNeedsInstall
}

func FindMarker() (Marker, bool) {
	for _, dir := range []string{".", "..", "../.."} {
		path := filepath.Join(dir, markerFile)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return parseMarker(path, data), true
	}
	return Marker{}, false
}

// MarkerPathForConfig keeps the marker next to the active .env. When no
// configuration exists yet, the installer writes both in the working directory.
func MarkerPathForConfig(cfg config.Config) string {
	envPath := cfg.EnvFile
	if strings.TrimSpace(envPath) == "" {
		envPath = config.DefaultEnvPath()
	}
	return filepath.Join(filepath.Dir(envPath), markerFile)
}

func EnvPath(cfg config.Config) string {
	if strings.TrimSpace(cfg.EnvFile) != "" {
		return cfg.EnvFile
	}
	return config.DefaultEnvPath()
}

func WriteMarker(path string, marker Marker) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("安装标记路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return ErrAlreadyInstalled
		}
		return err
	}
	defer file.Close()
	installedAt := marker.InstalledAt
	if installedAt.IsZero() {
		installedAt = time.Now()
	}
	content := strings.Join([]string{
		"# SmartDoc Vision installation marker",
		"# Keep this file: its presence prevents the web installer from running again.",
		"installed_at=" + installedAt.Format(time.RFC3339),
		"version=" + marker.Version,
		"storage=" + marker.Storage,
		"admin=" + marker.Admin,
		"source=" + marker.Source,
		"",
	}, "\n")
	if _, err := file.WriteString(content); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func parseMarker(path string, data []byte) Marker {
	marker := Marker{Path: path}
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	marker.InstalledAt, _ = time.Parse(time.RFC3339, values["installed_at"])
	marker.Version = values["version"]
	marker.Storage = values["storage"]
	marker.Admin = values["admin"]
	marker.Source = values["source"]
	return marker
}
