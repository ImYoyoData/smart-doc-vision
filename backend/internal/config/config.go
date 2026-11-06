package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the IANA database so Windows and slim containers resolve Asia/Shanghai

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const defaultMasterKey = "dev-smartdoc-vision-master-key-change-me"
const defaultJWTSecret = "dev-smartdoc-vision-change-me"

// DefaultTimeZone is used when APP_TIMEZONE is empty. "Local" (or "server")
// follows the operating system time zone instead.
const DefaultTimeZone = "Asia/Shanghai"

// defaultServerHost listens on every interface, matching the behaviour of the
// previous hard-coded ":port" address while still being explicit in the config.
const defaultServerHost = "0.0.0.0"

const (
	// DriverMySQL and DriverPostgres are the supported database backends.
	DriverMySQL    = "mysql"
	DriverPostgres = "postgres"
)

type Config struct {
	AppEnv           string
	TimeZone         string
	ServerHost       string
	ServerPort       string
	DatabaseDriver   string
	DatabaseDSN      string
	DatabaseHost     string
	DatabasePort     int
	DatabaseUser     string
	DatabasePassword string
	DatabaseName     string
	DatabaseCharset  string
	DatabaseSSLMode  string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	JWTSecret        string
	MasterKey        string
	AdminUsername    string
	AdminPassword    string
	CORSOrigins      []string
	WorkerCount      int
	AllowPrivate     bool

	SiteName   string
	SiteNameEn string
	SiteLogo   string
	SiteURL    string

	// DefaultAI*/JEV* are legacy .env seed values kept for compatibility; new
	// deployments configure models on the 「模型接入」 page instead.
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
		Host     *string `yaml:"host"`
		Port     *int    `yaml:"port"`
		TimeZone *string `yaml:"timezone"`
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
		"DB_DSN", "DATABASE_URL", "DB_DRIVER", "DB_HOST", "DB_USER", "DB_NAME",
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
	serverHost := fallbackString(pickString("SERVER_HOST", fileCfg.HTTP.Host, dotenvValues, defaultServerHost), defaultServerHost)

	// The time zone must be applied before any DSN or timestamp is built:
	// time.Local drives time.Now(), the MySQL `loc` parameter and GORM's
	// NowFunc, so setting it once keeps logs, JSON output and the database in
	// agreement.
	timeZone := pickString("APP_TIMEZONE", fileCfg.HTTP.TimeZone, dotenvValues, DefaultTimeZone)
	if resolved, err := ApplyTimeZone(timeZone); err != nil {
		log.Printf("config: %v; falling back to %s", err, DefaultTimeZone)
		timeZone, _ = ApplyTimeZone(DefaultTimeZone)
	} else {
		timeZone = resolved
	}

	databaseDriver := resolveDatabaseDriver(dotenvValues)
	defaultHost, defaultPort, defaultUser := "127.0.0.1", 3306, "root"
	if databaseDriver == DriverPostgres {
		defaultPort, defaultUser = 5432, "postgres"
	}
	databaseHost := pickString("DB_HOST", nil, dotenvValues, "")
	if databaseHost == "" {
		databaseHost = pickString("MYSQL_HOST", fileCfg.MySQL.Host, dotenvValues, defaultHost)
	}
	databasePort := pickInt("DB_PORT", nil, dotenvValues, 0)
	if databasePort <= 0 || databasePort > 65535 {
		databasePort = pickInt("MYSQL_PORT", fileCfg.MySQL.Port, dotenvValues, defaultPort)
	}
	if databasePort <= 0 || databasePort > 65535 {
		databasePort = defaultPort
	}
	databaseUser := pickString("DB_USER", nil, dotenvValues, "")
	if databaseUser == "" {
		databaseUser = pickString("MYSQL_USER", fileCfg.MySQL.User, dotenvValues, defaultUser)
	}
	databasePassword := pickString("DB_PASSWORD", nil, dotenvValues, "")
	if databasePassword == "" {
		databasePassword = pickString("MYSQL_PASSWORD", fileCfg.MySQL.Password, dotenvValues, "")
	}
	databaseName := pickString("DB_NAME", nil, dotenvValues, "")
	if databaseName == "" {
		databaseName = pickString("MYSQL_DATABASE", fileCfg.MySQL.Database, dotenvValues, "smart-doc-vision")
	}
	databaseCharset := pickString("DB_CHARSET", nil, dotenvValues, "")
	if databaseCharset == "" {
		databaseCharset = pickString("MYSQL_CHARSET", fileCfg.MySQL.Charset, dotenvValues, "utf8mb4")
	}
	databaseSSLMode := fallbackString(pickString("DB_SSLMODE", nil, dotenvValues, "disable"), "disable")
	databaseDSN := resolveDatabaseDSN(fileCfg, dotenvValues, databaseDriver,
		databaseHost, databasePort, databaseUser, databasePassword, databaseName, databaseCharset, databaseSSLMode)
	if databaseDriver == DriverMySQL {
		databaseDSN = ensureMySQLCharset(databaseDSN)
		databaseDSN = ensureMySQLLocation(databaseDSN)
	}

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
		TimeZone:                timeZone,
		ServerHost:              serverHost,
		ServerPort:              strconv.Itoa(serverPort),
		DatabaseDriver:          databaseDriver,
		DatabaseDSN:             databaseDSN,
		DatabaseHost:            databaseHost,
		DatabasePort:            databasePort,
		DatabaseUser:            databaseUser,
		DatabasePassword:        databasePassword,
		DatabaseName:            databaseName,
		DatabaseCharset:         databaseCharset,
		DatabaseSSLMode:         databaseSSLMode,
		RedisAddr:               redisAddr,
		RedisPassword:           pickString("REDIS_PASSWORD", fileCfg.Redis.Password, dotenvValues, ""),
		RedisDB:                 pickInt("REDIS_DB", fileCfg.Redis.DB, dotenvValues, 3),
		JWTSecret:               jwtSecret,
		MasterKey:               masterKey,
		AdminUsername:           pickString("ADMIN_USERNAME", fileCfg.Admin.Username, dotenvValues, ""),
		AdminPassword:           pickString("ADMIN_PASSWORD", fileCfg.Admin.Password, dotenvValues, ""),
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

// ApplyTimeZone loads the requested IANA zone (or "Local"/"server" for the
// operating system zone) and makes it the process-wide default by assigning
// time.Local. It returns the name that was applied.
func ApplyTimeZone(name string) (string, error) {
	value := strings.TrimSpace(name)
	switch strings.ToLower(value) {
	case "", "local", "server", "system":
		// Keep whatever the OS reports; resolve it once so callers see a name.
		return time.Local.String(), nil
	}
	location, err := time.LoadLocation(value)
	if err != nil {
		return "", fmt.Errorf("APP_TIMEZONE %q 无法识别: %w", value, err)
	}
	time.Local = location
	return location.String(), nil
}

// TimeZoneLabel renders a configured zone together with the offset currently
// in effect, e.g. "Asia/Shanghai (UTC+08:00)". It makes a wrong zone obvious
// in the startup log and on the system config page.
func TimeZoneLabel(name string) string {
	return fallbackString(strings.TrimSpace(name), DefaultTimeZone) + " (UTC" + time.Now().Format("-07:00") + ")"
}

// ListenAddr is the host:port the HTTP server binds to. An empty host means
// every interface, which is also the default.
func (c Config) ListenAddr() string {
	host := strings.TrimSpace(c.ServerHost)
	if host == "" {
		host = defaultServerHost
	}
	port := strings.TrimSpace(c.ServerPort)
	if port == "" {
		port = "8080"
	}
	return net.JoinHostPort(host, port)
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
		"DB_DSN", "DATABASE_URL", "DB_DRIVER", "DB_HOST", "DB_USER", "DB_NAME",
		"MYSQL_DSN", "MYSQL_HOST", "MYSQL_USER", "MYSQL_DATABASE",
		"REDIS_ADDR", "REDIS_HOST", "REDIS_DB", "JWT_SECRET", "MASTER_KEY",
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
	dbPassword := strings.TrimSpace(values["DB_PASSWORD"])
	dbDSN := strings.TrimSpace(values["DB_DSN"])
	switch {
	case jwt == "replace-with-a-long-random-secret":
		return true
	case master == "replace-with-at-least-32-characters-master-key":
		return true
	case password == "your_password" && dsn == "":
		return true
	case dbPassword == "your_password" && dbDSN == "":
		return true
	default:
		return false
	}
}

// NormalizeDatabaseDriver maps user input to a supported driver name.
func NormalizeDatabaseDriver(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DriverPostgres, "postgresql", "pg":
		return DriverPostgres
	case DriverMySQL, "mariadb":
		return DriverMySQL
	default:
		return DriverMySQL
	}
}

