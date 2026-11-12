package user

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

var _ output.UserRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID        uuid.UUID `gorm:"column:id"`
	Name      string    `gorm:"column:name"`
	Email     string    `gorm:"column:email"`
	Status    string    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() *domain.User {
	return &domain.User{
		ID:        m.ID.String(),
		Name:      m.Name,
		Email:     m.Email,
		Status:    m.Status,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_user").Where("id = ?", id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "user")
	}
	return m.toDomain(), nil
}

func (r *Repository) Access(ctx context.Context, id string, failuresSince, now time.Time) (*domain.UserAccess, error) {
	var out struct {
		LastLoginAt    *time.Time `gorm:"column:last_login_at"`
		FailedLogins   int        `gorm:"column:failed_login_attempts"`
		LockedUntil    *time.Time `gorm:"column:locked_until"`
		RecentFailures int        `gorm:"column:recent_failures"`
		ActiveSessions int        `gorm:"column:active_sessions"`
		// One live token per user and purpose (uni_usr_token_live), so the subquery reads one row.
		ActivationExpiresAt *time.Time `gorm:"column:activation_expires_at"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.last_login_at, c.failed_login_attempts, c.locked_until,
		       (SELECT count(*) FROM usr.usr_login_attempt a
		         WHERE a.email = u.email AND a.created_at >= ? AND `+shared.CountedFailure+`) AS recent_failures,
		       (SELECT count(*) FROM usr.usr_session s
		         WHERE s.user_id = u.id AND s.revoked_at IS NULL AND s.expires_at > ?) AS active_sessions,
		       (SELECT t.expires_at FROM usr.usr_token t
		         WHERE t.user_id = u.id AND t.purpose = ? AND t.used_at IS NULL) AS activation_expires_at
		FROM usr.usr_user u JOIN usr.usr_credential c ON c.user_id = u.id
		WHERE u.id = ?
	`, failuresSince, now, domain.TokenPurposeActivation, id).Take(&out).Error
	if err != nil {
		return nil, shared.WrapError(err, "user")
	}
	a := domain.UserAccess(out)
	return &a, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_user").Where("email = ?", email).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "user")
	}
	return m.toDomain(), nil
}

func (r *Repository) Create(ctx context.Context, u *domain.User) error {
	id := uuid.New()
	if u.ID != "" {
		parsed, err := uuid.Parse(u.ID)
		if err != nil {
			return shared.WrapError(err, "user")
		}
		id = parsed
	}
	now := time.Now()
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_user (id, name, email, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, u.Name, u.Email, u.Status, now, now).Error
	if err != nil {
		return shared.WrapError(err, "user")
	}
	u.ID = id.String()
	u.CreatedAt = now
	u.UpdatedAt = now
	return nil
}

// shortcut: OFFSET and leading-wildcard ILIKE scan what they skip, fine for thousands of users; move to keyset and pg_trgm beyond that.
func (r *Repository) List(ctx context.Context, filter domain.UserFilter) ([]domain.User, int64, error) {
	matching := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("usr.usr_user u").Where(shared.Unprotected)
		if filter.Search != "" {
			pattern := shared.Contains(filter.Search)
			q = q.Where("(u.name ILIKE ? OR u.email::text ILIKE ?)", pattern, pattern)
		}
		if filter.Status != "" {
			q = q.Where("u.status = ?", filter.Status)
		}
		if filter.RoleID != "" {
			q = q.Where("EXISTS (SELECT 1 FROM usr.usr_user_role m WHERE m.user_id = u.id AND m.role_id = ?)", filter.RoleID)
		}
		return q
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "user")
	}
	var rows []row
	err := matching().
		Select("u.id, u.name, u.email, u.status, u.created_at, u.updated_at").
		Order(shared.OrderBy("u", filter.Sort, filter.Order)).
		Limit(filter.Page.Limit).Offset(filter.Page.Offset()).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "user")
	}
	out := make([]domain.User, 0, len(rows))
	for _, m := range rows {
		out = append(out, *m.toDomain())
	}
	return out, total, nil
}

func (r *Repository) Counts(ctx context.Context) (domain.UserCounts, error) {
	var out struct{ Total, Pending, Locked int64 }
	err := r.db.WithContext(ctx).Raw(`
		SELECT count(*) AS total, count(*) FILTER (WHERE u.status = ?) AS pending, count(*) FILTER (WHERE u.status = ?) AS locked
		FROM usr.usr_user u WHERE `+shared.Unprotected,
		domain.UserStatusPending, domain.UserStatusLocked).Scan(&out).Error
	if err != nil {
		return domain.UserCounts{}, shared.WrapError(err, "user")
	}
	return domain.UserCounts{Total: out.Total, Pending: out.Pending, Locked: out.Locked}, nil
}

func (r *Repository) Enable(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_user u
		SET status = CASE
		      WHEN EXISTS (SELECT 1 FROM usr.usr_credential c WHERE c.user_id = u.id AND c.password_hash IS NOT NULL)
		      THEN ? ELSE ? END,
		    updated_at = now()
		WHERE u.id = ? AND u.status = ?
	`, domain.UserStatusActive, domain.UserStatusPending, id, domain.UserStatusDisabled).Error
	return shared.WrapError(err, "user")
}

func (r *Repository) LockStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := r.db.WithContext(ctx).Raw(`SELECT status FROM usr.usr_user WHERE id = ? FOR SHARE`, id).Find(&status).Error
	if err != nil {
		return "", shared.WrapError(err, "user")
	}
	if status == "" {
		return "", shared.WrapError(gorm.ErrRecordNotFound, "user")
	}
	return status, nil
}

func (r *Repository) UpdateName(ctx context.Context, id, name string) error {
	res := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_user SET name = ? WHERE id = ?`, name, id)
	if res.Error == nil && res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "user")
	}
	return shared.WrapError(res.Error, "user")
}

func (r *Repository) AvatarAt(ctx context.Context, id string) (*time.Time, error) {
	var m struct {
		AvatarAt *time.Time `gorm:"column:avatar_at"`
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT avatar_at FROM usr.usr_user WHERE id = ?`, id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "user")
	}
	return m.AvatarAt, nil
}

func (r *Repository) SetAvatarAt(ctx context.Context, id string, at *time.Time) error {
	res := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_user SET avatar_at = ? WHERE id = ?`, at, id)
	if res.Error == nil && res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "user")
	}
	return shared.WrapError(res.Error, "user")
}

func (r *Repository) UpdateStatus(ctx context.Context, id, status string) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_user SET status = ?, updated_at = now() WHERE id = ? AND status <> ?
	`, status, id, domain.UserStatusDisabled).Error
	return shared.WrapError(err, "user")
}
