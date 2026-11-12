package output

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// JobRepository persists sys.job.
type JobRepository interface {
	// Enqueue adds a job due now. Call it through a UnitOfWork, so the job
	// commits or rolls back with the change it follows.
	Enqueue(ctx context.Context, kind string, payload []byte) error
	// Claim leases due jobs of the given kinds until now+lease; a crashed
	// worker's jobs come back after that. Kinds this worker cannot run stay
	// queued for one that can, so a rolling deploy never fails a new kind.
	Claim(ctx context.Context, kinds []string, limit int, now time.Time, lease time.Duration) ([]domain.Job, error)
	MarkDone(ctx context.Context, id, claimID string, processedAt time.Time) error
	// MarkFailed records a failed run: retried at availableAt, or finished when terminal.
	MarkFailed(ctx context.Context, id, claimID string, attempts int, availableAt time.Time, lastError string, terminal bool) error
}
