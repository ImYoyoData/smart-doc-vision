package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

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

	DefaultAIBaseURL        string
	DefaultAIModel          string
	DefaultAIAPIKey         string
	DefaultAISupportsVision bool

	JEVBaseURL string
	JEVModel   string
}

func Load() Config {
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
		}
	}

	cfg := Config{
		AppEnv:                  env("APP_ENV", "development"),
		ServerPort:              env("SERVER_PORT", "8080"),
		MySQLDSN:                env("MYSQL_DSN", "root:your_password@tcp(127.0.0.1:3306)/smart-doc-vision?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:               env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:           env("REDIS_PASSWORD", ""),
		RedisDB:                 envInt("REDIS_DB", 3),
		JWTSecret:               env("JWT_SECRET", "dev-smartdoc-vision-change-me"),
		MasterKey:               env("MASTER_KEY", "dev-smartdoc-vision-master-key-change-me"),
		AdminUsername:           env("ADMIN_USERNAME", "admin"),
		AdminPassword:           env("ADMIN_PASSWORD", "Admin@123456"),
		CORSOrigins:             splitCSV(env("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		WorkerCount:             envInt("WORKER_COUNT", 4),
		AllowPrivate:            envBool("MEDIA_ALLOW_PRIVATE", false),
		DefaultAIBaseURL:        env("DEFAULT_AI_BASE_URL", "https://api.openai.com/v1"),
		DefaultAIModel:          env("DEFAULT_AI_MODEL", "gpt-4o-mini"),
		DefaultAIAPIKey:         env("DEFAULT_AI_API_KEY", ""),
		DefaultAISupportsVision: envBool("DEFAULT_AI_SUPPORTS_VISION", false),
		JEVBaseURL:              env("JEV_BASE_URL", "https://api.typesafe.ai/v1"),
		JEVModel:                env("JEV_MODEL", "jev-latest"),
	}

	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 1
	}
	if cfg.WorkerCount > 32 {
		cfg.WorkerCount = 32
	}
	cfg.MySQLDSN = ensureMySQLCharset(cfg.MySQLDSN)
	return cfg
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

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
