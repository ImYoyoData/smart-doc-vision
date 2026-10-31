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

	"smartdoc-vision/backend/internal/api"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/install"
	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/metrics"
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

type bootstrapOptions struct {
	forceAdmin bool
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
		var err error
		app, err = bootstrap(cfg, bootstrapOptions{})
		if err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
		switcher.Set(app.handler)
		if _, found := install.FindMarker(); !found {
			marker := install.Marker{
				Path:    install.MarkerPathForConfig(cfg),
				Version: appversion.Version,
				Storage: cfg.StorageDriver,
				Admin:   cfg.AdminUsername,
				Source:  "existing-config",
			}
			if err := install.WriteMarker(marker.Path, marker); err != nil {
				log.Printf("install marker: %v", err)
			} else {
				log.Printf("install marker: created %s", marker.Path)
			}
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
	if installer != nil {
		installer.Close()
	} else if app != nil {
		app.close()
	}
	log.Println("stopped")
}

func installApplication(ctx context.Context, base config.Config, req install.Request) (http.Handler, func(), error) {
	cfg := install.BuildConfig(base, req)
	if err := install.EnsureDatabase(ctx, req.MySQL); err != nil {
		return nil, nil, err
	}
	app, err := bootstrap(cfg, bootstrapOptions{forceAdmin: true})
	if err != nil {
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

func bootstrap(cfg config.Config, opts bootstrapOptions) (*appRuntime, error) {
	cipher, err := security.NewCipher(cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(cfg)
	if err != nil {
		return nil, err
	}
	cache, err := store.OpenRedis(cfg)
	if err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}

	closeAll := func() {
		_ = cache.Close()
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	}

	if opts.forceAdmin {
		if err := store.EnsureAdmin(db, cfg.AdminUsername, cfg.AdminPassword); err != nil {
			closeAll()
			return nil, err
		}
	}
	seeder := store.Seeder{DB: db, Cipher: cipher, Cfg: cfg}
	if err := seeder.Run(); err != nil {
		closeAll()
		return nil, err
	}

	fileStore, err := storage.New(install.StorageConfig(cfg))
	if err != nil {
		closeAll()
		return nil, err
	}
	loader := media.NewLoader(media.NewFetcher(cfg.AllowPrivate), fileStore)

	eng := engine.New(db, cache, cipher, cfg, loader)
	eng.Start()

	tasks := service.NewTaskService(db, cache, eng, cfg.AllowPrivate, loader)
	triggerManager := trigger.NewManager(db, cipher, tasks)
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
		closeAll()
	}
	return &appRuntime{handler: server.Router(), close: closeRuntime}, nil
}
