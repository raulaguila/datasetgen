package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// HealthChecker is the inbound port for readiness/liveness aggregation.
type HealthChecker interface {
	Check(ctx context.Context) (*domain.HealthReport, error)
}
