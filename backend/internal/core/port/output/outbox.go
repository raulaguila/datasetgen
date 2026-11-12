package output

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// OutboxRepository persists sys.outbox.
type OutboxRepository interface {
	Enqueue(ctx context.Context, purpose string, payload []byte) error
	// ClaimPending leases due rows until now+lease; a crashed worker's rows come back after that.
	ClaimPending(ctx context.Context, limit int, now time.Time, lease time.Duration) ([]domain.OutboxMessage, error)
	MarkSent(ctx context.Context, id, claimID string, processedAt time.Time) error
	MarkFailed(ctx context.Context, id, claimID string, attempts int, availableAt time.Time, processedAt *time.Time, lastError string, terminal bool) error
}
