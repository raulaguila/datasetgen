package postgres

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds database configuration
type Config struct {
	URL      string
	MaxConns int
}

// Connect establishes a connection to the PostgreSQL database
func Connect(cfg *Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: time.Now,
		// No PrepareStmt: its cache has no size limit, and every IN list length is
		// a new statement on every connection. pgx already caches 512 per connection.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxConns)
	sqlDB.SetMaxIdleConns(cfg.MaxConns)

	// Each idle connection holds a Postgres backend process: hand it back after a peak.
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	// SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}
