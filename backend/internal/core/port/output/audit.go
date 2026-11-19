package output

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// AuditRepository appends to sys.audit_log. Call it inside the transaction
// of the change, so both commit or roll back together.
type AuditRepository interface {
	Record(ctx context.Context, entry domain.AuditEntry) error
	List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error)
}
