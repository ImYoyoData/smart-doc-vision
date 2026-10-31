package install

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/schema"
	"smartdoc-vision/backend/internal/storage"
)

var (
	databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,63}$`)
	adminNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)
)

// Database describes how to reach the application database. Driver is
// mysql (default) or postgres; Charset only applies to MySQL and SSLMode only
// applies to PostgreSQL.
type Database struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Charset  string `json:"charset"`
	SSLMode  string `json:"sslmode"`
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

type Site struct {
	Name   string `json:"name"`
	NameEn string `json:"name_en"`
	Logo   string `json:"logo"`
}

type Request struct {
	Agreement bool     `json:"agreement"`
	Database  Database `json:"database"`
	Redis     Redis    `json:"redis"`
	Storage   Storage  `json:"storage"`
	Admin     Admin    `json:"admin"`
	Site      Site     `json:"site"`
	// DatabaseMode is create, repair or reset. The installer requires an
	// explicit choice when application tables already exist.
	DatabaseMode  string `json:"database_mode"`
	ConfirmReset  bool   `json:"confirm_reset"`
	ConfirmRepair bool   `json:"confirm_repair"`
}

func (r *Request) Normalize() {
	r.Database.Driver = config.NormalizeDatabaseDriver(r.Database.Driver)
	r.Database.Host = strings.TrimSpace(r.Database.Host)
	r.Database.User = strings.TrimSpace(r.Database.User)
	r.Database.Database = strings.TrimSpace(r.Database.Database)
	r.Database.Charset = strings.TrimSpace(r.Database.Charset)
	r.Database.SSLMode = strings.ToLower(strings.TrimSpace(r.Database.SSLMode))
	if r.Database.Driver == config.DriverPostgres {
		if r.Database.Port == 0 {
			r.Database.Port = 5432
		}
		if r.Database.User == "" {
			r.Database.User = "postgres"
		}
		r.Database.Charset = ""
		if r.Database.SSLMode == "" {
			r.Database.SSLMode = "disable"
		}
	} else {
		if r.Database.Port == 0 {
			r.Database.Port = 3306
		}
		if r.Database.User == "" {
			r.Database.User = "root"
		}
		r.Database.SSLMode = ""
		if r.Database.Charset == "" {
			r.Database.Charset = "utf8mb4"
		}
	}
	if r.Database.Database == "" {
		r.Database.Database = "smart-doc-vision"
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
	r.DatabaseMode = strings.ToLower(strings.TrimSpace(r.DatabaseMode))

	r.Admin.Username = strings.TrimSpace(r.Admin.Username)
	if r.Admin.Username == "" {
		r.Admin.Username = "admin"
	}

	r.Site.Name = strings.TrimSpace(r.Site.Name)
	r.Site.NameEn = strings.TrimSpace(r.Site.NameEn)
	r.Site.Logo = strings.TrimSpace(r.Site.Logo)
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
	switch r.DatabaseMode {
	case "", "create", "repair", "reset":
	default:
		return fmt.Errorf("数据库处理方式无效")
	}
	if r.DatabaseMode == "reset" && !r.ConfirmReset {
		return fmt.Errorf("清空数据库需要二次确认")
	}
	if r.DatabaseMode == "repair" && !r.ConfirmRepair {
		return fmt.Errorf("修复数据库结构需要二次确认")
	}
	if err := r.ValidateSite(); err != nil {
		return err
	}
	return r.ValidateConnections()
}

func (r Request) ValidateSite() error {
	if len([]rune(r.Site.Name)) > 64 {
		return fmt.Errorf("网站名称不能超过 64 个字符")
	}
	if len([]rune(r.Site.NameEn)) > 64 {
		return fmt.Errorf("英文名称不能超过 64 个字符")
	}
	if len(r.Site.Logo) > 1024 {
		return fmt.Errorf("Logo 地址过长")
	}
	return nil
}

// ValidateConnections checks the fields needed before probing MySQL, Redis and
// storage. The test endpoint does not require the agreement or admin account.
func (r Request) ValidateConnections() error {
	if r.Database.Driver != config.DriverMySQL && r.Database.Driver != config.DriverPostgres {
		return fmt.Errorf("数据库类型需为 mysql 或 postgres")
	}
	if r.Database.Host == "" {
		return fmt.Errorf("数据库地址不能为空")
	}
	if r.Database.Port < 1 || r.Database.Port > 65535 {
		return fmt.Errorf("数据库端口无效")
	}
	if r.Database.User == "" {
		return fmt.Errorf("数据库用户名不能为空")
	}
	if !databaseNamePattern.MatchString(r.Database.Database) {
		return fmt.Errorf("数据库名只能包含字母、数字、下划线或短横线，且不超过 63 个字符")
	}
	if r.Database.Driver == config.DriverPostgres {
		switch r.Database.SSLMode {
		case "", "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		default:
			return fmt.Errorf("PostgreSQL SSL 模式无效")
		}
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

func databaseDSN(cfg Database, includeDatabase bool) string {
	if cfg.Driver == config.DriverPostgres {
		return postgresDSN(cfg, includeDatabase)
	}
	return mysqlDSN(cfg, includeDatabase)
}

func mysqlDSN(cfg Database, includeDatabase bool) string {
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

func postgresDSN(cfg Database, includeDatabase bool) string {
	target := url.URL{Scheme: "postgres", Path: "/postgres"}
	if includeDatabase {
		target.Path = "/" + cfg.Database
	}
	if cfg.User != "" {
		target.User = url.UserPassword(cfg.User, cfg.Password)
	}
	target.Host = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	query := url.Values{}
	sslmode := strings.TrimSpace(cfg.SSLMode)
	if sslmode == "" {
		sslmode = "disable"
	}
	query.Set("sslmode", sslmode)
	query.Set("connect_timeout", "10")
	target.RawQuery = query.Encode()
	return target.String()
}

// BuildConfig converts the installer form into an in-memory application config.
func BuildConfig(base config.Config, req Request) config.Config {
	req.Normalize()
	base.AppEnv = "production"
	base.DatabaseDriver = req.Database.Driver
	base.DatabaseDSN = databaseDSN(req.Database, true)
	base.DatabaseHost = req.Database.Host
	base.DatabasePort = req.Database.Port
	base.DatabaseUser = req.Database.User
	base.DatabasePassword = req.Database.Password
	base.DatabaseName = req.Database.Database
	base.DatabaseCharset = req.Database.Charset
	base.DatabaseSSLMode = req.Database.SSLMode
	base.RedisAddr = net.JoinHostPort(req.Redis.Host, strconv.Itoa(req.Redis.Port))
	base.RedisPassword = req.Redis.Password
	base.RedisDB = req.Redis.DB
	base.AdminUsername = req.Admin.Username
	base.AdminPassword = req.Admin.Password
	base.SiteName = req.Site.Name
	if base.SiteName == "" {
		base.SiteName = "智审图档"
	}
	base.SiteNameEn = req.Site.NameEn
	if base.SiteNameEn == "" {
		base.SiteNameEn = "SmartDoc Vision"
	}
	base.SiteLogo = req.Site.Logo
	// A repair or reinstall against an existing database must keep the original
	// secrets: MASTER_KEY decrypts every stored API key and channel secret.
	if !base.Configured {
		base.JWTSecret = config.RandomSecret(32)
		base.MasterKey = config.RandomSecret(32)
	}
	if strings.TrimSpace(base.JWTSecret) == "" {
		base.JWTSecret = config.RandomSecret(32)
	}
	if strings.TrimSpace(base.MasterKey) == "" {
		base.MasterKey = config.RandomSecret(32)
	}
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

func EnsureDatabase(ctx context.Context, cfg Database) error {
	if !databaseNamePattern.MatchString(cfg.Database) {
		return fmt.Errorf("数据库名不合法")
	}
	if cfg.Driver == config.DriverPostgres {
		return ensurePostgresDatabase(ctx, cfg)
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

func ensurePostgresDatabase(ctx context.Context, cfg Database) error {
	db, err := sql.Open("pgx", postgresDSN(cfg, false))
	if err != nil {
		return fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.Database).Scan(&exists); err != nil {
		return fmt.Errorf("检测数据库是否存在失败: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE "`+cfg.Database+`" ENCODING 'UTF8'`); err != nil {
		return fmt.Errorf("创建数据库失败: %w", err)
	}
	return nil
}

