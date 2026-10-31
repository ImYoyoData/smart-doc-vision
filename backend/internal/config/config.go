package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const defaultMasterKey = "dev-smartdoc-vision-master-key-change-me"
const defaultJWTSecret = "dev-smartdoc-vision-change-me"

type Config struct {
	AppEnv        string
	ServerPort    string
	MySQLDSN      string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	JWTSecret     string
	MasterKey     string
	AdminUsername string
	AdminPassword string
	CORSOrigins   []string
	WorkerCount   int
	AllowPrivate  bool

	SiteName   string
	SiteNameEn string
	SiteLogo   string
	SiteURL    string

	DefaultAIBaseURL        string
	DefaultAIModel          string
	DefaultAIAPIKey         string
	DefaultAISupportsVision bool

	JEVBaseURL string
	JEVModel   string

	StorageDriver           string
	StorageLocalDir         string
	StoragePublicBaseURL    string
	StorageS3Provider       string
	StorageS3Endpoint       string
	StorageS3Region         string
	StorageS3Bucket         string
	StorageS3AccessKey      string
	StorageS3SecretKey      string
	StorageS3Prefix         string
	StorageS3ForcePathStyle bool

	// Configured reports whether a config file or config environment variable
	// was found. When false the installer must run instead of connecting to
	// MySQL/Redis with built-in development defaults.
	Configured bool

	// Sources is for diagnostics only and is safe to log.
	ConfigFile string
	EnvFile    string
}

type yamlConfig struct {
	HTTP struct {
		Port *int `yaml:"port"`
	} `yaml:"http"`
	MySQL struct {
		DSN      *string `yaml:"dsn"`
		Host     *string `yaml:"host"`
		Port     *int    `yaml:"port"`
		User     *string `yaml:"user"`
		Password *string `yaml:"password"`
		Database *string `yaml:"database"`
		Charset  *string `yaml:"charset"`
	} `yaml:"mysql"`
	Redis struct {
		Addr     *string `yaml:"addr"`
		Host     *string `yaml:"host"`
		Port     *int    `yaml:"port"`
		Password *string `yaml:"password"`
		DB       *int    `yaml:"db"`
	} `yaml:"redis"`
	Security struct {
		JWTSecret *string `yaml:"jwt_secret"`
		MasterKey *string `yaml:"master_key"`
	} `yaml:"security"`
	Admin struct {
		Username *string `yaml:"username"`
		Password *string `yaml:"password"`
	} `yaml:"admin"`
	CORS struct {
		Origins []string `yaml:"origins"`
	} `yaml:"cors"`
	WorkerCount *int `yaml:"worker_count"`
	Media       struct {
		AllowPrivate *bool `yaml:"allow_private"`
	} `yaml:"media"`
	AI struct {
		DefaultBaseURL        *string `yaml:"openai_base_url"`
		DefaultModel          *string `yaml:"openai_model"`
		DefaultAPIKey         *string `yaml:"openai_api_key"`
		DefaultSupportsVision *bool   `yaml:"openai_supports_vision"`
		JEVBaseURL            *string `yaml:"jev_base_url"`
		JEVModel              *string `yaml:"jev_model"`
	} `yaml:"ai"`
}

