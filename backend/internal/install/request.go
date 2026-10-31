package install

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/storage"
)

var (
	databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	adminNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)
)

type MySQL struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Charset  string `json:"charset"`
}

type Redis struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type S3 struct {
	Provider       string `json:"provider"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	Prefix         string `json:"prefix"`
	ForcePathStyle bool   `json:"force_path_style"`
}

type Storage struct {
	Driver        string `json:"driver"`
	LocalDir      string `json:"local_dir"`
	PublicBaseURL string `json:"public_base_url"`
	S3            S3     `json:"s3"`
}

type Admin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Request struct {
	Agreement bool    `json:"agreement"`
	MySQL     MySQL   `json:"mysql"`
	Redis     Redis   `json:"redis"`
	Storage   Storage `json:"storage"`
	Admin     Admin   `json:"admin"`
}

func (r *Request) Normalize() {
	r.MySQL.Host = strings.TrimSpace(r.MySQL.Host)
	r.MySQL.User = strings.TrimSpace(r.MySQL.User)
	r.MySQL.Database = strings.TrimSpace(r.MySQL.Database)
	r.MySQL.Charset = strings.TrimSpace(r.MySQL.Charset)
	if r.MySQL.Port == 0 {
		r.MySQL.Port = 3306
	}
	if r.MySQL.User == "" {
		r.MySQL.User = "root"
	}
	if r.MySQL.Database == "" {
		r.MySQL.Database = "smart-doc-vision"
	}
	if r.MySQL.Charset == "" {
		r.MySQL.Charset = "utf8mb4"
	}

	r.Redis.Host = strings.TrimSpace(r.Redis.Host)
	if r.Redis.Port == 0 {
		r.Redis.Port = 6379
	}

	r.Storage.Driver = strings.ToLower(strings.TrimSpace(r.Storage.Driver))
	if r.Storage.Driver == "" {
		r.Storage.Driver = "local"
	}
	r.Storage.LocalDir = strings.TrimSpace(r.Storage.LocalDir)
	if r.Storage.LocalDir == "" {
		r.Storage.LocalDir = filepath.Join("data", "uploads")
	}
	r.Storage.PublicBaseURL = strings.TrimSpace(r.Storage.PublicBaseURL)
	r.Storage.S3.Provider = strings.ToLower(strings.TrimSpace(r.Storage.S3.Provider))
	r.Storage.S3.Endpoint = strings.TrimSpace(r.Storage.S3.Endpoint)
	r.Storage.S3.Region = strings.TrimSpace(r.Storage.S3.Region)
	r.Storage.S3.Bucket = strings.TrimSpace(r.Storage.S3.Bucket)
	r.Storage.S3.AccessKey = strings.TrimSpace(r.Storage.S3.AccessKey)
	r.Storage.S3.SecretKey = strings.TrimSpace(r.Storage.S3.SecretKey)
	r.Storage.S3.Prefix = strings.Trim(strings.TrimSpace(r.Storage.S3.Prefix), "/")

	r.Admin.Username = strings.TrimSpace(r.Admin.Username)
	if r.Admin.Username == "" {
		r.Admin.Username = "admin"
	}
}

func (r Request) Validate() error {
	if !r.Agreement {
		return fmt.Errorf("请先阅读并同意用户使用协议")
	}
	if !adminNamePattern.MatchString(r.Admin.Username) {
		return fmt.Errorf("管理员账号需为 3~32 位字母、数字、下划线、点或短横线")
	}
	if len(r.Admin.Password) < 8 || len(r.Admin.Password) > 72 {
		return fmt.Errorf("管理员密码长度需为 8~72 位")
	}
	return r.ValidateConnections()
}

// ValidateConnections checks the fields needed before probing MySQL, Redis and
// storage. The test endpoint does not require the agreement or admin account.
func (r Request) ValidateConnections() error {
	if r.MySQL.Host == "" {
		return fmt.Errorf("MySQL 地址不能为空")
	}
	if r.MySQL.Port < 1 || r.MySQL.Port > 65535 {
		return fmt.Errorf("MySQL 端口无效")
	}
	if r.MySQL.User == "" {
		return fmt.Errorf("MySQL 用户名不能为空")
	}
	if !databaseNamePattern.MatchString(r.MySQL.Database) {
		return fmt.Errorf("MySQL 数据库名只能包含字母、数字和下划线")
	}
	if r.Redis.Host == "" {
		return fmt.Errorf("Redis 地址不能为空")
	}
	if r.Redis.Port < 1 || r.Redis.Port > 65535 {
		return fmt.Errorf("Redis 端口无效")
	}
	if r.Redis.DB < 0 || r.Redis.DB > 15 {
		return fmt.Errorf("Redis 数据库编号必须在 0~15 之间")
	}
	switch r.Storage.Driver {
	case "local":
		if r.Storage.LocalDir == "" {
			return fmt.Errorf("本地存储目录不能为空")
		}
	case "s3":
		if r.Storage.S3.Bucket == "" {
			return fmt.Errorf("对象存储 Bucket 不能为空")
		}
		if r.Storage.S3.AccessKey == "" || r.Storage.S3.SecretKey == "" {
			return fmt.Errorf("对象存储 AccessKey / SecretKey 不能为空")
		}
	default:
		return fmt.Errorf("不支持的存储类型 %q", r.Storage.Driver)
	}
	return nil
}

func mysqlDSN(cfg MySQL, includeDatabase bool) string {
	driverCfg := mysqlDriver.NewConfig()
	driverCfg.User = cfg.User
	driverCfg.Passwd = cfg.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if includeDatabase {
		driverCfg.DBName = cfg.Database
	}
	driverCfg.ParseTime = true
	driverCfg.Loc = time.Local
	driverCfg.Params = map[string]string{"charset": cfg.Charset}
	return driverCfg.FormatDSN()
}

// BuildConfig converts the installer form into an in-memory application config.
func BuildConfig(base config.Config, req Request) config.Config {
	req.Normalize()
	base.AppEnv = "production"
	base.MySQLDSN = mysqlDSN(req.MySQL, true)
	base.RedisAddr = net.JoinHostPort(req.Redis.Host, strconv.Itoa(req.Redis.Port))
	base.RedisPassword = req.Redis.Password
	base.RedisDB = req.Redis.DB
	base.AdminUsername = req.Admin.Username
	base.AdminPassword = req.Admin.Password
	base.JWTSecret = config.RandomSecret(32)
	base.MasterKey = config.RandomSecret(32)
	base.Configured = true

	base.StorageDriver = req.Storage.Driver
	if localDir, err := filepath.Abs(req.Storage.LocalDir); err == nil {
		base.StorageLocalDir = localDir
	} else {
		base.StorageLocalDir = req.Storage.LocalDir
	}
	base.StoragePublicBaseURL = req.Storage.PublicBaseURL
	base.StorageS3Provider = req.Storage.S3.Provider
	base.StorageS3Endpoint = req.Storage.S3.Endpoint
	base.StorageS3Region = req.Storage.S3.Region
	base.StorageS3Bucket = req.Storage.S3.Bucket
	base.StorageS3AccessKey = req.Storage.S3.AccessKey
	base.StorageS3SecretKey = req.Storage.S3.SecretKey
	base.StorageS3Prefix = req.Storage.S3.Prefix
	base.StorageS3ForcePathStyle = req.Storage.S3.ForcePathStyle
	base.EnvFile = EnvPath(base)
	return base
}

func StorageConfig(cfg config.Config) storage.Config {
	return storage.Config{
		Driver:           cfg.StorageDriver,
		LocalDir:         cfg.StorageLocalDir,
		PublicBaseURL:    cfg.StoragePublicBaseURL,
		SigningKey:       cfg.MasterKey,
		S3Provider:       cfg.StorageS3Provider,
		S3Endpoint:       cfg.StorageS3Endpoint,
		S3Region:         cfg.StorageS3Region,
		S3Bucket:         cfg.StorageS3Bucket,
		S3AccessKey:      cfg.StorageS3AccessKey,
		S3SecretKey:      cfg.StorageS3SecretKey,
		S3Prefix:         cfg.StorageS3Prefix,
		S3ForcePathStyle: cfg.StorageS3ForcePathStyle,
	}
}

func EnsureDatabase(ctx context.Context, cfg MySQL) error {
	if !databaseNamePattern.MatchString(cfg.Database) {
		return fmt.Errorf("数据库名不合法")
	}
	db, err := sql.Open("mysql", mysqlDSN(cfg, false))
	if err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	statement := "CREATE DATABASE IF NOT EXISTS `" + cfg.Database + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("创建数据库失败: %w", err)
	}
	return nil
}