// InferDatabaseDriver guesses the driver from a DSN scheme.
func InferDatabaseDriver(dsn string) string {
	value := strings.ToLower(strings.TrimSpace(dsn))
	if strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://") {
		return DriverPostgres
	}
	return DriverMySQL
}

func resolveDatabaseDriver(dotenv map[string]string) string {
	if value := pickString("DB_DRIVER", nil, dotenv, ""); value != "" {
		return NormalizeDatabaseDriver(value)
	}
	for _, key := range []string{"DB_DSN", "DATABASE_URL"} {
		if raw := pickString(key, nil, dotenv, ""); raw != "" {
			return InferDatabaseDriver(raw)
		}
	}
	return DriverMySQL
}

func resolveDatabaseDSN(fileCfg yamlConfig, dotenv map[string]string, driver, host string, port int, user, password, database, charset, sslmode string) string {
	for _, key := range []string{"DB_DSN", "DATABASE_URL"} {
		if value := pickString(key, nil, dotenv, ""); value != "" {
			return value
		}
	}
	if driver == DriverMySQL {
		// Legacy MySQL installations keep their original precedence: explicit
		// MYSQL_* fields beat a full MYSQL_DSN, and config.yml is the fallback.
		if dotenvHas(dotenv, "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MYSQL_CHARSET") ||
			envHas("DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_CHARSET") {
			return buildDatabaseDSN(driver, host, port, user, password, database, charset, sslmode)
		}
		if value := pickString("MYSQL_DSN", fileCfg.MySQL.DSN, dotenv, ""); value != "" {
			return value
		}
	}
	return buildDatabaseDSN(driver, host, port, user, password, database, charset, sslmode)
}

