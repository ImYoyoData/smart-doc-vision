package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
	appschema "smartdoc-vision/backend/internal/schema"
	"smartdoc-vision/backend/internal/security"
)

// schemaStatus is the normal-mode response for the SPA compatibility check.
func (s *Server) schemaStatus(c *gin.Context) {
	ok(c, gin.H{
		"required":  false,
		"can_apply": false,
		"report": gin.H{
			"required":       false,
			"schema_version": appschema.Version,
		},
	})
}

// SchemaApplyFunc applies an administrator-confirmed schema upgrade and
// returns the full application handler that replaces the migration router.
type SchemaApplyFunc func(ctx context.Context) (http.Handler, func(), error)

// SchemaServer runs a minimal login + schema API while the database structure
// differs from the current program. Normal business APIs are unavailable.
type SchemaServer struct {
	srv     *Server
	report  appschema.Report
	onApply SchemaApplyFunc
	onReady func(http.Handler)

	mu          sync.Mutex
	applying    bool
	appClose    func()
	transferred bool
}

func NewSchemaServer(cfg config.Config, db *gorm.DB, cache *redis.Client, cipher *security.Cipher, report appschema.Report, onApply SchemaApplyFunc, onReady func(http.Handler)) *SchemaServer {
	srv := NewServer(cfg, db, cache, cipher, nil, nil, nil)
	srv.schemaGate = true
	return &SchemaServer{srv: srv, report: report, onApply: onApply, onReady: onReady}
}

func (s *SchemaServer) Close() {
	s.mu.Lock()
	closeFn := s.appClose
	transferred := s.transferred
	s.appClose = nil
	s.mu.Unlock()
	if closeFn != nil {
		closeFn()
		return
	}
	if transferred {
		return
	}
	if s.srv.redis != nil {
		_ = s.srv.redis.Close()
	}
	if s.srv.db != nil {
		if sqlDB, err := s.srv.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (s *SchemaServer) Router() *gin.Engine {
	cfg := s.srv.cfg
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), traceMiddleware(), bodyLimitMiddleware(8<<20))
	corsCfg := cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Trace-Id"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	if cfg.AppEnv == "development" {
		configured := append([]string(nil), cfg.CORSOrigins...)
		corsCfg.AllowOrigins = nil
		corsCfg.AllowOriginFunc = func(origin string) bool {
			return originInList(origin, configured) || isDevFrontendOrigin(origin)
		}
	}
	r.Use(cors.New(corsCfg))
	registerAdminUI(r)
	r.GET("/health", s.srv.health)
	r.GET("/api/install/status", s.srv.installStatus)
	r.GET("/api/site", func(c *gin.Context) { ok(c, siteInfoPayload(s.srv.cfg)) })
	r.GET("/api/site/logo", func(c *gin.Context) { serveSiteLogo(s.srv.cfg, c) })

	admin := r.Group("/api/admin/v1")
	{
		admin.GET("/auth/captcha", s.srv.captchaChallenge)
		admin.POST("/auth/login", s.srv.login)
		admin.POST("/auth/login/mfa", s.srv.loginWithMFA)
		admin.POST("/auth/refresh", s.srv.refresh)
		admin.POST("/auth/mfa", s.srv.verifyMFA)
		auth := admin.Group("")
		auth.Use(s.srv.authRequired())
		{
			auth.GET("/auth/me", s.srv.me)
			auth.POST("/auth/logout", s.srv.logout)
			auth.GET("/system/schema/status", s.status)
			auth.POST("/system/schema/apply", s.apply)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/open/") {
			fail(c, http.StatusServiceUnavailable, "SCHEMA_UPGRADE_REQUIRED",
				"数据库结构需要兼容升级，请由超级管理员确认后继续", nil)
			return
		}
		c.Status(http.StatusNotFound)
	})
	return r
}

func (s *SchemaServer) status(c *gin.Context) {
	ok(c, gin.H{
		"required":  true,
		"can_apply": !s.report.MissingCore && s.report.HasSuperAdmin,
		"report":    s.report,
	})
}

func (s *SchemaServer) apply(c *gin.Context) {
	user := currentUser(c)
	if user.Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可以执行数据库兼容升级")
		return
	}
	if s.report.MissingCore || !s.report.HasSuperAdmin {
		fail(c, http.StatusConflict, "SCHEMA_CORE_MISSING",
			"核心用户表或超级管理员账号缺失，无法在线修复；请恢复数据库后重启", nil)
		return
	}
	s.mu.Lock()
	if s.applying {
		s.mu.Unlock()
		fail(c, http.StatusConflict, "SCHEMA_APPLY_IN_PROGRESS", "兼容升级正在执行，请勿重复提交", nil)
		return
	}
	s.applying = true
	s.mu.Unlock()
	reset := func() {
		s.mu.Lock()
		s.applying = false
		s.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	handler, closeFn, err := s.onApply(ctx)
	if err != nil {
		reset()
		fail(c, http.StatusInternalServerError, "SCHEMA_APPLY_FAILED", "兼容升级失败: "+err.Error(), nil)
		return
	}
	s.mu.Lock()
	s.appClose = closeFn
	s.transferred = true
	s.applying = false
	s.mu.Unlock()
	if s.onReady != nil {
		s.onReady(handler)
	}
	ok(c, gin.H{"applied": true, "schema_version": appschema.Version})
}