// WriteEnv serializes the installed configuration. It never overwrites an
// existing .env file.
func WriteEnv(cfg config.Config) (string, error) {
	path := EnvPath(cfg)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("无法确定 .env 路径")
	}
	values := map[string]string{
		"APP_ENV":                     cfg.AppEnv,
		"SERVER_PORT":                 cfg.ServerPort,
		"MYSQL_DSN":                   cfg.MySQLDSN,
		"REDIS_ADDR":                  cfg.RedisAddr,
		"REDIS_PASSWORD":              cfg.RedisPassword,
		"REDIS_DB":                    strconv.Itoa(cfg.RedisDB),
		"JWT_SECRET":                  cfg.JWTSecret,
		"MASTER_KEY":                  cfg.MasterKey,
		"ADMIN_USERNAME":              cfg.AdminUsername,
		"ADMIN_PASSWORD":              cfg.AdminPassword,
		"CORS_ORIGINS":                strings.Join(cfg.CORSOrigins, ","),
		"WORKER_COUNT":                strconv.Itoa(cfg.WorkerCount),
		"MEDIA_ALLOW_PRIVATE":         strconv.FormatBool(cfg.AllowPrivate),
		"DEFAULT_AI_BASE_URL":         cfg.DefaultAIBaseURL,
		"DEFAULT_AI_MODEL":            cfg.DefaultAIModel,
		"DEFAULT_AI_API_KEY":          cfg.DefaultAIAPIKey,
		"DEFAULT_AI_SUPPORTS_VISION":  strconv.FormatBool(cfg.DefaultAISupportsVision),
		"JEV_BASE_URL":                cfg.JEVBaseURL,
		"JEV_MODEL":                   cfg.JEVModel,
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
	body, err := godotenv.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("生成 .env 失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("创建配置目录失败: %w", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		if !config.EnvFileLooksLikeTemplate(path) {
			return "", fmt.Errorf("配置文件 %s 已存在，不会覆盖", path)
		}
		backup := fmt.Sprintf("%s.bak.%d", path, time.Now().UnixNano())
		if renameErr := os.Rename(path, backup); renameErr != nil {
			return "", fmt.Errorf("备份模板 .env 失败: %w", renameErr)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("配置文件 %s 已存在，不会覆盖", path)
		}
		return "", fmt.Errorf("写入 .env 失败: %w", err)
	}
	defer file.Close()
	header := "# SmartDoc Vision configuration generated by the web installer\n" +
		"# Installed at " + time.Now().Format(time.RFC3339) + "\n"
	if _, err := file.WriteString(header + body); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("写入 .env 失败: %w", err)
	}
	return path, nil
}