func buildDatabaseDSN(driver, host string, port int, user, password, database, charset, sslmode string) string {
	if driver == DriverPostgres {
		return buildPostgresDSN(host, port, user, password, database, sslmode)
	}
	return buildMySQLDSN(host, port, user, password, database, charset)
}

func buildPostgresDSN(host string, port int, user, password, database, sslmode string) string {
	target := url.URL{Scheme: "postgres", Path: "/" + database}
	if strings.TrimSpace(user) != "" {
		target.User = url.UserPassword(user, password)
	}
	target.Host = net.JoinHostPort(host, strconv.Itoa(port))
	query := url.Values{}
	query.Set("sslmode", fallbackString(sslmode, "disable"))
	query.Set("connect_timeout", "10")
	if name := LocalTimeZoneName(); name != "" {
		query.Set("timezone", name)
	}
	target.RawQuery = query.Encode()
	return target.String()
}

// LocalTimeZoneName returns the IANA name of time.Local, or "" when the
// process follows an unnamed OS zone ("Local" on Windows). It is used to align
// database sessions with the Go runtime.
func LocalTimeZoneName() string {
	name := strings.TrimSpace(time.Local.String())
	if name == "" || strings.EqualFold(name, "Local") {
		return ""
	}
	if _, err := time.LoadLocation(name); err != nil {
		return ""
	}
	return name
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

// ensureMySQLLocation makes an explicit DSN follow the application time zone.
// Without `loc` the driver falls back to UTC, so every DATETIME written by Go
// would be stored as UTC while the UI and reports assume local wall-clock time.
func ensureMySQLLocation(dsn string) string {
	if hasDSNParam(dsn, "loc") {
		return dsn
	}
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return dsn
	}
	parsed.Loc = time.Local
	return parsed.FormatDSN()
}

