package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// AuditService reads the audit log.
type AuditService interface {
	List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error)
}
