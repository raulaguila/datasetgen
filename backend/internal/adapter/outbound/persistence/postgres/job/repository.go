package job

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

type Repository struct {
	db *gorm.DB
}

var _ output.JobRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID          uuid.UUID `gorm:"column:id"`
	ClaimID     uuid.UUID `gorm:"column:claim_id"`
	RequestID   string    `gorm:"column:request_id"`
	Kind        string    `gorm:"column:kind"`
	Payload     []byte    `gorm:"column:payload"`
	Attempts    int       `gorm:"column:attempts"`
	MaxAttempts int       `gorm:"column:max_attempts"`
}

func (r *Repository) Enqueue(ctx context.Context, kind string, payload []byte) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO sys.job (id, kind, payload, request_id)
		VALUES (?, ?, ?::jsonb, NULLIF(?, ''))
	`, uuid.New(), kind, string(payload), reqctx.RequestID(ctx)).Error
	return shared.WrapError(err, "job")
}

func (r *Repository) Claim(ctx context.Context, kinds []string, limit int, now time.Time, lease time.Duration) ([]domain.Job, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		WITH cte AS (
			SELECT id
			FROM sys.job
			WHERE status IN ('pending', 'processing') AND available_at <= ? AND kind IN ?
			ORDER BY available_at
			FOR UPDATE SKIP LOCKED
			LIMIT ?
		)
		UPDATE sys.job j
		SET status = 'processing', available_at = ?, claim_id = ?
		FROM cte
		WHERE j.id = cte.id
		RETURNING j.id, j.claim_id, COALESCE(j.request_id, '') AS request_id, j.kind, j.payload, j.attempts, j.max_attempts
	`, now, kinds, limit, now.Add(lease), uuid.New()).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "job")
	}
	out := make([]domain.Job, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.Job{
			ID: m.ID.String(), ClaimID: m.ClaimID.String(), RequestID: m.RequestID,
			Kind: m.Kind, Payload: m.Payload, Attempts: m.Attempts, MaxAttempts: m.MaxAttempts,
		})
	}
	return out, nil
}

func (r *Repository) MarkDone(ctx context.Context, id, claimID string, processedAt time.Time) error {
	return marked(r.db.WithContext(ctx).Exec(`
		UPDATE sys.job SET status = 'done', processed_at = ?, last_error = NULL
		WHERE id = ? AND claim_id = ?::uuid AND status = 'processing'
	`, processedAt, id, claimID))
}

func (r *Repository) MarkFailed(ctx context.Context, id, claimID string, attempts int, availableAt time.Time, lastError string, terminal bool) error {
	return marked(r.db.WithContext(ctx).Exec(`
		UPDATE sys.job
		SET status = CASE WHEN ? THEN 'failed' ELSE 'pending' END,
		    attempts = ?, available_at = ?, last_error = ?,
		    processed_at = CASE WHEN ? THEN now() END
		WHERE id = ? AND claim_id = ?::uuid AND status = 'processing'
	`, terminal, attempts, availableAt, lastError, terminal, id, claimID))
}

// marked reports a lost lease (another worker took the job) as not found.
func marked(res *gorm.DB) error {
	if res.Error != nil {
		return shared.WrapError(res.Error, "job")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "job")
	}
	return nil
}
