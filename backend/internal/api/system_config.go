package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"

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
			Key: "mysql", Title: "MySQL", Description: "主数据库连接；密码在服务端脱敏后返回。",
			Fields: mysqlConfigFields(cfg.MySQLDSN),
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
			Key: "security", Title: "安全与初始账号", Description: "JWT、加密主密钥和初始化管理员；密钥只显示脱敏值。",
			Fields: []systemConfigField{
				secretConfigField("JWT_SECRET", "JWT 签名密钥", cfg.JWTSecret),
				secretConfigField("MASTER_KEY", "数据加密主密钥", cfg.MasterKey),
				plainConfigField("ADMIN_USERNAME", "初始管理员账号", cfg.AdminUsername),
				secretConfigField("ADMIN_PASSWORD", "初始管理员密码", cfg.AdminPassword),
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
	}
}

func mysqlConfigFields(dsn string) []systemConfigField {
	if strings.TrimSpace(dsn) == "" {
		return []systemConfigField{plainConfigField("MYSQL_DSN", "连接 DSN", "")}
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return []systemConfigField{secretConfigField("MYSQL_DSN", "连接 DSN", dsn)}
	}
	charset := parsed.Params["charset"]
	return []systemConfigField{
		plainConfigField("MYSQL_HOST", "地址", parsed.Addr),
		plainConfigField("MYSQL_DATABASE", "数据库", parsed.DBName),
		plainConfigField("MYSQL_USER", "用户名", parsed.User),
		secretConfigField("MYSQL_PASSWORD", "密码", parsed.Passwd),
		plainConfigField("MYSQL_CHARSET", "字符集", charset),
	}
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
