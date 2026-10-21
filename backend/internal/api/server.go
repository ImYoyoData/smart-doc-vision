package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
	"smartdoc-vision/backend/internal/webui"
)

type Server struct {
	cfg    config.Config
	db     *gorm.DB
	redis  *redis.Client
	cipher *security.Cipher
	tokens *security.TokenManager
	engine *engine.Engine
	tasks  *service.TaskService
}

func NewServer(cfg config.Config, db *gorm.DB, cache *redis.Client, cipher *security.Cipher, eng *engine.Engine, tasks *service.TaskService) *Server {
	return &Server{
		cfg:    cfg,
		db:     db,
		redis:  cache,
		cipher: cipher,
		tokens: security.NewTokenManager(cfg.JWTSecret),
		engine: eng,
		tasks:  tasks,
	}
}

func (s *Server) registerAdminUI(r *gin.Engine) {
	handler, ok := webui.New()
	if !ok {
		message := "管理界面尚未打包。请先运行 build.ps1（或 npm run build）生成前端资源。"
		r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "SmartDoc Vision API\n"+message+"\n") })
		r.GET("/admin", func(c *gin.Context) { c.String(http.StatusServiceUnavailable, message) })
		r.Any("/admin/*filepath", func(c *gin.Context) { c.String(http.StatusServiceUnavailable, message) })
		return
	}
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/admin/") })
	r.GET("/admin", func(c *gin.Context) { c.Redirect(http.StatusFound, "/admin/") })
	r.Any("/admin/*filepath", gin.WrapH(handler))
}

func (s *Server) Router() *gin.Engine {
	if s.cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), traceMiddleware())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     s.cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Trace-Id", "Idempotency-Key"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	r.GET("/health", s.health)
	s.registerAdminUI(r)

	open := r.Group("/open/v1")
	{
		open.POST("/projects/:projectCode/audits/:auditCode/submit", s.openSubmit)
		open.GET("/projects/:projectCode/tasks/:taskNo", s.openTask)
		open.GET("/ws/projects/:projectCode/audits/:auditCode", s.openWS)
	}

	admin := r.Group("/api/admin/v1")
	{
		admin.POST("/auth/login", s.login)
		admin.POST("/auth/refresh", s.refresh)
		auth := admin.Group("")
		auth.Use(s.authRequired())
		{
			auth.GET("/auth/me", s.me)
			auth.POST("/auth/logout", func(c *gin.Context) { ok(c, gin.H{"logged_out": true}) })
			auth.POST("/auth/change-password", s.changePassword)

			auth.GET("/projects", s.listProjects)
			auth.POST("/projects", s.createProject)
			auth.GET("/projects/:id", s.getProject)
			auth.PUT("/projects/:id", s.updateProject)
			auth.DELETE("/projects/:id", s.deleteProject)
			auth.POST("/projects/:id/restore", s.restoreProject)
			auth.PATCH("/projects/:id/status", s.updateProjectStatus)
			auth.GET("/projects/:id/credentials", s.listCredentials)
			auth.POST("/projects/:id/credentials", s.createCredential)
			auth.PUT("/projects/:id/credentials/:credentialId", s.updateCredential)
			auth.POST("/projects/:id/credentials/:credentialId/rotate", s.rotateCredential)
			auth.DELETE("/projects/:id/credentials/:credentialId", s.deleteCredential)

			auth.GET("/providers", s.listProviders)
			auth.POST("/providers", s.createProvider)
			auth.GET("/providers/:id", s.getProvider)
			auth.PUT("/providers/:id", s.updateProvider)
			auth.DELETE("/providers/:id", s.deleteProvider)
			auth.POST("/providers/:id/test", s.testProvider)

			auth.GET("/audits", s.listAudits)
			auth.POST("/audits", s.createAudit)
			auth.GET("/audits/:id", s.getAudit)
			auth.PUT("/audits/:id", s.updateAudit)
			auth.DELETE("/audits/:id", s.deleteAudit)
			auth.POST("/audits/:id/restore", s.restoreAudit)
			auth.POST("/audits/:id/clone", s.cloneAudit)
			auth.PATCH("/audits/:id/status", s.updateAuditStatus)
			auth.GET("/audits/:id/integration", s.integrationGuide)
			auth.GET("/audits/:id/export", s.exportAuditTasks)
			auth.GET("/reviews/summary", s.reviewSummary)
			auth.PUT("/trigger-sources/:id", s.updateTrigger)
			auth.POST("/trigger-sources/:id/test", s.testTrigger)

			auth.GET("/tasks", s.listTasks)
			auth.POST("/tasks", s.submitTask)
			auth.GET("/tasks/:id", s.getTask)
			auth.POST("/tasks/:id/retry", s.retryTask)
			auth.POST("/tasks/:id/review", s.reviewTask)
			auth.POST("/tasks/batch-retry", s.batchRetryTasks)

			auth.GET("/dashboard", s.dashboard)
			auth.GET("/analytics", s.analytics)
			auth.GET("/audit-logs", s.listAuditLogs)
			auth.GET("/recycle-bin", s.listRecycleBin)
			auth.GET("/meta/rule-templates", s.ruleTemplates)
			auth.GET("/meta/roles", s.roleDefinitions)
			auth.GET("/meta/system", s.systemInfo)
			auth.GET("/users", s.listUsers)
			auth.POST("/users", s.createUser)
			auth.PUT("/users/:id", s.updateUser)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		fail(c, http.StatusNotFound, "NOT_FOUND", "接口不存在", nil)
	})
	return r
}

func traceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Set("trace_id", traceID)
		c.Set("started_at", time.Now())
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

func (s *Server) health(c *gin.Context) {
	dbStatus := "ok"
	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		dbStatus = "down"
	}
	redisStatus := "ok"
	if err := s.redis.Ping(c.Request.Context()).Err(); err != nil {
		redisStatus = "down"
	}
	status := http.StatusOK
	if dbStatus != "ok" {
		status = http.StatusServiceUnavailable
	}
	failOrOk(c, status, gin.H{
		"status": "ok", "mysql": dbStatus, "redis": redisStatus,
		"redis_db": s.cfg.RedisDB, "time": time.Now().Format(time.RFC3339),
	})
}

func failOrOk(c *gin.Context, status int, data gin.H) {
	if status != http.StatusOK {
		c.JSON(status, apiResponse{Code: "DEGRADED", Message: "dependency unavailable", Data: data, TraceID: c.GetString("trace_id")})
		return
	}
	ok(c, data)
}

func (s *Server) systemInfo(c *gin.Context) {
	ok(c, gin.H{
		"name":         "智审图档",
		"name_en":      "SmartDoc Vision",
		"version":      "1.0.0",
		"redis_db":     s.cfg.RedisDB,
		"worker_count": s.cfg.WorkerCount,
		"provider_kinds": []gin.H{
			{"key": model.ProviderOpenAI, "name": "通用大模型（OpenAI 兼容）", "supports_images": "按模型配置"},
			{"key": model.ProviderAnthropic, "name": "Anthropic Claude（Messages API）", "supports_images": "按模型配置"},
			{"key": model.ProviderJEV, "name": "JEV / TypeSafe System One", "supports_images": "原生支持"},
		},
	})
}

func (s *Server) ruleTemplates(c *gin.Context) {
	ok(c, gin.H{
		"templates":               catalog.RuleTemplates(),
		"default_review_prompt":   "根据档案内容安全、完整性与敏感信息规范进行审核；无法确认时转人工复核。",
		"default_classify_prompt": catalog.DefaultClassifyPrompt,
		"example_categories":      catalog.ExampleClassificationCategories(),
	})
}

func (s *Server) writeAudit(c *gin.Context, projectID *uint64, action, objectType, objectID, result string, detail any) {
	user := currentUser(c)
	log := model.AuditLog{
		ProjectID:  projectID,
		Operator:   user.Username,
		OperatorIP: c.ClientIP(),
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Result:     result,
		DetailJSON: model.JSONBytes(detail),
		TraceID:    c.GetString("trace_id"),
	}
	if user.ID != 0 {
		uid := user.ID
		log.OperatorID = &uid
	}
	if err := s.db.Create(&log).Error; err != nil {
		// Never block the business request solely because audit persistence failed.
		_ = err
	}
}

func (s *Server) createAuditLogNoReq(projectID *uint64, username, action, objectType, objectID, result string, detail any, traceID string) {
	log := model.AuditLog{
		ProjectID: projectID, Operator: username, Action: action, ObjectType: objectType,
		ObjectID: objectID, Result: result, DetailJSON: model.JSONBytes(detail), TraceID: traceID,
	}
	_ = s.db.Create(&log).Error
}

func parseUint(s string) (uint64, bool) {
	var v uint64
	if strings.TrimSpace(s) == "" {
		return 0, false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + uint64(ch-'0')
	}
	return v, true
}
