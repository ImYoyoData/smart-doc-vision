package config

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func TestBuildMySQLDSN(t *testing.T) {
	dsn := buildMySQLDSN("127.0.0.1", 3306, "root", "p@ss:word", "smart-doc-vision", "utf8mb4")
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("invalid DSN: %v (%s)", err, dsn)
	}
	if parsed.User != "root" || parsed.Passwd != "p@ss:word" || parsed.DBName != "smart-doc-vision" {
		t.Fatalf("unexpected parsed DSN: %+v", parsed)
	}
	if !parsed.ParseTime {
		t.Fatalf("expected ParseTime=true")
	}
}

func TestEnsureMySQLCharset(t *testing.T) {
	if got := ensureMySQLCharset("root:pw@tcp(127.0.0.1:3306)/demo"); !strings.Contains(got, "charset=utf8mb4") {
		t.Fatalf("missing charset: %s", got)
	}
	if got := ensureMySQLCharset("root:pw@tcp(127.0.0.1:3306)/demo?charset=utf8"); got != "root:pw@tcp(127.0.0.1:3306)/demo?charset=utf8" {
		t.Fatalf("unexpected charset rewrite: %s", got)
	}
}

func TestEnsureMySQLLocation(t *testing.T) {
	// A DSN without loc makes the driver read DATETIME columns as UTC, which
	// shifts every stored timestamp by the zone offset.
	dsn := ensureMySQLLocation("root:pw@tcp(127.0.0.1:3306)/demo?charset=utf8mb4")
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("invalid DSN: %v (%s)", err, dsn)
	}
	if parsed.Loc == nil || parsed.Loc.String() != time.Local.String() {
		t.Fatalf("expected loc=%s, got %v", time.Local, parsed.Loc)
	}
	// An explicit loc is left untouched so operators can override it.
	explicit := "root:pw@tcp(127.0.0.1:3306)/demo?loc=UTC"
	if got := ensureMySQLLocation(explicit); got != explicit {
		t.Fatalf("explicit loc was rewritten: %s", got)
	}
}

func TestApplyTimeZone(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()

	name, err := ApplyTimeZone("UTC")
	if err != nil {
		t.Fatalf("UTC should load: %v", err)
	}
	if name != "UTC" || time.Local.String() != "UTC" {
		t.Fatalf("time.Local = %s (reported %s)", time.Local, name)
	}
	if _, offset := time.Now().Zone(); offset != 0 {
		t.Fatalf("expected UTC offset 0, got %d", offset)
	}

	if _, err := ApplyTimeZone("Asia/Shanghai"); err != nil {
		t.Fatalf("Asia/Shanghai should load from embedded tzdata: %v", err)
	}
	if _, offset := time.Now().In(time.Local).Zone(); offset != 8*3600 {
		t.Fatalf("expected +08:00, got %d", offset)
	}

	if _, err := ApplyTimeZone("Mars/Olympus"); err == nil {
		t.Fatal("unknown zone should fail so Load can fall back")
	}

	// "Local" keeps the current zone instead of resetting it.
	if _, err := ApplyTimeZone("Local"); err != nil {
		t.Fatalf("Local should be accepted: %v", err)
	}
	if time.Local.String() != "Asia/Shanghai" {
		t.Fatalf("Local should keep the applied zone, got %s", time.Local)
	}
}

func TestLocalTimeZoneName(t *testing.T) {
	original := time.Local
	defer func() { time.Local = original }()
	if _, err := ApplyTimeZone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	if got := LocalTimeZoneName(); got != "Asia/Shanghai" {
		t.Fatalf("LocalTimeZoneName() = %q", got)
	}
}

func TestWriteDefaultEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := writeDefaultEnv(path); err != nil {
		t.Fatal(err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"SERVER_PORT", "DB_DRIVER", "DB_HOST", "DB_PORT", "DB_NAME", "REDIS_ADDR", "REDIS_DB", "JWT_SECRET", "MASTER_KEY"} {
		if strings.TrimSpace(values[key]) == "" {
			t.Fatalf("generated .env missing %s", key)
		}
	}
	if _, ok := values["ADMIN_PASSWORD"]; ok {
		t.Fatal("generated .env must not contain administrator credentials")
	}
}

func TestEnvTemplateTrimsOptionalSections(t *testing.T) {
	template := EnvTemplate(map[string]string{
		"APP_ENV":             "production",
		"DB_DRIVER":           DriverMySQL,
		"STORAGE_DRIVER":      "local",
		"STORAGE_LOCAL_DIR":   "data/uploads",
		"DEFAULT_AI_BASE_URL": "https://api.openai.com/v1",
	})
	for _, want := range []string{"APP_TIMEZONE=Asia/Shanghai", "STORAGE_LOCAL_DIR=data/uploads", "DB_CHARSET="} {
		if !strings.Contains(template, want) {
			t.Fatalf("template missing %q:\n%s", want, template)
		}
	}
	// Model defaults now live in the provider page; a MySQL + local-storage
	// deployment must not be handed S3 or JEV placeholders to maintain.
	for _, unwanted := range []string{"DEFAULT_AI_BASE_URL=", "JEV_MODEL=", "STORAGE_S3_BUCKET=", "DB_SSLMODE="} {
		if strings.Contains(template, unwanted) {
			t.Fatalf("template should not contain %q:\n%s", unwanted, template)
		}
	}

	s3 := EnvTemplate(map[string]string{"DB_DRIVER": DriverPostgres, "STORAGE_DRIVER": "s3"})
	for _, want := range []string{"DB_SSLMODE=", "STORAGE_S3_BUCKET="} {
		if !strings.Contains(s3, want) {
			t.Fatalf("s3/postgres template missing %q:\n%s", want, s3)
		}
	}
	if strings.Contains(s3, "DB_CHARSET=") || strings.Contains(s3, "STORAGE_LOCAL_DIR=") {
		t.Fatalf("s3/postgres template kept MySQL/local-only keys:\n%s", s3)
	}
}

func TestPickPrecedence(t *testing.T) {
	t.Setenv("SDV_TEST_PICK", "os")
	yamlValue := "yaml"
	dotenv := map[string]string{"SDV_TEST_PICK": "dotenv"}
	if got := pickString("SDV_TEST_PICK", &yamlValue, dotenv, "fallback"); got != "os" {
		t.Fatalf("expected OS env to win, got %q", got)
	}

	yamlOnly := "yaml-value"
	if got := pickString("SDV_TEST_PICK_YAML", &yamlOnly, map[string]string{"SDV_TEST_PICK_YAML": "dotenv-value"}, "fallback"); got != "dotenv-value" {
		t.Fatalf("expected .env to beat YAML, got %q", got)
	}

	if got := pickString("SDV_TEST_PICK_DOTENV", nil, map[string]string{"SDV_TEST_PICK_DOTENV": "dotenv-value"}, "fallback"); got != "dotenv-value" {
		t.Fatalf("expected .env fallback, got %q", got)
	}
}

func TestEnvFileLooksLikeTemplate(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.env")
	content := "MYSQL_HOST=127.0.0.1\nMYSQL_PASSWORD=your_password\nJWT_SECRET=replace-with-a-long-random-secret\n"
	if err := os.WriteFile(template, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if !EnvFileLooksLikeTemplate(template) {
		t.Fatal("expected shipped example values to be detected as a template")
	}
	realConfig := filepath.Join(dir, "real.env")
	content = "MYSQL_DSN=root:secret@tcp(127.0.0.1:3306)/smart-doc-vision\nJWT_SECRET=actual-secret\n"
	if err := os.WriteFile(realConfig, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if EnvFileLooksLikeTemplate(realConfig) {
		t.Fatal("real configuration must not be treated as a template")
	}
}

func TestAutoGeneratedEnvMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX mode bits")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := writeDefaultEnv(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("generated .env should not be group/world readable: %v", info.Mode().Perm())
	}
}

func TestMigrateLegacyEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	legacy := strings.Join([]string{
		"APP_ENV=production",
		"SERVER_PORT=8080",
		"MYSQL_HOST=db.legacy",
		"MYSQL_PORT=3307",
		"MYSQL_USER=svc",
		"MYSQL_PASSWORD=legacy-pass",
		"MYSQL_DATABASE=legacy_db",
		"MYSQL_CHARSET=utf8mb4",
		"ADMIN_USERNAME=admin",
		"ADMIN_PASSWORD=Admin@123456",
		"SITE_NAME=旧站点",
		"CUSTOM_FLAG=keep-me",
	}, "\n")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		EnvFile: path, AppEnv: "production", ServerPort: "8080",
		DatabaseDriver: DriverMySQL, DatabaseHost: "db.legacy", DatabasePort: 3307,
		DatabaseUser: "svc", DatabasePassword: "legacy-pass", DatabaseName: "legacy_db",
		DatabaseCharset: "utf8mb4", DatabaseSSLMode: "disable",
		DatabaseDSN: "svc:legacy-pass@tcp(db.legacy:3307)/legacy_db?charset=utf8mb4&parseTime=True",
		RedisAddr:   "127.0.0.1:6379", RedisDB: 3,
		JWTSecret: "jwt", MasterKey: "master",
		SiteName: "旧站点", SiteNameEn: "SmartDoc Vision",
		CORSOrigins: []string{"http://localhost:5173"}, WorkerCount: 4,
		StorageDriver: "local", StorageLocalDir: "data/uploads", StorageS3Provider: "custom",
	}
	backup, err := MigrateLegacyEnvFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected a migration backup")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "ADMIN_USERNAME", "ADMIN_PASSWORD"} {
		if _, ok := values[key]; ok {
			t.Fatalf("legacy key %s should be removed", key)
		}
	}
	if values["DB_DRIVER"] != DriverMySQL || values["DB_HOST"] != "db.legacy" || values["DB_PORT"] != "3307" ||
		values["DB_USER"] != "svc" || values["DB_PASSWORD"] != "legacy-pass" || values["DB_NAME"] != "legacy_db" {
		t.Fatalf("legacy database settings not converted: %+v", values)
	}
	if values["SITE_NAME"] != "旧站点" || values["CUSTOM_FLAG"] != "keep-me" {
		t.Fatalf("existing settings must be preserved: %+v", values)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "MASTER_KEY") || !strings.Contains(string(content), "DB_DRIVER") {
		t.Fatal("rewritten .env should keep the commented template")
	}
	if _, err := MigrateLegacyEnvFile(cfg); err != nil {
		t.Fatalf("second migration should be a no-op: %v", err)
	}
}

func TestMigrateLegacyEnvFileFromDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	legacyDSN := "svc:legacy-pass@tcp(db.legacy:3307)/legacy_db?charset=utf8mb4&parseTime=True&loc=Local"
	legacy := strings.Join([]string{
		"MYSQL_DSN=" + legacyDSN,
		"REDIS_ADDR=127.0.0.1:6379",
		"JWT_SECRET=jwt",
		"MASTER_KEY=master",
	}, "\n")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		EnvFile: path, DatabaseDriver: DriverMySQL, DatabaseDSN: legacyDSN,
		RedisAddr: "127.0.0.1:6379", RedisDB: 3,
		JWTSecret: "jwt", MasterKey: "master",
		StorageDriver: "local", StorageLocalDir: "data/uploads",
	}
	if _, err := MigrateLegacyEnvFile(cfg); err != nil {
		t.Fatal(err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_DSN"] != legacyDSN {
		t.Fatalf("legacy DSN should be preserved as DB_DSN: %q", values["DB_DSN"])
	}
	if values["DB_HOST"] != "db.legacy" || values["DB_PORT"] != "3307" ||
		values["DB_USER"] != "svc" || values["DB_PASSWORD"] != "legacy-pass" || values["DB_NAME"] != "legacy_db" {
		t.Fatalf("DSN should also be expanded into split fields: %+v", values)
	}
	if _, ok := values["MYSQL_DSN"]; ok {
		t.Fatal("legacy MYSQL_DSN should be removed")
	}
}

