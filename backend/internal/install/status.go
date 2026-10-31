package install

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"smartdoc-vision/backend/internal/config"
	appversion "smartdoc-vision/backend/internal/version"
)

type SystemInfo struct {
	Version         string `json:"version"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	WorkingDir      string `json:"working_dir"`
	EnvFile         string `json:"env_file"`
	DefaultEnvPath  string `json:"default_env_path"`
	Docker          bool   `json:"docker"`
	LocalStorageDir string `json:"local_storage_dir"`
	DockerMountHint string `json:"docker_mount_hint,omitempty"`
}

type Defaults struct {
	MySQL   MySQL   `json:"mysql"`
	Redis   Redis   `json:"redis"`
	Storage Storage `json:"storage"`
	Admin   Admin   `json:"admin"`
	Site    Site    `json:"site"`
}

type Status struct {
	State       State      `json:"state"`
	Version     string     `json:"version"`
	InstalledAt string     `json:"installed_at,omitempty"`
	MarkerPath  string     `json:"marker_path,omitempty"`
	EnvFile     string     `json:"env_file,omitempty"`
	Configured  bool       `json:"configured"`
	System      SystemInfo `json:"system"`
	Defaults    Defaults   `json:"defaults"`
}

func BuildStatus(cfg config.Config, state State) Status {
	wd, _ := os.Getwd()
	envFile := cfg.EnvFile
	if envFile == "" {
		envFile = config.DefaultEnvPath()
	}
	localDir := cfg.StorageLocalDir
	if abs, err := filepath.Abs(localDir); err == nil {
		localDir = abs
	}
	docker := inDocker()
	status := Status{
		State:      state,
		Version:    appversion.Version,
		EnvFile:    envFile,
		Configured: cfg.Configured,
		System: SystemInfo{
			Version:         appversion.Version,
			OS:              runtime.GOOS,
			Arch:            runtime.GOARCH,
			WorkingDir:      wd,
			EnvFile:         envFile,
			DefaultEnvPath:  config.DefaultEnvPath(),
			Docker:          docker,
			LocalStorageDir: localDir,
		},
		Defaults: defaultsFromConfig(cfg),
	}
	if docker {
		status.System.DockerMountHint = fmt.Sprintf(
			"检测到 Docker 环境。请将宿主机目录挂载到容器内（例如 -v /host/smartdoc-data:/app/data），并保持本地存储目录为 /app/data/uploads，否则容器删除后文件会丢失。",
		)
	}
	if marker, ok := FindMarker(); ok {
		status.MarkerPath = marker.Path
		if !marker.InstalledAt.IsZero() {
			status.InstalledAt = marker.InstalledAt.Format("2006-01-02 15:04:05")
		}
		if marker.Version != "" {
			status.Version = marker.Version
		}
	}
	return status
}

func defaultsFromConfig(cfg config.Config) Defaults {
	localDir := cfg.StorageLocalDir
	if abs, err := filepath.Abs(localDir); err == nil {
		localDir = abs
	}
	defaults := Defaults{
		MySQL: MySQL{
			Host: "127.0.0.1", Port: 3306, User: "root", Database: "smart-doc-vision", Charset: "utf8mb4",
		},
		Redis: Redis{
			Host: "127.0.0.1", Port: 6379, DB: cfg.RedisDB,
		},
		Storage: Storage{
			Driver:   "local",
			LocalDir: localDir,
			S3: S3{
				Provider: "aliyun",
			},
		},
		Admin: Admin{Username: "admin"},
		Site: Site{
			Name:   cfg.SiteName,
			NameEn: cfg.SiteNameEn,
			Logo:   cfg.SiteLogo,
		},
	}
	if parsed, err := mysqlDriver.ParseDSN(cfg.MySQLDSN); err == nil {
		if host, port, err := net.SplitHostPort(parsed.Addr); err == nil {
			defaults.MySQL.Host = host
			if value, err := strconv.Atoi(port); err == nil {
				defaults.MySQL.Port = value
			}
		}
		if parsed.User != "" {
			defaults.MySQL.User = parsed.User
		}
		if parsed.DBName != "" {
			defaults.MySQL.Database = parsed.DBName
		}
	}
	if host, port, err := net.SplitHostPort(cfg.RedisAddr); err == nil {
		defaults.Redis.Host = host
		if value, err := strconv.Atoi(port); err == nil {
			defaults.Redis.Port = value
		}
	} else if strings.TrimSpace(cfg.RedisAddr) != "" {
		defaults.Redis.Host = strings.TrimSpace(cfg.RedisAddr)
	}
	if strings.EqualFold(cfg.StorageDriver, "s3") {
		defaults.Storage.Driver = "s3"
		defaults.Storage.S3.Provider = cfg.StorageS3Provider
	}
	return defaults
}

func inDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		value := string(data)
		if strings.Contains(value, "docker") || strings.Contains(value, "kubepods") || strings.Contains(value, "containerd") {
			return true
		}
	}
	return false
}
