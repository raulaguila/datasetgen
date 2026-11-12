package postgres

import (
	"context"

	"gorm.io/gorm"
)

// HealthChecker performs low-cost liveness checks against PostgreSQL.
type HealthChecker struct {
	db *gorm.DB
}

func NewHealthChecker(db *gorm.DB) *HealthChecker {
	return &HealthChecker{db: db}
}

func (h *HealthChecker) Ping(ctx context.Context) error {
	if h == nil || h.db == nil {
		return gorm.ErrInvalidDB
	}

	sqlDB, err := h.db.DB()
	if err != nil {
		return err
	}

	return sqlDB.PingContext(ctx)
}