func TestBuildPostgresDSN(t *testing.T) {
	dsn := buildPostgresDSN("db.local", 5432, "svc", "p@ss:word", "smart_doc", "require")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("invalid postgres DSN: %v (%s)", err, dsn)
	}
	if parsed.Scheme != "postgres" || parsed.Host != "db.local:5432" || strings.TrimPrefix(parsed.Path, "/") != "smart_doc" {
		t.Fatalf("unexpected postgres DSN: %s", dsn)
	}
	password, _ := parsed.User.Password()
	if parsed.User.Username() != "svc" || password != "p@ss:word" {
		t.Fatalf("unexpected credentials in postgres DSN: %s", dsn)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("missing sslmode in postgres DSN: %s", dsn)
	}
	if got := InferDatabaseDriver(dsn); got != DriverPostgres {
		t.Fatalf("expected postgres driver, got %s", got)
	}
}

func TestDotenvBeatsYAMLDSN(t *testing.T) {
	host := "db.yml"
	var cfg yamlConfig
	cfg.MySQL.Host = &host
	dotenv := map[string]string{
		"MYSQL_DSN": "root:dotenv@tcp(dotenv-host:3306)/dotenv-db?charset=utf8mb4",
	}
	dsn := resolveDatabaseDSN(cfg, dotenv, DriverMySQL, "db.yml", 3306, "root", "secret", "smartdoc", "utf8mb4", "disable")
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Addr != "dotenv-host:3306" || parsed.DBName != "dotenv-db" {
		t.Fatalf(".env DSN should beat config.yml fields: %+v", parsed)
	}
}

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string
		port string
		want string
	}{
		{name: "defaults", host: "", port: "", want: "0.0.0.0:8080"},
		{name: "explicit", host: "127.0.0.1", port: "9000", want: "127.0.0.1:9000"},
		{name: "ipv6", host: "::", port: "8080", want: "[::]:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ServerHost: tc.host, ServerPort: tc.port}
			if got := cfg.ListenAddr(); got != tc.want {
				t.Fatalf("ListenAddr() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListenAddrFromEnv(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	content := "SERVER_HOST=127.0.0.1\nSERVER_PORT=9000\nDB_HOST=127.0.0.1\nJWT_SECRET=jwt\nMASTER_KEY=0123456789abcdef0123456789abcdef\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Empty environment values are ignored by pickString, so the .env values win.
	t.Setenv("SERVER_HOST", "")
	t.Setenv("SERVER_PORT", "")
	cfg := Load()
	if cfg.ServerHost != "127.0.0.1" || cfg.ServerPort != "9000" {
		t.Fatalf("unexpected listen config: host=%q port=%q", cfg.ServerHost, cfg.ServerPort)
	}
	if got := cfg.ListenAddr(); got != "127.0.0.1:9000" {
		t.Fatalf("ListenAddr() = %q", got)
	}
}

func TestDotenvBeatsYAMLRedisAddr(t *testing.T) {
	host := "redis.yml"
	var cfg yamlConfig
	cfg.Redis.Host = &host
	addr := resolveRedisAddr(cfg, map[string]string{"REDIS_ADDR": "dotenv-host:6379"}, "redis.yml", 6379)
	if addr != "dotenv-host:6379" {
		t.Fatalf(".env addr should beat config.yml fields, got %s", addr)
	}
}
