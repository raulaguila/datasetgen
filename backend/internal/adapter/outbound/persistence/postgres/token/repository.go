package token

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.TokenRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID        uuid.UUID  `gorm:"column:id"`
	UserID    uuid.UUID  `gorm:"column:user_id"`
	Purpose   string     `gorm:"column:purpose"`
	TokenHash []byte     `gorm:"column:token_hash"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (m row) toDomain() *domain.EmailToken {
	return &domain.EmailToken{
		ID:        m.ID.String(),
		UserID:    m.UserID.String(),
		Purpose:   m.Purpose,
		TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt,
		UsedAt:    m.UsedAt,
		CreatedAt: m.CreatedAt,
	}
}

func (r *Repository) Create(ctx context.Context, t *domain.EmailToken) error {
	id := uuid.New()
	now := t.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_token (id, user_id, purpose, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, t.UserID, t.Purpose, t.TokenHash, t.ExpiresAt, now).Error
	if err != nil {
		return shared.WrapError(err, "token")
	}
	t.ID = id.String()
	t.CreatedAt = now
	return nil
}

func (r *Repository) FindActiveByHash(ctx context.Context, purpose string, hash []byte) (*domain.EmailToken, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_token").
		Where("purpose = ? AND token_hash = ?", purpose, hash).
		Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "token")
	}
	return m.toDomain(), nil
}

func (r *Repository) MarkUsed(ctx context.Context, id string, usedAt time.Time) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_token SET used_at = ? WHERE id = ? AND used_at IS NULL
	`, usedAt, id)
	if res.Error != nil {
		return shared.WrapError(res.Error, "token")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "token")
	}
	return nil
}

func (r *Repository) IssuedSince(ctx context.Context, userID, purpose string, since time.Time) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("usr.usr_token").
		Where("user_id = ? AND purpose = ? AND used_at IS NULL AND created_at >= ?", userID, purpose, since).
		Count(&n).Error
	if err != nil {
		return false, shared.WrapError(err, "token")
	}
	return n > 0, nil
}

func (r *Repository) CountIssuedSince(ctx context.Context, userID, purpose string, since time.Time) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("usr.usr_token").
		Where("user_id = ? AND purpose = ? AND created_at >= ?", userID, purpose, since).
		Count(&n).Error
	if err != nil {
		return 0, shared.WrapError(err, "token")
	}
	return int(n), nil
}

func (r *Repository) InvalidateActive(ctx context.Context, userID, purpose string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_token SET used_at = now()
		WHERE user_id = ? AND purpose = ? AND used_at IS NULL
	`, userID, purpose).Error
	if err != nil {
		return shared.WrapError(err, "token")
	}
	return nil
}