// InspectDatabase connects to the target database and reports structural
// differences without modifying anything.
func InspectDatabase(ctx context.Context, cfg Database) (schema.Report, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dialector := gorm.Dialector(mysql.Open(mysqlDSN(cfg, true)))
	if cfg.Driver == config.DriverPostgres {
		dialector = postgres.Open(postgresDSN(cfg, true))
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:  gormlogger.Default.LogMode(gormlogger.Silent),
		NowFunc: func() time.Time { return time.Now().Local() },
	})
	if err != nil {
		if unknownDatabaseError(err) {
			return schema.FreshReport(), nil
		}
		return schema.Report{}, fmt.Errorf("连接 %s 失败: %w", databaseLabel(cfg.Driver), err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return schema.Report{}, err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(2)
	if err := sqlDB.PingContext(timeoutCtx); err != nil {
		if unknownDatabaseError(err) {
			return schema.FreshReport(), nil
		}
		return schema.Report{}, fmt.Errorf("连接 %s 失败: %w", databaseLabel(cfg.Driver), err)
	}
	report, err := schema.Inspect(db.WithContext(timeoutCtx))
	if err != nil {
		if unknownDatabaseError(err) {
			return schema.FreshReport(), nil
		}
		return report, err
	}
	return report, nil
}

func unknownDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "3D000" {
		return true
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unknown database") || strings.Contains(value, "error 1049") ||
		(strings.Contains(value, "database") && strings.Contains(value, "does not exist"))
}

func databaseLabel(driver string) string {
	if driver == config.DriverPostgres {
		return "PostgreSQL"
	}
	return "MySQL"
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
		"DB_DRIVER":                   cfg.DatabaseDriver,
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
	body := config.EnvTemplate(values)
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
	header := "# SmartDoc Vision 配置文件（由安装向导生成）\n" +
		"# 安装时间 " + time.Now().Format(time.RFC3339) + "\n"
	if _, err := file.WriteString(header + body); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("写入 .env 失败: %w", err)
	}
	return path, nil
}
