package outbox

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

var _ output.OutboxRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ClaimID     uuid.UUID  `gorm:"column:claim_id"`
	RequestID   string     `gorm:"column:request_id"`
	ID          uuid.UUID  `gorm:"column:id"`
	Purpose     string     `gorm:"column:purpose"`
	Payload     []byte     `gorm:"column:payload"`
	Status      string     `gorm:"column:status"`
	Attempts    int        `gorm:"column:attempts"`
	AvailableAt time.Time  `gorm:"column:available_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
	LastError   *string    `gorm:"column:last_error"`
}

func (m row) toDomain() domain.OutboxMessage {
	return domain.OutboxMessage{
		ID:          m.ID.String(),
		ClaimID:     m.ClaimID.String(),
		RequestID:   m.RequestID,
		Purpose:     m.Purpose,
		Payload:     m.Payload,
		Status:      m.Status,
		Attempts:    m.Attempts,
		AvailableAt: m.AvailableAt,
		CreatedAt:   m.CreatedAt,
		ProcessedAt: m.ProcessedAt,
		LastError:   m.LastError,
	}
}

func (r *Repository) Enqueue(ctx context.Context, purpose string, payload []byte) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO sys.outbox (id, purpose, payload, request_id)
		VALUES (?, ?, ?::jsonb, NULLIF(?, ''))
	`, uuid.New(), purpose, string(payload), reqctx.RequestID(ctx)).Error
	if err != nil {
		return shared.WrapError(err, "outbox")
	}
	return nil
}

func (r *Repository) ClaimPending(ctx context.Context, limit int, now time.Time, lease time.Duration) ([]domain.OutboxMessage, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		WITH cte AS (
			SELECT id
			FROM sys.outbox
			WHERE status IN ('pending', 'processing') AND available_at <= ?
			ORDER BY available_at
			FOR UPDATE SKIP LOCKED
			LIMIT ?
		)
		UPDATE sys.outbox o
		SET status = 'processing', available_at = ?, claim_id = ?
		FROM cte
		WHERE o.id = cte.id
		RETURNING o.id, o.claim_id, COALESCE(o.request_id, '') AS request_id, o.purpose, o.payload, o.status, o.attempts, o.available_at, o.created_at, o.processed_at, o.last_error
	`, now, limit, now.Add(lease), uuid.New()).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "outbox")
	}
	out := make([]domain.OutboxMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toDomain())
	}
	return out, nil
}

func (r *Repository) MarkSent(ctx context.Context, id, claimID string, processedAt time.Time) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE sys.outbox
		SET status = 'sent', processed_at = ?, last_error = NULL, payload = payload - 'token'
		WHERE id = ? AND claim_id = ?::uuid AND status = 'processing'
	`, processedAt, id, claimID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "outbox")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "outbox")
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, id, claimID string, attempts int, availableAt time.Time, processedAt *time.Time, lastError string, terminal bool) error {
	status := domain.OutboxStatusPending
	if terminal {
		status = domain.OutboxStatusFailed
	}
	res := r.db.WithContext(ctx).Exec(`
		UPDATE sys.outbox
		SET status = ?, attempts = ?, available_at = ?, processed_at = ?, last_error = ?,
		    payload = CASE WHEN ? THEN payload - 'token' ELSE payload END
		WHERE id = ? AND claim_id = ?::uuid AND status = 'processing'
	`, status, attempts, availableAt, processedAt, lastError, terminal, id, claimID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "outbox")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "outbox")
	}
	return nil
}
