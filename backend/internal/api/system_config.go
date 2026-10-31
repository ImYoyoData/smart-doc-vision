package api

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/security"
	appversion "smartdoc-vision/backend/internal/version"
)

type systemConfigField struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Sensitive  bool   `json:"sensitive"`
	Configured bool   `json:"configured"`
}

type systemConfigGroup struct {
	Key         string              `json:"key"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Fields      []systemConfigField `json:"fields"`
}

func (s *Server) systemConfig(c *gin.Context) {
	if !s.hasPermission(currentUser(c).Role, "system:config") {
		forbidden(c, "当前角色无权查看系统配置")
		return
	}
	ok(c, gin.H{
		"version":     appversion.Version,
		"env_file":    s.cfg.EnvFile,
		"config_file": s.cfg.ConfigFile,
		"groups":      s.systemConfigGroups(),
	})
}

func (s *Server) systemConfigGroups() []systemConfigGroup {
	cfg := s.cfg
	return []systemConfigGroup{
		{
			Key: "runtime", Title: "运行环境", Description: "进程启动时解析出的基础运行参数。",
			Fields: []systemConfigField{
				plainConfigField("APP_ENV", "运行环境", cfg.AppEnv),
				plainConfigField("SERVER_PORT", "服务端口", cfg.ServerPort),
				plainConfigField("WORKER_COUNT", "审核 Worker 数", fmt.Sprint(cfg.WorkerCount)),
				plainConfigField("MEDIA_ALLOW_PRIVATE", "允许私有网络地址", yesNo(cfg.AllowPrivate)),
				plainConfigField("CORS_ORIGINS", "允许的 CORS 来源", strings.Join(cfg.CORSOrigins, ", ")),
			},
		},
		{
			Key: "database", Title: "数据库", Description: "主数据库连接（MySQL 或 PostgreSQL）；密码在服务端脱敏后返回。",
			Fields: databaseConfigFields(cfg),
		},
		{
			Key: "ai", Title: "默认模型", Description: "未在模型接入中单独配置时使用的默认 AI 参数。",
			Fields: []systemConfigField{
				plainConfigField("DEFAULT_AI_BASE_URL", "默认 Base URL", cfg.DefaultAIBaseURL),
				plainConfigField("DEFAULT_AI_MODEL", "默认模型", cfg.DefaultAIModel),
				secretConfigField("DEFAULT_AI_API_KEY", "默认 API Key", cfg.DefaultAIAPIKey),
				plainConfigField("DEFAULT_AI_SUPPORTS_VISION", "默认支持图片", yesNo(cfg.DefaultAISupportsVision)),
				plainConfigField("JEV_BASE_URL", "JEV Base URL", cfg.JEVBaseURL),
				plainConfigField("JEV_MODEL", "JEV 模型", cfg.JEVModel),
			},
		},
		{
			Key: "security", Title: "安全与密钥", Description: "登录令牌与数据加密密钥；密钥只显示脱敏值。后台账号保存在数据库中，请在「账号管理」中维护。",
			Fields: []systemConfigField{
				secretConfigField("JWT_SECRET", "JWT 签名密钥", cfg.JWTSecret),
				secretConfigField("MASTER_KEY", "数据加密主密钥", cfg.MasterKey),
			},
		},
		{
			Key: "redis", Title: "Redis", Description: "平台缓存与队列连接；密码已脱敏。",
			Fields: []systemConfigField{
				plainConfigField("REDIS_ADDR", "连接地址", cfg.RedisAddr),
				plainConfigField("REDIS_DB", "数据库编号", fmt.Sprint(cfg.RedisDB)),
				secretConfigField("REDIS_PASSWORD", "访问密码", cfg.RedisPassword),
			},
		},
		{
			Key: "storage", Title: "文件存储", Description: "上传图片与文件的外部存储位置；对象存储密钥已脱敏。",
			Fields: storageConfigFields(cfg),
		},
	}
}

func storageConfigFields(cfg config.Config) []systemConfigField {
	driver := strings.ToLower(strings.TrimSpace(cfg.StorageDriver))
	label := "本地文件存储"
	if driver == "s3" {
		label = "对象存储（S3 兼容）"
	}
	fields := []systemConfigField{
		plainConfigField("STORAGE_DRIVER", "存储方式", label),
		plainConfigField("STORAGE_PUBLIC_BASE_URL", "公开访问前缀", cfg.StoragePublicBaseURL),
	}
	if driver == "s3" {
		provider := strings.TrimSpace(cfg.StorageS3Provider)
		endpoint := strings.TrimSpace(cfg.StorageS3Endpoint)
		if endpoint == "" {
			endpoint = "按服务商默认端点"
		}
		fields = append(fields,
			plainConfigField("STORAGE_S3_PROVIDER", "服务商", provider),
			plainConfigField("STORAGE_S3_ENDPOINT", "Endpoint", endpoint),
			plainConfigField("STORAGE_S3_REGION", "Region", cfg.StorageS3Region),
			plainConfigField("STORAGE_S3_BUCKET", "Bucket", cfg.StorageS3Bucket),
			plainConfigField("STORAGE_S3_PREFIX", "路径前缀", cfg.StorageS3Prefix),
			secretConfigField("STORAGE_S3_ACCESS_KEY", "AccessKey", cfg.StorageS3AccessKey),
			secretConfigField("STORAGE_S3_SECRET_KEY", "SecretKey", cfg.StorageS3SecretKey),
			plainConfigField("STORAGE_S3_FORCE_PATH_STYLE", "强制 Path-Style", yesNo(cfg.StorageS3ForcePathStyle)),
		)
	} else {
		fields = append(fields, plainConfigField("STORAGE_LOCAL_DIR", "本地目录", cfg.StorageLocalDir))
	}
	return fields
}

func databaseConfigFields(cfg config.Config) []systemConfigField {
	driver := config.NormalizeDatabaseDriver(cfg.DatabaseDriver)
	label := "MySQL"
	if driver == config.DriverPostgres {
		label = "PostgreSQL"
	}
	fields := []systemConfigField{plainConfigField("DB_DRIVER", "类型", label)}
	dsn := strings.TrimSpace(cfg.DatabaseDSN)
	if dsn == "" {
		return append(fields, secretConfigField("DB_DSN", "连接 DSN", ""))
	}
	if driver == config.DriverPostgres {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return append(fields, secretConfigField("DB_DSN", "连接 DSN", dsn))
		}
		username, password := "", ""
		if parsed.User != nil {
			username = parsed.User.Username()
			password, _ = parsed.User.Password()
		}
		return append(fields,
			plainConfigField("DB_HOST", "地址", parsed.Host),
			plainConfigField("DB_NAME", "数据库", strings.TrimPrefix(parsed.Path, "/")),
			plainConfigField("DB_USER", "用户名", username),
			secretConfigField("DB_PASSWORD", "密码", password),
			plainConfigField("DB_SSLMODE", "SSL 模式", parsed.Query().Get("sslmode")),
		)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return append(fields, secretConfigField("DB_DSN", "连接 DSN", dsn))
	}
	return append(fields,
		plainConfigField("DB_HOST", "地址", parsed.Addr),
		plainConfigField("DB_NAME", "数据库", parsed.DBName),
		plainConfigField("DB_USER", "用户名", parsed.User),
		secretConfigField("DB_PASSWORD", "密码", parsed.Passwd),
		plainConfigField("DB_CHARSET", "字符集", parsed.Params["charset"]),
	)
}

func plainConfigField(key, label, value string) systemConfigField {
	value = strings.TrimSpace(value)
	if value == "" {
		return systemConfigField{Key: key, Label: label, Value: "未配置"}
	}
	return systemConfigField{Key: key, Label: label, Value: value, Configured: true}
}

func secretConfigField(key, label, value string) systemConfigField {
	value = strings.TrimSpace(value)
	if value == "" {
		return systemConfigField{Key: key, Label: label, Value: "未配置", Sensitive: true}
	}
	return systemConfigField{
		Key: key, Label: label, Value: security.MaskSecret(value),
		Sensitive: true, Configured: true,
	}
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}
