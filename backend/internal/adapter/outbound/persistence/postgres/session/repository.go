package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/clientip"
)

type Repository struct {
	db *gorm.DB
}

var _ output.SessionRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID                       uuid.UUID  `gorm:"column:id"`
	UserID                   uuid.UUID  `gorm:"column:user_id"`
	ExpiresAt                time.Time  `gorm:"column:expires_at"`
	RevokedAt                *time.Time `gorm:"column:revoked_at"`
	RevocationReason         *string    `gorm:"column:revocation_reason"`
	RefreshTokenHash         []byte     `gorm:"column:refresh_token_hash"`
	PreviousRefreshTokenHash []byte     `gorm:"column:previous_refresh_token_hash"`
	IPAddress                *string    `gorm:"column:ip_address"`
	UserAgent                *string    `gorm:"column:user_agent"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	LastSeenAt               time.Time  `gorm:"column:last_seen_at"`
}

func (m row) toDomain() *domain.Session {
	return &domain.Session{
		ID:                       m.ID.String(),
		UserID:                   m.UserID.String(),
		ExpiresAt:                m.ExpiresAt,
		RevokedAt:                m.RevokedAt,
		RevocationReason:         m.RevocationReason,
		RefreshTokenHash:         m.RefreshTokenHash,
		PreviousRefreshTokenHash: m.PreviousRefreshTokenHash,
		IPAddress:                m.IPAddress,
		UserAgent:                m.UserAgent,
		CreatedAt:                m.CreatedAt,
		LastSeenAt:               m.LastSeenAt,
	}
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Session, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_session").Where("id = ?", id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "session")
	}
	return m.toDomain(), nil
}

// FindLive reads only what Bearer checks on every request: the refresh hashes
// and the client details stay in the database.
func (r *Repository) FindLive(ctx context.Context, id string) (*domain.Session, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_session").
		Select("id, user_id, expires_at, revoked_at").Where("id = ?", id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "session")
	}
	return m.toDomain(), nil
}

func (r *Repository) Create(ctx context.Context, s *domain.Session) error {
	id := uuid.New()
	if s.ID != "" {
		parsed, err := uuid.Parse(s.ID)
		if err != nil {
			return shared.WrapError(err, "session")
		}
		id = parsed
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_session (
			id, user_id, expires_at, revoked_at, revocation_reason,
			refresh_token_hash, previous_refresh_token_hash,
			ip_address, user_agent, created_at, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?::inet, ?, ?, ?)
	`, id, s.UserID, s.ExpiresAt, s.RevokedAt, s.RevocationReason,
		s.RefreshTokenHash, nilIfEmpty(s.PreviousRefreshTokenHash),
		s.IPAddress, s.UserAgent, s.CreatedAt, s.LastSeenAt).Error
	if err != nil {
		return shared.WrapError(err, "session")
	}
	s.ID = id.String()
	return nil
}

func (r *Repository) Rotate(ctx context.Context, id string, currentHash, newHash []byte, expiresAt, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_session
		SET previous_refresh_token_hash = refresh_token_hash,
		    refresh_token_hash = ?, expires_at = ?, last_seen_at = ?
		WHERE id = ? AND refresh_token_hash = ? AND revoked_at IS NULL
	`, newHash, expiresAt, now, id, currentHash)
	if res.Error != nil {
		return false, shared.WrapError(res.Error, "session")
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) Revoke(ctx context.Context, id string, now time.Time, reason string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_session
		SET revoked_at = ?, revocation_reason = ?, last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, now, reason, now, id).Error
	return shared.WrapError(err, "session")
}

func (r *Repository) RevokeAllForUser(ctx context.Context, userID string, now time.Time, reason string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_session
		SET revoked_at = ?, revocation_reason = ?, last_seen_at = ?
		WHERE user_id = ? AND revoked_at IS NULL
	`, now, reason, now, userID).Error
	if err != nil {
		return shared.WrapError(err, "session")
	}
	return nil
}

func (r *Repository) ListActive(ctx context.Context, userID string, now time.Time, limit int) ([]domain.Session, error) {
	var rows []row
	err := r.db.WithContext(ctx).Table("usr.usr_session").
		Select("id, user_id, expires_at, ip_address, user_agent, created_at, last_seen_at"). // no refresh hashes
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
		Order("last_seen_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "session")
	}
	out := make([]domain.Session, 0, len(rows))
	for _, m := range rows {
		out = append(out, *m.toDomain())
	}
	return out, nil
}

func (r *Repository) RevokeOwned(ctx context.Context, id, userID string, now time.Time, reason string) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_session
		SET revoked_at = ?, revocation_reason = ?, last_seen_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, now, reason, now, id, userID)
	if res.Error != nil {
		return false, shared.WrapError(res.Error, "session")
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) RevokeOthers(ctx context.Context, userID, keepID string, now time.Time, reason string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_session
		SET revoked_at = ?, revocation_reason = ?, last_seen_at = ?
		WHERE user_id = ? AND id <> ? AND revoked_at IS NULL
	`, now, reason, now, userID, keepID).Error
	return shared.WrapError(err, "session")
}

func (r *Repository) KnownClient(ctx context.Context, userID, ip string) (bool, error) {
	first, last := clientip.Range(ip)
	var known bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM usr.usr_session
			WHERE user_id = ? AND ip_address BETWEEN ?::inet AND ?::inet
		)
	`, userID, first, last).Find(&known).Error
	if err != nil {
		return false, shared.WrapError(err, "session")
	}
	return known, nil
}

func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