func Load() Config {
	envPath := findConfigFile(".env")
	yamlPath := findConfigFile("config.yml")

	dotenvValues := map[string]string{}
	if envPath != "" {
		values, err := godotenv.Read(envPath)
		if err != nil {
			log.Printf("config: cannot read %s: %v", envPath, err)
		} else {
			dotenvValues = values
		}
	}
	// A file that only sets a port or AI defaults is not a complete deployment;
	// database/Redis settings (or a legacy config.yml) make it "configured".
	// A shipped .env template with placeholder secrets also counts as
	// unconfigured so the web installer can still run.
	dotenvConfigured := dotenvHas(dotenvValues,
		"MYSQL_DSN", "MYSQL_HOST", "MYSQL_USER", "MYSQL_DATABASE",
		"REDIS_ADDR", "REDIS_HOST", "REDIS_DB",
	)
	if envLooksLikeTemplate(dotenvValues) {
		dotenvConfigured = false
	}
	configured := yamlPath != "" || hasConfigEnv() || dotenvConfigured

	fileCfg := yamlConfig{}
	if yamlPath != "" && envPath != "" {
		log.Printf("config: both %s and %s found; .env values take precedence", envPath, yamlPath)
	}
	if yamlPath != "" {
		raw, err := os.ReadFile(yamlPath)
		if err != nil {
			log.Printf("config: cannot read %s: %v", yamlPath, err)
		} else if err := yaml.Unmarshal(raw, &fileCfg); err != nil {
			log.Printf("config: cannot parse %s, using defaults/env: %v", yamlPath, err)
			fileCfg = yamlConfig{}
		}
	}

	serverPort := pickInt("SERVER_PORT", fileCfg.HTTP.Port, dotenvValues, 8080)
	if serverPort <= 0 || serverPort > 65535 {
		serverPort = 8080
	}

	mysqlHost := fallbackString(pickString("MYSQL_HOST", fileCfg.MySQL.Host, dotenvValues, "127.0.0.1"), "127.0.0.1")
	mysqlPort := pickInt("MYSQL_PORT", fileCfg.MySQL.Port, dotenvValues, 3306)
	if mysqlPort <= 0 || mysqlPort > 65535 {
		mysqlPort = 3306
	}
	mysqlUser := fallbackString(pickString("MYSQL_USER", fileCfg.MySQL.User, dotenvValues, "root"), "root")
	mysqlPassword := pickString("MYSQL_PASSWORD", fileCfg.MySQL.Password, dotenvValues, "")
	mysqlDatabase := fallbackString(pickString("MYSQL_DATABASE", fileCfg.MySQL.Database, dotenvValues, "smart-doc-vision"), "smart-doc-vision")
	mysqlCharset := fallbackString(pickString("MYSQL_CHARSET", fileCfg.MySQL.Charset, dotenvValues, "utf8mb4"), "utf8mb4")
	mysqlDSN := resolveMySQLDSN(fileCfg, dotenvValues, mysqlHost, mysqlPort, mysqlUser, mysqlPassword, mysqlDatabase, mysqlCharset)

	redisHost := fallbackString(pickString("REDIS_HOST", fileCfg.Redis.Host, dotenvValues, "127.0.0.1"), "127.0.0.1")
	redisPort := pickInt("REDIS_PORT", fileCfg.Redis.Port, dotenvValues, 6379)
	if redisPort <= 0 || redisPort > 65535 {
		redisPort = 6379
	}
	redisAddr := resolveRedisAddr(fileCfg, dotenvValues, redisHost, redisPort)

	jwtSecret := fallbackString(pickString("JWT_SECRET", fileCfg.Security.JWTSecret, dotenvValues, defaultJWTSecret), defaultJWTSecret)
	masterKey := fallbackString(pickString("MASTER_KEY", fileCfg.Security.MasterKey, dotenvValues, defaultMasterKey), defaultMasterKey)
	if len(masterKey) < 16 {
		log.Printf("config: MASTER_KEY is shorter than 16 characters; using development default")
		masterKey = defaultMasterKey
	}

	cfg := Config{
		AppEnv:                  pickString("APP_ENV", nil, dotenvValues, "production"),
		ServerPort:              strconv.Itoa(serverPort),
		MySQLDSN:                ensureMySQLCharset(mysqlDSN),
		RedisAddr:               redisAddr,
		RedisPassword:           pickString("REDIS_PASSWORD", fileCfg.Redis.Password, dotenvValues, ""),
		RedisDB:                 pickInt("REDIS_DB", fileCfg.Redis.DB, dotenvValues, 3),
		JWTSecret:               jwtSecret,
		MasterKey:               masterKey,
		AdminUsername:           fallbackString(pickString("ADMIN_USERNAME", fileCfg.Admin.Username, dotenvValues, "admin"), "admin"),
		AdminPassword:           fallbackString(pickString("ADMIN_PASSWORD", fileCfg.Admin.Password, dotenvValues, "Admin@123456"), "Admin@123456"),
		CORSOrigins:             pickList("CORS_ORIGINS", fileCfg.CORS.Origins, dotenvValues, []string{"http://localhost:5173", "http://127.0.0.1:5173"}),
		WorkerCount:             pickInt("WORKER_COUNT", fileCfg.WorkerCount, dotenvValues, 4),
		AllowPrivate:            pickBool("MEDIA_ALLOW_PRIVATE", fileCfg.Media.AllowPrivate, dotenvValues, false),
		SiteName:                fallbackString(pickString("SITE_NAME", nil, dotenvValues, "智审图档"), "智审图档"),
		SiteNameEn:              fallbackString(pickString("SITE_NAME_EN", nil, dotenvValues, "SmartDoc Vision"), "SmartDoc Vision"),
		SiteLogo:                pickString("SITE_LOGO", nil, dotenvValues, ""),
		SiteURL:                 strings.TrimRight(pickString("SITE_URL", nil, dotenvValues, "http://127.0.0.1:"+strconv.Itoa(serverPort)), "/"),
		DefaultAIBaseURL:        fallbackString(pickString("DEFAULT_AI_BASE_URL", fileCfg.AI.DefaultBaseURL, dotenvValues, "https://api.openai.com/v1"), "https://api.openai.com/v1"),
		DefaultAIModel:          fallbackString(pickString("DEFAULT_AI_MODEL", fileCfg.AI.DefaultModel, dotenvValues, "gpt-4o-mini"), "gpt-4o-mini"),
		DefaultAIAPIKey:         pickString("DEFAULT_AI_API_KEY", fileCfg.AI.DefaultAPIKey, dotenvValues, ""),
		DefaultAISupportsVision: pickBool("DEFAULT_AI_SUPPORTS_VISION", fileCfg.AI.DefaultSupportsVision, dotenvValues, false),
		JEVBaseURL:              fallbackString(pickString("JEV_BASE_URL", fileCfg.AI.JEVBaseURL, dotenvValues, "https://api.typesafe.ai/v1"), "https://api.typesafe.ai/v1"),
		JEVModel:                fallbackString(pickString("JEV_MODEL", fileCfg.AI.JEVModel, dotenvValues, "jev-latest"), "jev-latest"),
		StorageDriver:           fallbackString(pickString("STORAGE_DRIVER", nil, dotenvValues, "local"), "local"),
		StorageLocalDir:         fallbackString(pickString("STORAGE_LOCAL_DIR", nil, dotenvValues, filepath.Join("data", "uploads")), filepath.Join("data", "uploads")),
		StoragePublicBaseURL:    pickString("STORAGE_PUBLIC_BASE_URL", nil, dotenvValues, ""),
		StorageS3Provider:       fallbackString(pickString("STORAGE_S3_PROVIDER", nil, dotenvValues, "custom"), "custom"),
		StorageS3Endpoint:       pickString("STORAGE_S3_ENDPOINT", nil, dotenvValues, ""),
		StorageS3Region:         pickString("STORAGE_S3_REGION", nil, dotenvValues, ""),
		StorageS3Bucket:         pickString("STORAGE_S3_BUCKET", nil, dotenvValues, ""),
		StorageS3AccessKey:      pickString("STORAGE_S3_ACCESS_KEY", nil, dotenvValues, ""),
		StorageS3SecretKey:      pickString("STORAGE_S3_SECRET_KEY", nil, dotenvValues, ""),
		StorageS3Prefix:         pickString("STORAGE_S3_PREFIX", nil, dotenvValues, ""),
		StorageS3ForcePathStyle: pickBool("STORAGE_S3_FORCE_PATH_STYLE", nil, dotenvValues, false),
		Configured:              configured,
		ConfigFile:              yamlPath,
		EnvFile:                 envPath,
	}

	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 1
	}
	if cfg.WorkerCount > 32 {
		cfg.WorkerCount = 32
	}
	if cfg.RedisDB < 0 {
		cfg.RedisDB = 0
	}
	return cfg
}

