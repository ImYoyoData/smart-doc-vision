package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/install"
	appversion "smartdoc-vision/backend/internal/version"
)

// installStatus is the public endpoint the SPA calls before authentication to
// decide whether to show the login page or the installer.
func (s *Server) installStatus(c *gin.Context) {
	state := install.Detect(s.cfg)
	payload := gin.H{
		"state":   state,
		"version": appversion.Version,
	}
	if marker, okMarker := install.FindMarker(); okMarker {
		if !marker.InstalledAt.IsZero() {
			payload["installed_at"] = marker.InstalledAt.Format("2006-01-02 15:04:05")
		}
		if marker.Version != "" {
			payload["version"] = marker.Version
		}
	}
	ok(c, payload)
}

// InstallHandler boots the real application after the installer has written
// its configuration. The returned handler replaces the installer router and the
// close function tears the application down on shutdown.
type InstallHandler func(ctx context.Context, req install.Request) (http.Handler, func(), error)

// InstallerServer serves only the installer API and the SPA while the system
// has no configuration or database.
type InstallerServer struct {
	cfg       config.Config
	state     install.State
	onInstall InstallHandler
	onReady   func(http.Handler)

	mu        sync.Mutex
	installed bool
	appClose  func()
}

func NewInstallerServer(cfg config.Config, state install.State, onInstall InstallHandler, onReady func(http.Handler)) *InstallerServer {
	return &InstallerServer{cfg: cfg, state: state, onInstall: onInstall, onReady: onReady}
}

func (s *InstallerServer) Close() {
	s.mu.Lock()
	closeFn := s.appClose
	s.appClose = nil
	s.mu.Unlock()
	if closeFn != nil {
		closeFn()
	}
}

func (s *InstallerServer) Router() *gin.Engine {
	if s.cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), traceMiddleware(), bodyLimitMiddleware(8<<20))
	corsCfg := cors.Config{
		AllowOrigins:     s.cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Trace-Id"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	if s.cfg.AppEnv == "development" {
		configured := append([]string(nil), s.cfg.CORSOrigins...)
		corsCfg.AllowOrigins = nil
		corsCfg.AllowOriginFunc = func(origin string) bool {
			return originInList(origin, configured) || isDevFrontendOrigin(origin)
		}
	}
	r.Use(cors.New(corsCfg))

	registerAdminUI(r)
	r.GET("/health", func(c *gin.Context) {
		ok(c, gin.H{"status": "installing", "state": s.state, "version": appversion.Version})
	})
	r.GET("/api/install/status", s.status)
	r.GET("/api/site", func(c *gin.Context) { ok(c, siteInfoPayload(s.cfg)) })
	r.GET("/api/site/logo", func(c *gin.Context) { serveSiteLogo(s.cfg, c) })
	r.POST("/api/install/test", s.test)
	r.POST("/api/install", s.install)
	return r
}

func (s *InstallerServer) status(c *gin.Context) {
	ok(c, install.BuildStatus(s.cfg, s.state))
}

func (s *InstallerServer) test(c *gin.Context) {
	if s.state != install.StateNeedsInstall {
		fail(c, http.StatusForbidden, "INSTALL_DISABLED", "当前状态不允许执行安装检测", nil)
		return
	}
	var req install.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Normalize()
	if err := req.ValidateConnections(); err != nil {
		badRequest(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	results := install.TestAll(ctx, s.cfg, req)
	payload := gin.H{"ok": install.AllOK(results), "results": results}
	if install.AllOK(results) {
		if report, err := install.InspectDatabase(ctx, req.MySQL); err != nil {
			payload["ok"] = false
			payload["database_error"] = "数据库结构检测失败: " + err.Error()
		} else {
			payload["database"] = report
		}
	}
	ok(c, payload)
}

func (s *InstallerServer) install(c *gin.Context) {
	if s.state != install.StateNeedsInstall {
		fail(c, http.StatusForbidden, "INSTALL_DISABLED", "系统已经安装或配置文件缺失，不能重新执行安装", nil)
		return
	}
	if s.onInstall == nil {
		fail(c, http.StatusServiceUnavailable, "INSTALL_UNAVAILABLE", "安装服务未就绪", nil)
		return
	}
	if _, found := install.FindMarker(); found {
		fail(c, http.StatusConflict, "ALREADY_INSTALLED", "系统已经安装，不能重复执行安装流程", nil)
		return
	}
	var req install.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		badRequest(c, err.Error())
		return
	}

	inspectCtx, inspectCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	report, inspectErr := install.InspectDatabase(inspectCtx, req.MySQL)
	inspectCancel()
	if inspectErr != nil {
		fail(c, http.StatusUnprocessableEntity, "DATABASE_INSPECT_FAILED", "数据库结构检测失败: "+inspectErr.Error(), nil)
		return
	}
	if req.DatabaseMode == "" {
		if report.Existing {
			fail(c, http.StatusConflict, "DATABASE_DECISION_REQUIRED",
				"检测到数据库已有业务表，请选择清空重建或校验修复", report)
			return
		}
		req.DatabaseMode = "create"
	}
	if report.Existing && req.DatabaseMode == "create" {
		fail(c, http.StatusConflict, "DATABASE_DECISION_REQUIRED",
			"数据库已有业务表，不能直接重新创建；请选择校验修复或清空重建", report)
		return
	}
	if req.DatabaseMode == "reset" && !req.ConfirmReset {
		badRequest(c, "清空数据库需要二次确认")
		return
	}

	s.mu.Lock()
	if s.installed {
		s.mu.Unlock()
		fail(c, http.StatusConflict, "ALREADY_INSTALLED", "安装正在进行或已完成", nil)
		return
	}
	s.installed = true
	s.mu.Unlock()
	reset := func() {
		s.mu.Lock()
		s.installed = false
		s.mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	results := install.TestAll(ctx, s.cfg, req)
	if !install.AllOK(results) {
		reset()
		fail(c, http.StatusUnprocessableEntity, "INSTALL_CHECK_FAILED", "环境检测未通过，请根据每项提示修正后重试", gin.H{"results": results})
		return
	}
	handler, closeFn, err := s.onInstall(ctx, req)
	if err != nil {
		reset()
		fail(c, http.StatusInternalServerError, "INSTALL_FAILED", "安装失败: "+err.Error(), gin.H{"results": results})
		return
	}
	s.mu.Lock()
	s.appClose = closeFn
	s.mu.Unlock()
	if s.onReady != nil {
		s.onReady(handler)
	}
	installedAt := time.Now()
	marker, _ := install.FindMarker()
	if !marker.InstalledAt.IsZero() {
		installedAt = marker.InstalledAt
	}
	ok(c, gin.H{
		"installed":    true,
		"installed_at": installedAt.Format("2006-01-02 15:04:05"),
		"version":      appversion.Version,
		"env_file":     install.EnvPath(s.cfg),
	})
}
