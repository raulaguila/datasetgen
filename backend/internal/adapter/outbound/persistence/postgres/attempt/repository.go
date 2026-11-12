package attempt

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

var _ output.LoginAttemptRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, a *domain.LoginAttempt) error {
	id := uuid.New()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_login_attempt (id, created_at, email, ip_address, user_agent, success, failure_kind)
		VALUES (?, ?, ?, ?::inet, ?, ?, ?)
	`, id, a.CreatedAt, a.Email, a.IPAddress, a.UserAgent, a.Success, a.FailureKind).Error
	if err != nil {
		return shared.WrapError(err, "login_attempt")
	}
	a.ID = id.String()
	return nil
}

// CountRecentFailures counts the three limits in one round trip, since it runs
// under the login locks. The OR lets the planner combine both partial indexes.
func (r *Repository) CountRecentFailures(ctx context.Context, email string, ip *string, emailSince, ipSince time.Time) (output.FailureCounts, error) {
	var first, last *string // NULL bounds match nothing: no IP counts nothing
	if ip != nil && *ip != "" {
		f, l := clientip.Range(*ip)
		first, last = &f, &l
	}
	var out output.FailureCounts
	// Plain parameters, no CTE: the planner needs constants to pick the partial indexes.
	byEmail := "email = ?::citext AND created_at >= ?"
	byIP := "ip_address BETWEEN ?::inet AND ?::inet AND created_at >= ?"
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			count(*) FILTER (WHERE `+byEmail+`) AS email,
			count(*) FILTER (WHERE `+byEmail+` AND ip_address BETWEEN ?::inet AND ?::inet) AS pair,
			count(*) FILTER (WHERE `+byIP+`) AS ip,
			min(created_at) FILTER (WHERE `+byEmail+`) AS email_oldest,
			min(created_at) FILTER (WHERE `+byEmail+` AND ip_address BETWEEN ?::inet AND ?::inet) AS pair_oldest,
			min(created_at) FILTER (WHERE `+byIP+`) AS ip_oldest
		FROM usr.usr_login_attempt
		WHERE `+shared.CountedFailure+` AND ((`+byEmail+`) OR (`+byIP+`))
	`, email, emailSince,
		email, emailSince, first, last,
		first, last, ipSince,
		email, emailSince,
		email, emailSince, first, last,
		first, last, ipSince,
		email, emailSince, first, last, ipSince).Find(&out).Error
	if err != nil {
		return output.FailureCounts{}, shared.WrapError(err, "login_attempt")
	}
	return out, nil
}

func (r *Repository) ClearFailures(ctx context.Context, email string, since time.Time) error {
	err := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_login_attempt WHERE email = ? AND created_at >= ? AND `+shared.CountedFailure, email, since).Error
	return shared.WrapError(err, "login_attempt")
}

func (r *Repository) Settle(ctx context.Context, id string, success bool, failureKind *string) error {
	err := r.db.WithContext(ctx).Exec(`UPDATE usr.usr_login_attempt SET success = ?, failure_kind = ? WHERE id = ?`, success, failureKind, id).Error
	return shared.WrapError(err, "login_attempt")
}

func (r *Repository) Release(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_login_attempt WHERE id = ?`, id).Error
	return shared.WrapError(err, "login_attempt")
}
