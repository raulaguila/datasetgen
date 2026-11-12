package infra

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"
	"github.com/raulaguila/dataset/pkg/logger"
)

type Resources struct {
	PostgresDB *gorm.DB
}

func (r *Resources) Shutdown(log *logger.Logger) {
	if r.PostgresDB != nil {
		if sqlDB, err := r.PostgresDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil && log != nil {
				log.Warn("Failed to close PostgreSQL connection", slog.String("error", err.Error()))
			}
		}
	}

	if log != nil {
		log.Info("Infrastructure connections closed")
	}
}

func New(cfg *config.Environment, log *logger.Logger) (*Resources, error) {
	log.Info("Connecting to PostgreSQL...")
	postgresDB, err := postgres.Connect(&postgres.Config{URL: cfg.Postgres.URL, MaxConns: cfg.Postgres.MaxConns})
	if err != nil {
		return nil, err
	}
	log.Info("PostgreSQL connected", slog.String("host", cfg.Postgres.Host), slog.String("database", cfg.Postgres.Base))

	return &Resources{PostgresDB: postgresDB}, nil
}