func findConfigFile(name string) string {
	for _, dir := range []string{".", "..", "../.."} {
		path := filepath.Join(dir, name)
		if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
			return path
		}
	}
	return ""
}

// FindConfigFile exposes the config search order to the installer.
func FindConfigFile(name string) string { return findConfigFile(name) }

// DefaultEnvPath is where the installer writes .env when no config exists.
func DefaultEnvPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ".env"
	}
	return filepath.Join(wd, ".env")
}

// RandomSecret generates a hex secret for JWT_SECRET / MASTER_KEY.
func RandomSecret(size int) string { return randomHex(size) }

func hasConfigEnv() bool {
	// SERVER_PORT alone must not count as a configured installation: a bare
	// port override is common even on a first run that still needs the wizard.
	for _, key := range []string{
		"MYSQL_DSN", "MYSQL_HOST", "MYSQL_USER", "MYSQL_DATABASE",
		"REDIS_ADDR", "REDIS_HOST", "REDIS_DB", "JWT_SECRET", "MASTER_KEY",
		"ADMIN_USERNAME", "ADMIN_PASSWORD",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// EnvFileLooksLikeTemplate reports whether path is an untouched copy of
// .env.example (placeholder secrets or example MySQL password). The installer
// may safely back it up and replace it.
func EnvFileLooksLikeTemplate(path string) bool {
	values, err := godotenv.Read(path)
	if err != nil {
		return false
	}
	return envLooksLikeTemplate(values)
}

func envLooksLikeTemplate(values map[string]string) bool {
	if len(values) == 0 {
		return false
	}
	jwt := strings.TrimSpace(values["JWT_SECRET"])
	master := strings.TrimSpace(values["MASTER_KEY"])
	password := strings.TrimSpace(values["MYSQL_PASSWORD"])
	dsn := strings.TrimSpace(values["MYSQL_DSN"])
	switch {
	case jwt == "replace-with-a-long-random-secret":
		return true
	case master == "replace-with-at-least-32-characters-master-key":
		return true
	case password == "your_password" && dsn == "":
		return true
	default:
		return false
	}
}

func resolveMySQLDSN(fileCfg yamlConfig, dotenv map[string]string, host string, port int, user, password, database, charset string) string {
	if value, ok := os.LookupEnv("MYSQL_DSN"); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	// .env is the primary configuration source: its fields beat a legacy
	// config.yml, and its full DSN beats legacy YAML fields.
	if dotenvHas(dotenv, "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MYSQL_CHARSET") {
		return buildMySQLDSN(host, port, user, password, database, charset)
	}
	if value := strings.TrimSpace(dotenv["MYSQL_DSN"]); value != "" {
		return value
	}
	if fileCfg.MySQL.DSN != nil && strings.TrimSpace(*fileCfg.MySQL.DSN) != "" {
		return strings.TrimSpace(*fileCfg.MySQL.DSN)
	}
	if yamlMySQLConfigured(fileCfg) {
		return buildMySQLDSN(host, port, user, password, database, charset)
	}
	return buildMySQLDSN(host, port, user, password, database, charset)
}

func resolveRedisAddr(fileCfg yamlConfig, dotenv map[string]string, host string, port int) string {
	if value, ok := os.LookupEnv("REDIS_ADDR"); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if dotenvHas(dotenv, "REDIS_HOST", "REDIS_PORT") {
		return net.JoinHostPort(host, strconv.Itoa(port))
	}
	if value := strings.TrimSpace(dotenv["REDIS_ADDR"]); value != "" {
		return value
	}
	if fileCfg.Redis.Addr != nil && strings.TrimSpace(*fileCfg.Redis.Addr) != "" {
		return strings.TrimSpace(*fileCfg.Redis.Addr)
	}
	if yamlRedisConfigured(fileCfg) {
		return net.JoinHostPort(host, strconv.Itoa(port))
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func yamlMySQLConfigured(cfg yamlConfig) bool {
	return cfg.MySQL.Host != nil || cfg.MySQL.Port != nil || cfg.MySQL.User != nil ||
		cfg.MySQL.Password != nil || cfg.MySQL.Database != nil || cfg.MySQL.Charset != nil
}

func yamlRedisConfigured(cfg yamlConfig) bool {
	return cfg.Redis.Host != nil || cfg.Redis.Port != nil || cfg.Redis.Password != nil || cfg.Redis.DB != nil
}

func envHas(keys ...string) bool {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func dotenvHas(dotenv map[string]string, keys ...string) bool {
	for _, key := range keys {
		if value, ok := dotenv[key]; ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func pickString(key string, yamlValue *string, dotenv map[string]string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := dotenv[key]; ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if yamlValue != nil {
		return strings.TrimSpace(*yamlValue)
	}
	return fallback
}

func pickInt(key string, yamlValue *int, dotenv map[string]string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		if number, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return number
		}
	}
	if value, ok := dotenv[key]; ok && strings.TrimSpace(value) != "" {
		if number, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return number
		}
	}
	if yamlValue != nil {
		return *yamlValue
	}
	return fallback
}

func pickBool(key string, yamlValue *bool, dotenv map[string]string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return parseBool(value, fallback)
	}
	if value, ok := dotenv[key]; ok && strings.TrimSpace(value) != "" {
		return parseBool(value, fallback)
	}
	if yamlValue != nil {
		return *yamlValue
	}
	return fallback
}

func pickList(key string, yamlValue []string, dotenv map[string]string, fallback []string) []string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return splitCSV(value)
	}
	if value, ok := dotenv[key]; ok && strings.TrimSpace(value) != "" {
		return splitCSV(value)
	}
	if len(yamlValue) > 0 {
		return yamlValue
	}
	return fallback
}

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func buildMySQLDSN(host string, port int, user, password, database, charset string) string {
	cfg := mysqlDriver.NewConfig()
	cfg.User = user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	cfg.DBName = database
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Params = map[string]string{"charset": fallbackString(charset, "utf8mb4")}
	return cfg.FormatDSN()
}

func ensureMySQLCharset(dsn string) string {
	if strings.Contains(strings.ToLower(dsn), "charset=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&charset=utf8mb4"
	}
	return dsn + "?charset=utf8mb4"
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func writeDefaultEnv(path string) error {
	jwtSecret := randomHex(32)
	masterKey := randomHex(32)
	content := fmt.Sprintf(`# SmartDoc Vision configuration
# Generated automatically on first run. Edit before production.
# Precedence: OS environment > .env > config.yml (legacy fallback) > built-in defaults.
APP_ENV=production
SERVER_PORT=8080
# Admin UI: http://127.0.0.1:8080/admin

MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=
MYSQL_DATABASE=smart-doc-vision
MYSQL_CHARSET=utf8mb4
# Full DSN takes precedence over the MYSQL_* fields above:
# MYSQL_DSN=root:your_password@tcp(127.0.0.1:3306)/smart-doc-vision?charset=utf8mb4&parseTime=True&loc=Local

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=3
# REDIS_ADDR=127.0.0.1:6379

JWT_SECRET=%s
MASTER_KEY=%s

ADMIN_USERNAME=admin
ADMIN_PASSWORD=Admin@123456

# Site branding. Restart after editing. SITE_LOGO accepts a local image path,
# an https URL, or a data:image/... URI; leave empty to use the built-in mark.
SITE_NAME=智审图档
SITE_NAME_EN=SmartDoc Vision
SITE_LOGO=
SITE_URL=http://127.0.0.1:8080

CORS_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
WORKER_COUNT=4
MEDIA_ALLOW_PRIVATE=false

DEFAULT_AI_BASE_URL=https://api.openai.com/v1
DEFAULT_AI_MODEL=gpt-4o-mini
DEFAULT_AI_API_KEY=
DEFAULT_AI_SUPPORTS_VISION=false

JEV_BASE_URL=https://api.typesafe.ai/v1
JEV_MODEL=jev-latest

# File storage: local directory by default, or any S3-compatible object storage.
STORAGE_DRIVER=local
STORAGE_LOCAL_DIR=data/uploads
STORAGE_PUBLIC_BASE_URL=
STORAGE_S3_PROVIDER=custom
STORAGE_S3_ENDPOINT=
STORAGE_S3_REGION=
STORAGE_S3_BUCKET=
STORAGE_S3_ACCESS_KEY=
STORAGE_S3_SECRET_KEY=
STORAGE_S3_PREFIX=
STORAGE_S3_FORCE_PATH_STYLE=false
`, jwtSecret, masterKey)
	return os.WriteFile(path, []byte(content), 0o600)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	return hex.EncodeToString(buf)
}
