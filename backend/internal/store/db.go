package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{
		Logger:  gormlogger.Default.LogMode(gormlogger.Warn),
		NowFunc: func() time.Time { return time.Now().Local() },
	})
	if err != nil {
		return nil, fmt.Errorf("connect mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := ping(sqlDB); err != nil {
		return nil, err
	}
	// Keep every table UTF-8 even when the target database was created with a
	// legacy default charset. Use a dedicated migration session so query state
	// from AutoMigrate never leaks into the shared runtime connection.
	migrateDB := db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	if err := migrateDB.AutoMigrate(
		&model.User{},
		&model.ProjectMember{},
		&model.UserAuditScope{},
		&model.Project{},
		&model.APICredential{},
		&model.AIProvider{},
		&model.Audit{},
		&model.TriggerSource{},
		&model.Task{},
		&model.TaskEvent{},
		&model.AuditLog{},
		&model.Role{},
		&model.ProjectSecret{},
		&model.ScriptExecution{},
		&model.RedisConnection{},
		&model.ProviderUsageLog{},
		&model.NotificationChannel{},
		&model.InAppMessage{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// AppKey moved to api_credentials; the legacy project column must not keep
	// a unique index or a second project with an empty AppKey cannot be created.
	if err := dropLegacyProjectAppKeyIndex(migrateDB); err != nil {
		log.Printf("drop legacy project app_key index: %v", err)
	}
	if err := cleanupLegacyData(db); err != nil {
		log.Printf("cleanup legacy data: %v", err)
	}
	return db, nil
}

func dropLegacyProjectAppKeyIndex(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasIndex(&model.Project{}, "idx_projects_app_key") {
		return nil
	}
	return migrator.DropIndex(&model.Project{}, "idx_projects_app_key")
}

func ping(db *sql.DB) error {
	var lastErr error
	for i := 0; i < 20; i++ {
		if err := db.Ping(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		log.Printf("waiting for mysql (%d/20): %v", i+1, lastErr)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("ping mysql: %w", lastErr)
}
