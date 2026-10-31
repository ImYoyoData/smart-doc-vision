package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"smartdoc-vision/backend/internal/config"
)

func TestMarkerLifecycleAndDetect(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	markerPath := filepath.Join(dir, markerFile)

	if state := Detect(config.Config{}); state != StateNeedsInstall {
		t.Fatalf("expected needs_install, got %s", state)
	}
	marker := Marker{Path: markerPath, InstalledAt: time.Now(), Version: "1.7.0", Storage: "local", Admin: "admin", Source: "test"}
	if err := WriteMarker(marker.Path, marker); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarker(marker.Path, marker); err != ErrAlreadyInstalled {
		t.Fatalf("expected ErrAlreadyInstalled, got %v", err)
	}
	if state := Detect(config.Config{}); state != StateConfigMissing {
		t.Fatalf("expected config_missing, got %s", state)
	}
	if state := Detect(config.Config{Configured: true}); state != StateInstalled {
		t.Fatalf("expected installed, got %s", state)
	}
	found, ok := FindMarker()
	if !ok || found.Version != "1.7.0" || found.Storage != "local" {
		t.Fatalf("unexpected marker: %+v (%v)", found, ok)
	}
}

func TestWriteEnvRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		AppEnv:                  "production",
		ServerPort:              "8080",
		MySQLDSN:                "root:pw@tcp(127.0.0.1:3306)/smart-doc-vision?charset=utf8mb4&parseTime=True",
		RedisAddr:               "127.0.0.1:6379",
		RedisPassword:           "",
		RedisDB:                 3,
		JWTSecret:               "jwt-secret",
		MasterKey:               "master-key",
		AdminUsername:           "admin",
		AdminPassword:           "Admin@123456",
		CORSOrigins:             []string{"http://localhost:5173"},
		WorkerCount:             4,
		DefaultAIBaseURL:        "https://api.openai.com/v1",
		DefaultAIModel:          "gpt-4o-mini",
		JEVBaseURL:              "https://api.typesafe.ai/v1",
		JEVModel:                "jev-latest",
		StorageDriver:           "local",
		StorageLocalDir:         filepath.Join(dir, "uploads"),
		StorageS3Provider:       "aliyun",
		StorageS3Region:         "cn-hangzhou",
		StorageS3ForcePathStyle: false,
		EnvFile:                 filepath.Join(dir, ".env"),
	}
	path, err := WriteEnv(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if path != cfg.EnvFile {
		t.Fatalf("unexpected env path %s", path)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MYSQL_DSN", "REDIS_ADDR", "JWT_SECRET", "MASTER_KEY", "ADMIN_PASSWORD", "STORAGE_DRIVER", "STORAGE_LOCAL_DIR"} {
		if strings.TrimSpace(values[key]) == "" {
			t.Fatalf("missing %s in generated env", key)
		}
	}
	if values["MYSQL_DSN"] != cfg.MySQLDSN {
		t.Fatalf("DSN not preserved: %q", values["MYSQL_DSN"])
	}
	if _, err := WriteEnv(cfg); err == nil {
		t.Fatal("expected an existing .env to be protected")
	}
}

func TestWriteEnvReplacesTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("MYSQL_HOST=127.0.0.1\nMYSQL_PASSWORD=your_password\nJWT_SECRET=replace-with-a-long-random-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{EnvFile: path, StorageDriver: "local", StorageLocalDir: filepath.Join(dir, "uploads")}
	if _, err := WriteEnv(cfg); err != nil {
		t.Fatalf("template .env should be replaceable: %v", err)
	}
	backups, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup of the template, got %v", backups)
	}
}

func TestBuildConfigAndValidation(t *testing.T) {
	base := config.Config{
		ServerPort:      "8080",
		RedisDB:         3,
		CORSOrigins:     []string{"http://localhost:5173"},
		WorkerCount:     4,
		StorageDriver:   "local",
		StorageLocalDir: "data/uploads",
	}
	req := Request{
		Agreement: true,
		MySQL:     MySQL{Host: "db.local", Port: 3307, User: "svc", Password: "p@ss", Database: "smartdoc", Charset: "utf8mb4"},
		Redis:     Redis{Host: "redis.local", Port: 6380, DB: 5},
		Storage:   Storage{Driver: "local", LocalDir: "data/uploads"},
		Admin:     Admin{Username: "root_admin", Password: "Secret@123"},
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg := BuildConfig(base, req)
	parsed, err := mysqlDriver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Addr != "db.local:3307" || parsed.DBName != "smartdoc" || parsed.Passwd != "p@ss" {
		t.Fatalf("unexpected DSN: %+v", parsed)
	}
	if cfg.RedisAddr != "redis.local:6380" || cfg.RedisDB != 5 {
		t.Fatalf("unexpected redis config: %s db=%d", cfg.RedisAddr, cfg.RedisDB)
	}
	if !cfg.Configured || cfg.AdminUsername != "root_admin" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !filepath.IsAbs(cfg.StorageLocalDir) {
		t.Fatalf("storage dir should be absolute: %s", cfg.StorageLocalDir)
	}

	bad := req
	bad.Agreement = false
	if err := bad.Validate(); err == nil {
		t.Fatal("expected agreement validation to fail")
	}
}

func TestWriteMarkerPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, markerFile)
	if err := WriteMarker(path, Marker{Version: "test"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not expose POSIX permission bits.
	if info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("marker should not be group/world readable: %v", info.Mode().Perm())
	}
}
