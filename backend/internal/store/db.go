package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/schema"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	dialector := gorm.Dialector(mysql.Open(cfg.DatabaseDSN))
	if cfg.DatabaseDriver == config.DriverPostgres {
		dialector = postgres.Open(cfg.DatabaseDSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
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
	return db, nil
}

// Prepare applies the model schema after the caller has explicitly accepted
// the migration (normal startup with a matching schema, installer, or an
// administrator-confirmed upgrade). It never drops application tables.
func Prepare(db *gorm.DB) error {
	if err := schema.Apply(db); err != nil {
		return err
	}
	// AppKey moved to api_credentials; the legacy project column must not keep
	// a unique index or a second project with an empty AppKey cannot be created.
	if err := dropLegacyProjectAppKeyIndex(db); err != nil {
		log.Printf("drop legacy project app_key index: %v", err)
	}
	if err := cleanupLegacyData(db); err != nil {
		log.Printf("cleanup legacy data: %v", err)
	}
	return nil
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
