package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smartdoc-vision/backend/internal/api"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/metrics"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
	"smartdoc-vision/backend/internal/store"
	"smartdoc-vision/backend/internal/trigger"
	"smartdoc-vision/backend/internal/webui"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	cfg := config.Load()
	if cfg.ConfigFile != "" {
		log.Printf("config: using %s", cfg.ConfigFile)
	}
	if cfg.EnvFile != "" {
		log.Printf("config: using %s", cfg.EnvFile)
	}

	cipher, err := security.NewCipher(cfg.MasterKey)
	if err != nil {
		log.Fatalf("init cipher: %v", err)
	}
	db, err := store.Open(cfg)
	if err != nil {
		log.Fatalf("init mysql: %v", err)
	}
	cache, err := store.OpenRedis(cfg)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	defer cache.Close()

	seeder := store.Seeder{DB: db, Cipher: cipher, Cfg: cfg}
	if err := seeder.Run(); err != nil {
		log.Fatalf("seed: %v", err)
	}

	eng := engine.New(db, cache, cipher, cfg)
	eng.Start()
	defer eng.Stop()

	tasks := service.NewTaskService(db, cache, eng, cfg.AllowPrivate)
	triggerManager := trigger.NewManager(db, cipher, tasks)
	triggerManager.Start()
	defer triggerManager.Stop()

	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	retentionService := service.NewRetentionService(db)
	go retentionService.Run(retentionCtx)
	defer retentionCancel()

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	metrics.StartSampler(monitorCtx)
	defer monitorCancel()

	server := api.NewServer(cfg, db, cache, cipher, eng, tasks)
	httpServer := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      180 * time.Second,
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
	eng.Stop()
	log.Println("stopped")
}
