package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/api"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/install"
	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/metrics"
	appschema "smartdoc-vision/backend/internal/schema"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
	"smartdoc-vision/backend/internal/storage"
	"smartdoc-vision/backend/internal/store"
	"smartdoc-vision/backend/internal/trigger"
	appversion "smartdoc-vision/backend/internal/version"
	"smartdoc-vision/backend/internal/webui"
)

type handlerSwitcher struct {
	mu      sync.RWMutex
	handler http.Handler
}

func (s *handlerSwitcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	handler := s.handler
	s.mu.RUnlock()
	if handler == nil {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
}

func (s *handlerSwitcher) Set(handler http.Handler) {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
}

type appRuntime struct {
	handler http.Handler
	close   func()
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	cfg := config.Load()
	if cfg.ConfigFile != "" {
		log.Printf("config: using %s", cfg.ConfigFile)
	}
	if cfg.EnvFile != "" {
		log.Printf("config: using %s", cfg.EnvFile)
	}

	state := install.Detect(cfg)
	switcher := &handlerSwitcher{}
	var installer *api.InstallerServer
	var schemaServer *api.SchemaServer
	var app *appRuntime

	switch state {
	case install.StateNeedsInstall:
		log.Printf("installer: no configuration found; web installer is enabled")
		installer = api.NewInstallerServer(cfg, state, func(ctx context.Context, req install.Request) (http.Handler, func(), error) {
			return installApplication(ctx, cfg, req)
		}, switcher.Set)
		switcher.Set(installer.Router())
	case install.StateConfigMissing:
		log.Printf("installer: .install.txt exists but no configuration was found; refusing to reinstall")
		installer = api.NewInstallerServer(cfg, state, nil, nil)
		switcher.Set(installer.Router())
	default:
		db, cache, cipher, err := connectInfra(cfg)
		if err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
		report, err := appschema.Inspect(db)
		if err != nil {
			closeInfra(db, cache)
			log.Fatalf("inspect schema: %v", err)
		}
		if report.Required {
			log.Printf("schema: database structure requires an administrator-confirmed upgrade (version %s, db %s)",
				report.SchemaVersion, report.DatabaseVersion)
			if report.MissingCore {
				log.Printf("schema: core tables are missing; online login/repair is unavailable until the database is restored")
			}
			schemaServer = api.NewSchemaServer(cfg, db, cache, cipher, report,
				func(ctx context.Context) (http.Handler, func(), error) {
					return prepareAndBuild(ctx, cfg, db, cache, cipher)
				}, switcher.Set)
			switcher.Set(schemaServer.Router())
		} else {
			if err := store.Prepare(db); err != nil {
				closeInfra(db, cache)
				log.Fatalf("prepare schema: %v", err)
			}
			if err := seedApp(cfg, db, cipher); err != nil {
				closeInfra(db, cache)
				log.Fatalf("seed: %v", err)
			}
			app, err = buildApp(cfg, db, cache, cipher)
			if err != nil {
				closeInfra(db, cache)
				log.Fatalf("bootstrap: %v", err)
			}
			switcher.Set(app.handler)
			ensureInstallMarker(cfg)
		}
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           switcher,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("SmartDoc Vision API listening on http://127.0.0.1:%s (Redis DB %d)", cfg.ServerPort, cfg.RedisDB)
		if webui.Available() {
			log.Printf("Admin UI: http://127.0.0.1:%s/admin", cfg.ServerPort)
		} else {
			log.Printf("Admin UI is not embedded; run build.ps1 to bundle it")
		}
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	switch {
	case schemaServer != nil:
		schemaServer.Close()
	case installer != nil:
		installer.Close()
	case app != nil:
		app.close()
	}
	log.Println("stopped")
}

func installApplication(ctx context.Context, base config.Config, req install.Request) (http.Handler, func(), error) {
	cfg := install.BuildConfig(base, req)
	if err := install.EnsureDatabase(ctx, req.MySQL); err != nil {
		return nil, nil, err
	}
	db, cache, cipher, err := connectInfra(cfg)
	if err != nil {
		return nil, nil, err
	}
	report, err := appschema.Inspect(db)
	if err != nil {
		closeInfra(db, cache)
		return nil, nil, err
	}
	mode := req.DatabaseMode
	if mode == "" {
		if report.Existing {
			closeInfra(db, cache)
			return nil, nil, errors.New("数据库已有业务表，请选择校验修复或清空重建")
		}
		mode = "create"
	}
	if report.Existing && mode == "create" {
		closeInfra(db, cache)
		return nil, nil, errors.New("数据库已有业务表，不能直接重新创建")
	}
	if mode == "reset" {
		if !req.ConfirmReset {
			closeInfra(db, cache)
			return nil, nil, errors.New("清空数据库需要二次确认")
		}
		if err := appschema.Reset(db); err != nil {
			closeInfra(db, cache)
			return nil, nil, err
		}
	}
	if mode == "repair" && !req.ConfirmRepair {
		closeInfra(db, cache)
		return nil, nil, errors.New("修复数据库结构需要二次确认")
	}
	if err := store.Prepare(db); err != nil {
		closeInfra(db, cache)
		return nil, nil, err
	}
	if err := store.EnsureAdmin(db, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		closeInfra(db, cache)
		return nil, nil, err
	}
	if err := seedApp(cfg, db, cipher); err != nil {
		closeInfra(db, cache)
		return nil, nil, err
	}
	app, err := buildApp(cfg, db, cache, cipher)
	if err != nil {
		closeInfra(db, cache)
		return nil, nil, err
	}
	envPath, err := install.WriteEnv(cfg)
	if err != nil {
		app.close()
		return nil, nil, err
	}
	cfg.EnvFile = envPath
	marker := install.Marker{
		Path:    install.MarkerPathForConfig(cfg),
		Version: appversion.Version,
		Storage: cfg.StorageDriver,
		Admin:   cfg.AdminUsername,
		Source:  "web-installer",
	}
	if err := install.WriteMarker(marker.Path, marker); err != nil {
		app.close()
		return nil, nil, err
	}
	log.Printf("installer: installation complete; config written to %s", envPath)
	return app.handler, app.close, nil
}

// prepareAndBuild is used after an administrator confirms a schema upgrade.
// It deliberately leaves the database connection open when a step fails so the
// administrator can inspect the error and retry.
func prepareAndBuild(ctx context.Context, cfg config.Config, db *gorm.DB, cache *redis.Client, cipher *security.Cipher) (http.Handler, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := store.Prepare(db); err != nil {
		return nil, nil, err
	}
	if err := seedApp(cfg, db, cipher); err != nil {
		return nil, nil, err
	}
	app, err := buildApp(cfg, db, cache, cipher)
	if err != nil {
		return nil, nil, err
	}
	ensureInstallMarker(cfg)
	return app.handler, app.close, nil
}

func connectInfra(cfg config.Config) (*gorm.DB, *redis.Client, *security.Cipher, error) {
	cipher, err := security.NewCipher(cfg.MasterKey)
	if err != nil {
		return nil, nil, nil, err
	}
	db, err := store.Open(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	cache, err := store.OpenRedis(cfg)
	if err != nil {
		closeInfra(db, nil)
		return nil, nil, nil, err
	}
	return db, cache, cipher, nil
}

func closeInfra(db *gorm.DB, cache *redis.Client) {
	if cache != nil {
		_ = cache.Close()
	}
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func seedApp(cfg config.Config, db *gorm.DB, cipher *security.Cipher) error {
	seeder := store.Seeder{DB: db, Cipher: cipher, Cfg: cfg}
	return seeder.Run()
}

func ensureInstallMarker(cfg config.Config) {
	if _, found := install.FindMarker(); found {
		return
	}
	marker := install.Marker{
		Path:    install.MarkerPathForConfig(cfg),
		Version: appversion.Version,
		Storage: cfg.StorageDriver,
		Admin:   cfg.AdminUsername,
		Source:  "existing-config",
	}
	if err := install.WriteMarker(marker.Path, marker); err != nil {
		log.Printf("install marker: %v", err)
		return
	}
	log.Printf("install marker: created %s", marker.Path)
}

func buildApp(cfg config.Config, db *gorm.DB, cache *redis.Client, cipher *security.Cipher) (*appRuntime, error) {
	fileStore, err := storage.New(install.StorageConfig(cfg))
	if err != nil {
		return nil, err
	}
	loader := media.NewLoader(media.NewFetcher(cfg.AllowPrivate), fileStore)

	eng := engine.New(db, cache, cipher, cfg, loader)
	eng.Start()

	tasks := service.NewTaskService(db, cache, eng, cfg.AllowPrivate, loader)
	triggerManager := trigger.NewManager(db, cipher, tasks, cfg.AllowPrivate)
	triggerManager.Start()

	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	retentionService := service.NewRetentionService(db)
	go retentionService.Run(retentionCtx)

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	metrics.AttachRedis(cache)
	metrics.StartSampler(monitorCtx)

	server := api.NewServer(cfg, db, cache, cipher, eng, tasks, loader)
	server.RecoverInterruptedTreaties()

	closeRuntime := func() {
		triggerManager.Stop()
		eng.Stop()
		retentionCancel()
		monitorCancel()
		closeInfra(db, cache)
	}
	return &appRuntime{handler: server.Router(), close: closeRuntime}, nil
}
