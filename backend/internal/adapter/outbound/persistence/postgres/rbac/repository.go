package rbac

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

// Repository reads effective permissions and superuser flag from Postgres.
type Repository struct {
	db *gorm.DB
}

var _ output.PermissionReader = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListResources returns the catalog. It has few rows, so it has no pagination.
func (r *Repository) ListResources(ctx context.Context) ([]domain.Resource, error) {
	var rows []domain.Resource
	err := r.db.WithContext(ctx).Raw(`
		SELECT code, description, actions FROM usr.usr_resource ORDER BY code
	`).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "resources")
	}
	return rows, nil
}

type accessRow struct {
	UserID    string     `gorm:"column:user_id"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	Actions   int32      `gorm:"column:actions"`
}

// Access runs on every request behind Require: one row with the session and the
// grants for this resource and '*' only. 2147483647 is every bit, as in the admin seed.
func (r *Repository) Access(ctx context.Context, sessionID, resource string) (*domain.Session, domain.Action, error) {
	var m accessRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.user_id, s.expires_at, s.revoked_at, (
			SELECT CASE WHEN bool_or(role.is_superuser OR p.resource = '*') THEN 2147483647
			            ELSE COALESCE(bit_or(p.actions) FILTER (WHERE p.resource = ?), 0) END
			FROM usr.usr_user_role ur
			JOIN usr.usr_role role ON role.id = ur.role_id AND role.enabled
			LEFT JOIN usr.usr_role_permission p ON p.role_id = role.id AND p.resource IN (?, '*')
			WHERE ur.user_id = s.user_id
		) AS actions
		FROM usr.usr_session s
		WHERE s.id = ?
	`, resource, resource, sessionID).Take(&m).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "session")
	}
	return &domain.Session{ID: sessionID, UserID: m.UserID, ExpiresAt: m.ExpiresAt, RevokedAt: m.RevokedAt}, domain.Action(m.Actions), nil
}

type effectiveRow struct {
	Superuser bool    `gorm:"column:superuser"`
	Resource  *string `gorm:"column:resource"`
	Actions   *int32  `gorm:"column:actions"`
}

// LoadEffective runs on every protected request, so it is one round trip: the
// left join keeps one row for a user with no grant, to carry the superuser flag.
func (r *Repository) LoadEffective(ctx context.Context, userID string) (*domain.EffectivePermissions, error) {
	var rows []effectiveRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT flag.superuser, p.resource, p.actions
		FROM (
			SELECT EXISTS (
				SELECT 1
				FROM usr.usr_user_role ur
				JOIN usr.usr_role role ON role.id = ur.role_id AND role.enabled = true
				WHERE ur.user_id = ? AND role.is_superuser = true
			) AS superuser
		) flag
		LEFT JOIN usr.vw_user_effective_permissions p ON p.user_id = ?
	`, userID, userID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "permissions")
	}

	eff := &domain.EffectivePermissions{UserID: userID, ByResource: make(map[string]domain.Action, len(rows))}
	for _, row := range rows {
		eff.IsSuperuser = row.Superuser
		if row.Resource != nil && row.Actions != nil {
			eff.ByResource[*row.Resource] = domain.Action(*row.Actions)
		}
	}
	return eff, nil
}