// hasDSNParam reports whether a MySQL DSN already carries the given query key.
func hasDSNParam(dsn, key string) bool {
	index := strings.Index(dsn, "?")
	if index < 0 {
		return false
	}
	for _, part := range strings.Split(dsn[index+1:], "&") {
		if name, _, found := strings.Cut(part, "="); found && strings.EqualFold(strings.TrimSpace(name), key) {
			return true
		}
	}
	return false
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

// EnvTemplate renders a fully commented .env file. The web installer and the
// development defaults share it so the explanations never drift apart.
func EnvTemplate(values map[string]string) string {
	value := func(key string) string { return formatEnvValue(values[key]) }
	// Fall back to the same defaults Load() uses so a partially filled caller
	// never produces entries such as "SERVER_PORT=".
	orDefault := func(key, fallback string) string {
		return formatEnvValue(fallbackString(values[key], fallback))
	}
	dsnLine := "# 可选：完整连接串，填写后优先于上面的分项配置。"
	if strings.TrimSpace(values["DB_DSN"]) != "" {
		dsnLine = "DB_DSN=" + value("DB_DSN")
	}
	driver := NormalizeDatabaseDriver(values["DB_DRIVER"])
	storageDriver := strings.ToLower(strings.TrimSpace(values["STORAGE_DRIVER"]))
	dbPort, dbUser := "3306", "root"
	if driver == DriverPostgres {
		dbPort, dbUser = "5432", "postgres"
	}
	lines := []string{
		"# =============================================================================",
		"# SmartDoc Vision 配置文件（.env）",
		"# -----------------------------------------------------------------------------",
		"# 1. 本文件只在进程启动时读取一次：修改或新增任意一项后，必须重启服务才会生效。",
		"# 2. 优先级：系统环境变量 > .env > config.yml（旧版兼容） > 内置默认值。",
		"# 3. 后台账号（超级管理员、普通用户）保存在数据库中，由安装向导创建、",
		"#    在「账号管理」页面维护。本文件不再包含任何账号密码，修改这里不会影响已有账号。",
		"# 4. MASTER_KEY 用于加密数据库中的模型密钥、接入凭证 AppSecret、通知渠道配置等。",
		"#    首次安装后绝对不要修改；一旦修改，所有已加密数据将无法解密（等同于丢失）。",
		"# =============================================================================",
		"",
		"# ================================ 运行环境 ====================================",
		"# production / development。生产环境请保持 production；",
		"# development 会放宽 CORS 并输出更详细的调试信息。",
		"APP_ENV=" + orDefault("APP_ENV", "production"),
		"# 时区：日志、任务时间、报表按天统计和数据库写入都使用该时区。",
		"# 默认 Asia/Shanghai；填 Local 表示跟随服务器操作系统时区。修改后需重启。",
		"APP_TIMEZONE=" + orDefault("APP_TIMEZONE", DefaultTimeZone),
		"# HTTP 监听地址。默认 0.0.0.0 表示监听全部网卡；",
		"# 只允许本机访问可改为 127.0.0.1，也可用启动参数 -host 临时覆盖。",
		"SERVER_HOST=" + orDefault("SERVER_HOST", defaultServerHost),
		"# HTTP 监听端口。修改后需重启，并同步调整反向代理和 SITE_URL；",
		"# 也可用启动参数 -port 临时覆盖。",
		"SERVER_PORT=" + orDefault("SERVER_PORT", "8080"),
		"",
		"# ================================= 数据库 =====================================",
		"# 数据库类型：mysql 或 postgres（PostgreSQL）。",
		"DB_DRIVER=" + driver,
		"DB_HOST=" + orDefault("DB_HOST", "127.0.0.1"),
		"DB_PORT=" + orDefault("DB_PORT", dbPort),
		"DB_USER=" + orDefault("DB_USER", dbUser),
		"DB_PASSWORD=" + value("DB_PASSWORD"),
		"DB_NAME=" + orDefault("DB_NAME", "smart-doc-vision"),
	}
	if driver == DriverPostgres {
		lines = append(lines,
			"# PostgreSQL 专用：disable / require / verify-ca / verify-full。",
			"DB_SSLMODE="+orDefault("DB_SSLMODE", "disable"),
		)
	} else {
		lines = append(lines,
			"# MySQL 专用：字符集，通常保持 utf8mb4。",
			"DB_CHARSET="+orDefault("DB_CHARSET", "utf8mb4"),
		)
	}
	lines = append(lines, dsnLine)
	if driver == DriverPostgres {
		lines = append(lines, "# DB_DSN=postgres://postgres:password@127.0.0.1:5432/smart-doc-vision?sslmode=disable")
	} else {
		lines = append(lines, "# DB_DSN=root:password@tcp(127.0.0.1:3306)/smart-doc-vision?charset=utf8mb4&parseTime=True&loc=Local")
	}
	lines = append(lines,
		"",
		"# ================================== Redis ====================================",
		"# 用于任务队列、去重锁、运行监控与缓存。",
		"REDIS_ADDR="+orDefault("REDIS_ADDR", "127.0.0.1:6379"),
		"REDIS_PASSWORD="+value("REDIS_PASSWORD"),
		"REDIS_DB="+orDefault("REDIS_DB", "3"),
		"",
		"# ================================ 安全密钥 ====================================",
		"# JWT 登录令牌的签名密钥；修改后所有已登录会话立即失效，可以随时更换。",
		"JWT_SECRET="+value("JWT_SECRET"),
		"# 数据加密主密钥；用于加密数据库中的敏感配置，首次安装后不要修改（见文件顶部）。",
		"MASTER_KEY="+value("MASTER_KEY"),
		"",
		"# ================================ 站点品牌 ====================================",
		"# 修改后需重启；也可以留空，在「系统配置 → 站点品牌」中在线修改。",
		"# SITE_LOGO 支持 https 地址、data:image 或服务器本地图片路径。",
		"SITE_NAME="+orDefault("SITE_NAME", "智审图档"),
		"SITE_NAME_EN="+orDefault("SITE_NAME_EN", "SmartDoc Vision"),
		"SITE_LOGO="+value("SITE_LOGO"),
		"SITE_URL="+orDefault("SITE_URL", "http://127.0.0.1:8080"),
		"",
		"# ================================ 运行参数 ====================================",
		"# 允许跨域访问的页面来源，多个用英文逗号分隔。",
		"CORS_ORIGINS="+orDefault("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"),
		"# 审核 Worker 并发数，范围 1~32。",
		"WORKER_COUNT="+orDefault("WORKER_COUNT", "4"),
		"# 是否允许模型读取内网地址（生产环境建议 false）。",
		"MEDIA_ALLOW_PRIVATE="+orDefault("MEDIA_ALLOW_PRIVATE", "false"),
		"",
		"# ================================ 文件存储 ====================================",
		"# local：服务器本地目录；s3：任意 S3 兼容对象存储。",
		"STORAGE_DRIVER="+orDefault("STORAGE_DRIVER", "local"),
	)
	if storageDriver == "s3" {
		lines = append(lines,
			"# aliyun | tencent | qiniu | aws | minio | custom",
			"STORAGE_S3_PROVIDER="+orDefault("STORAGE_S3_PROVIDER", "custom"),
			"STORAGE_S3_REGION="+value("STORAGE_S3_REGION"),
			"STORAGE_S3_BUCKET="+value("STORAGE_S3_BUCKET"),
			"STORAGE_S3_ACCESS_KEY="+value("STORAGE_S3_ACCESS_KEY"),
			"STORAGE_S3_SECRET_KEY="+value("STORAGE_S3_SECRET_KEY"),
			"# 已知服务商可留空，由服务商与 Region 自动推导；MinIO/自定义必须填写。",
			"STORAGE_S3_ENDPOINT="+value("STORAGE_S3_ENDPOINT"),
			"STORAGE_S3_PREFIX="+value("STORAGE_S3_PREFIX"),
			"STORAGE_S3_FORCE_PATH_STYLE="+orDefault("STORAGE_S3_FORCE_PATH_STYLE", "false"),
			"# 可选 CDN/公网前缀，映射到存储根目录。",
			"STORAGE_PUBLIC_BASE_URL="+value("STORAGE_PUBLIC_BASE_URL"),
		)
	} else {
		lines = append(lines, "STORAGE_LOCAL_DIR="+orDefault("STORAGE_LOCAL_DIR", filepath.Join("data", "uploads")))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func formatEnvValue(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t#\"'\\\r\n") {
		replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
		return `"` + replacer.Replace(value) + `"`
	}
	return value
}

// legacyEnvKeys are the pre-DB_* database settings and the obsolete ADMIN_*
// keys that current deployments no longer need in .env.
var legacyEnvKeys = []string{
	"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD",
	"MYSQL_DATABASE", "MYSQL_CHARSET", "MYSQL_DSN",
	"ADMIN_USERNAME", "ADMIN_PASSWORD",
}

// MigrateLegacyEnvFile rewrites an existing .env that still uses MYSQL_* or
// ADMIN_* keys into the current DB_* format. The original file is kept as
// <path>.bak.<timestamp> and unknown keys are preserved at the end.
func MigrateLegacyEnvFile(cfg Config) (string, error) {
	path := strings.TrimSpace(cfg.EnvFile)
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	values, err := godotenv.Unmarshal(string(raw))
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if !hasLegacyEnvKeys(values) {
		return "", nil
	}

	merged := map[string]string{
		"APP_ENV":                     cfg.AppEnv,
		"APP_TIMEZONE":                fallbackString(cfg.TimeZone, DefaultTimeZone),
		"SERVER_HOST":                 cfg.ServerHost,
		"SERVER_PORT":                 cfg.ServerPort,
		"DB_DRIVER":                   NormalizeDatabaseDriver(cfg.DatabaseDriver),
		"DB_HOST":                     cfg.DatabaseHost,
		"DB_PORT":                     strconv.Itoa(cfg.DatabasePort),
		"DB_USER":                     cfg.DatabaseUser,
		"DB_PASSWORD":                 cfg.DatabasePassword,
		"DB_NAME":                     cfg.DatabaseName,
		"DB_SSLMODE":                  cfg.DatabaseSSLMode,
		"DB_CHARSET":                  cfg.DatabaseCharset,
		"REDIS_ADDR":                  cfg.RedisAddr,
		"REDIS_PASSWORD":              cfg.RedisPassword,
		"REDIS_DB":                    strconv.Itoa(cfg.RedisDB),
		"JWT_SECRET":                  cfg.JWTSecret,
		"MASTER_KEY":                  cfg.MasterKey,
		"SITE_NAME":                   cfg.SiteName,
		"SITE_NAME_EN":                cfg.SiteNameEn,
		"SITE_LOGO":                   cfg.SiteLogo,
		"SITE_URL":                    cfg.SiteURL,
		"CORS_ORIGINS":                strings.Join(cfg.CORSOrigins, ","),
		"WORKER_COUNT":                strconv.Itoa(cfg.WorkerCount),
		"MEDIA_ALLOW_PRIVATE":         strconv.FormatBool(cfg.AllowPrivate),
		"STORAGE_DRIVER":              cfg.StorageDriver,
		"STORAGE_LOCAL_DIR":           cfg.StorageLocalDir,
		"STORAGE_PUBLIC_BASE_URL":     cfg.StoragePublicBaseURL,
		"STORAGE_S3_PROVIDER":         cfg.StorageS3Provider,
		"STORAGE_S3_ENDPOINT":         cfg.StorageS3Endpoint,
		"STORAGE_S3_REGION":           cfg.StorageS3Region,
		"STORAGE_S3_BUCKET":           cfg.StorageS3Bucket,
		"STORAGE_S3_ACCESS_KEY":       cfg.StorageS3AccessKey,
		"STORAGE_S3_SECRET_KEY":       cfg.StorageS3SecretKey,
		"STORAGE_S3_PREFIX":           cfg.StorageS3Prefix,
		"STORAGE_S3_FORCE_PATH_STYLE": strconv.FormatBool(cfg.StorageS3ForcePathStyle),
	}

	// A full DSN can carry options (mTLS, time zone, ...) that the split fields
	// cannot express, so keep it as DB_DSN to stay byte-for-byte equivalent.
	// Split fields are still filled in for readability.
	if dsn := strings.TrimSpace(values["DB_DSN"]); dsn != "" {
		merged["DB_DSN"] = dsn
	} else if legacyDSN := strings.TrimSpace(values["MYSQL_DSN"]); legacyDSN != "" {
		merged["DB_DSN"] = legacyDSN
	}
	if dsn := strings.TrimSpace(merged["DB_DSN"]); dsn != "" {
		fillDatabaseFields(merged, dsn)
	}

	legacy := make(map[string]bool, len(legacyEnvKeys))
	for _, key := range legacyEnvKeys {
		legacy[key] = true
	}
	extras := make([]string, 0)
	for key, value := range values {
		if legacy[key] {
			continue
		}
		if _, known := merged[key]; known {
			continue
		}
		extras = append(extras, key+"="+formatEnvValue(value))
	}
	sort.Strings(extras)

	backup := fmt.Sprintf("%s.bak.%d", path, time.Now().UnixNano())
	if err := os.Rename(path, backup); err != nil {
		return "", fmt.Errorf("备份 %s 失败: %w", path, err)
	}
	content := "# 本文件由旧版 MYSQL_* / ADMIN_* 配置自动转换生成；原文件已备份为 " +
		filepath.Base(backup) + "。\n" + EnvTemplate(merged)
	if len(extras) > 0 {
		content += "\n# ========================== 原有自定义配置 ============================\n" +
			strings.Join(extras, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		_ = os.Rename(backup, path)
		return "", fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return backup, nil
}

func hasLegacyEnvKeys(values map[string]string) bool {
	for _, key := range legacyEnvKeys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func fillDatabaseFields(target map[string]string, dsn string) {
	if strings.HasPrefix(strings.ToLower(dsn), "postgres") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return
		}
		if host, port, err := net.SplitHostPort(parsed.Host); err == nil {
			target["DB_HOST"] = host
			target["DB_PORT"] = port
		}
		if parsed.User != nil {
			target["DB_USER"] = parsed.User.Username()
			if password, ok := parsed.User.Password(); ok {
				target["DB_PASSWORD"] = password
			}
		}
		if name := strings.TrimPrefix(parsed.Path, "/"); name != "" {
			target["DB_NAME"] = name
		}
		if sslmode := parsed.Query().Get("sslmode"); sslmode != "" {
			target["DB_SSLMODE"] = sslmode
		}
		return
	}
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return
	}
	if host, port, splitErr := net.SplitHostPort(parsed.Addr); splitErr == nil {
		target["DB_HOST"] = host
		target["DB_PORT"] = port
	}
	if parsed.User != "" {
		target["DB_USER"] = parsed.User
	}
	if parsed.Passwd != "" {
		target["DB_PASSWORD"] = parsed.Passwd
	}
	if parsed.DBName != "" {
		target["DB_NAME"] = parsed.DBName
	}
	if charset := parsed.Params["charset"]; charset != "" {
		target["DB_CHARSET"] = charset
	}
}

func writeDefaultEnv(path string) error {
	values := map[string]string{
		"APP_ENV":             "production",
		"APP_TIMEZONE":        DefaultTimeZone,
		"SERVER_HOST":         defaultServerHost,
		"SERVER_PORT":         "8080",
		"DB_DRIVER":           DriverMySQL,
		"DB_HOST":             "127.0.0.1",
		"DB_PORT":             "3306",
		"DB_USER":             "root",
		"DB_NAME":             "smart-doc-vision",
		"DB_SSLMODE":          "disable",
		"DB_CHARSET":          "utf8mb4",
		"REDIS_ADDR":          "127.0.0.1:6379",
		"REDIS_DB":            "3",
		"JWT_SECRET":          randomHex(32),
		"MASTER_KEY":          randomHex(32),
		"SITE_NAME":           "智审图档",
		"SITE_NAME_EN":        "SmartDoc Vision",
		"SITE_URL":            "http://127.0.0.1:8080",
		"CORS_ORIGINS":        "http://localhost:5173,http://127.0.0.1:5173",
		"WORKER_COUNT":        "4",
		"MEDIA_ALLOW_PRIVATE": "false",
		"STORAGE_DRIVER":      "local",
		"STORAGE_LOCAL_DIR":   filepath.Join("data", "uploads"),
	}
	return os.WriteFile(path, []byte(EnvTemplate(values)), 0o600)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	return hex.EncodeToString(buf)
}
