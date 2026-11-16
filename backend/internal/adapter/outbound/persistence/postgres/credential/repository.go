package credential

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.CredentialRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	UserID              string     `gorm:"column:user_id"`
	PasswordHash        *string    `gorm:"column:password_hash"`
	FailedLoginAttempts int        `gorm:"column:failed_login_attempts"`
	LockedUntil         *time.Time `gorm:"column:locked_until"`
	LastLoginAt         *time.Time `gorm:"column:last_login_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (m row) toDomain() *domain.Credential {
	return &domain.Credential{
		UserID:              m.UserID,
		PasswordHash:        m.PasswordHash,
		FailedLoginAttempts: m.FailedLoginAttempts,
		LockedUntil:         m.LockedUntil,
		LastLoginAt:         m.LastLoginAt,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
}

func (r *Repository) FindByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_credential").Where("user_id = ?", userID).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "credential")
	}
	return m.toDomain(), nil
}

func (r *Repository) Create(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_credential (user_id) VALUES (?) ON CONFLICT (user_id) DO NOTHING
	`, userID).Error
	return shared.WrapError(err, "credential")
}

func (r *Repository) ClearLock(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_credential SET failed_login_attempts = 0, locked_until = NULL WHERE user_id = ?
	`, userID).Error
	return shared.WrapError(err, "credential")
}

func (r *Repository) SetPassword(ctx context.Context, userID, hash string) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_credential
		SET password_hash = ?, failed_login_attempts = 0, locked_until = NULL
		WHERE user_id = ?
	`, hash, userID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "credential")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "credential")
	}
	return nil
}

func (r *Repository) RecordFailure(ctx context.Context, userID string, maxAttempts int, lockUntil, now time.Time) (*time.Time, error) {
	var out struct {
		LockedUntil *time.Time `gorm:"column:locked_until"`
	}
	err := r.db.WithContext(ctx).Raw(`
		UPDATE usr.usr_credential SET
			failed_login_attempts = CASE WHEN locked_until <= ? THEN 1 ELSE failed_login_attempts + 1 END,
			locked_until = CASE
				WHEN (CASE WHEN locked_until <= ? THEN 1 ELSE failed_login_attempts + 1 END) >= ? THEN ?::timestamptz
				WHEN locked_until <= ? THEN NULL
				ELSE locked_until
			END
		WHERE user_id = ?
		RETURNING locked_until
	`, now, now, maxAttempts, lockUntil, now, userID).Find(&out).Error
	if err != nil {
		return nil, shared.WrapError(err, "credential")
	}
	return out.LockedUntil, nil
}

func (r *Repository) RecordSuccess(ctx context.Context, userID, verifiedHash string, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_credential
		SET failed_login_attempts = 0, locked_until = NULL, last_login_at = ?
		WHERE user_id = ? AND password_hash = ? AND (locked_until IS NULL OR locked_until <= ?)
	`, now, userID, verifiedHash, now)
	if res.Error != nil {
		return false, shared.WrapError(res.Error, "credential")
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) MFA(ctx context.Context, userID string) (*domain.MFA, error) {
	var m struct {
		UserID    string
		Secret    string
		EnabledAt *time.Time
		LastStep  int64
	}
	err := r.db.WithContext(ctx).Raw(`SELECT user_id, secret, enabled_at, last_step FROM usr.usr_mfa WHERE user_id = ?`, userID).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "mfa")
	}
	return &domain.MFA{UserID: m.UserID, Secret: m.Secret, EnabledAt: m.EnabledAt, LastStep: m.LastStep}, nil
}

func (r *Repository) SetupMFA(ctx context.Context, userID, sealedSecret string) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_mfa (user_id, secret) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET secret = EXCLUDED.secret, last_step = 0, created_at = now()
		WHERE usr.usr_mfa.enabled_at IS NULL`, userID, sealedSecret)
	return res.RowsAffected == 1, shared.WrapError(res.Error, "mfa")
}

func (r *Repository) EnableMFA(ctx context.Context, userID string, step int64, codeHashes [][]byte, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_mfa SET enabled_at = ?, last_step = ? WHERE user_id = ? AND enabled_at IS NULL`, now, step, userID)
	if res.Error != nil || res.RowsAffected == 0 {
		return false, shared.WrapError(res.Error, "mfa")
	}
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_recovery_code WHERE user_id = ?`, userID).Error; err != nil {
		return false, shared.WrapError(err, "mfa")
	}
	for _, h := range codeHashes { // 10 codes, once per enable
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO usr.usr_recovery_code (user_id, code_hash) VALUES (?, ?)`, userID, h).Error; err != nil {
			return false, shared.WrapError(err, "mfa")
		}
	}
	return true, nil
}

func (r *Repository) UseMFAStep(ctx context.Context, userID string, step int64) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_mfa SET last_step = ? WHERE user_id = ? AND enabled_at IS NOT NULL AND last_step < ?`, step, userID, step)
	return res.RowsAffected == 1, shared.WrapError(res.Error, "mfa")
}

func (r *Repository) UseRecoveryCode(ctx context.Context, userID string, hash []byte, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_recovery_code SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, now, userID, hash)
	return res.RowsAffected == 1, shared.WrapError(res.Error, "mfa")
}

func (r *Repository) RecoveryCodesLeft(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM usr.usr_recovery_code WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&n).Error
	return n, shared.WrapError(err, "mfa")
}

func (r *Repository) DeleteMFA(ctx context.Context, userID string) error {
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_recovery_code WHERE user_id = ?`, userID).Error; err != nil {
		return shared.WrapError(err, "mfa")
	}
	return shared.WrapError(r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_mfa WHERE user_id = ?`, userID).Error, "mfa")
}
