package role

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

var _ output.RoleRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// DefaultRoleIDs lists every enabled default role; none is a valid answer.
func (r *Repository) DefaultRoleIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Raw(`
		SELECT id::text FROM usr.usr_role WHERE is_default AND enabled ORDER BY name
	`).Find(&ids).Error
	if err != nil {
		return nil, shared.WrapError(err, "role")
	}
	return ids, nil
}

func (r *Repository) FindIDByName(ctx context.Context, name string) (string, error) {
	var id string
	err := r.db.WithContext(ctx).Raw(`SELECT id::text FROM usr.usr_role WHERE name = ?`, name).Find(&id).Error
	if err != nil {
		return "", shared.WrapError(err, "role")
	}
	if id == "" {
		return "", shared.WrapError(gorm.ErrRecordNotFound, "role")
	}
	return id, nil
}

func (r *Repository) HasMembers(ctx context.Context, roleID string) (bool, error) {
	var found bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (SELECT 1 FROM usr.usr_user_role WHERE role_id = ?)
	`, roleID).Find(&found).Error
	if err != nil {
		return false, shared.WrapError(err, "user_role")
	}
	return found, nil
}

type roleRow struct {
	ID          string         `gorm:"column:id"`
	Name        string         `gorm:"column:name"`
	Enabled     bool           `gorm:"column:enabled"`
	IsDefault   bool           `gorm:"column:is_default"`
	IsSuperuser bool           `gorm:"column:is_superuser"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
	Resource    *string        `gorm:"column:resource"`
	Actions     *domain.Action `gorm:"column:actions"`
}

// load reads the roles of source with their permissions in one round trip, in
// the given order of the alias r.
func (r *Repository) load(ctx context.Context, source, order string, args ...any) ([]domain.Role, error) {
	var rows []roleRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.id::text AS id, r.name, r.enabled, r.is_default, r.is_superuser, r.created_at, p.resource, p.actions
		FROM `+source+` r
		LEFT JOIN usr.usr_role_permission p ON p.role_id = r.id
		ORDER BY `+order+`, r.id, p.resource
	`, args...).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "role")
	}
	roles := []domain.Role{}
	for _, row := range rows {
		if len(roles) == 0 || roles[len(roles)-1].ID != row.ID {
			roles = append(roles, domain.Role{
				ID: row.ID, Name: row.Name, Enabled: row.Enabled, IsDefault: row.IsDefault,
				IsSuperuser: row.IsSuperuser, CreatedAt: row.CreatedAt, Permissions: map[string]domain.Action{},
			})
		}
		if row.Resource != nil && row.Actions != nil {
			roles[len(roles)-1].Permissions[*row.Resource] = *row.Actions
		}
	}
	return roles, nil
}

func (r *Repository) ForUser(ctx context.Context, userID string) ([]domain.Role, error) {
	return r.load(ctx, `(SELECT r.* FROM usr.usr_role r JOIN usr.usr_user_role ur ON ur.role_id = r.id WHERE ur.user_id = ?)`, "r.name", userID)
}

func (r *Repository) Unassign(ctx context.Context, userID, roleID string) error {
	err := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_user_role WHERE user_id = ? AND role_id = ?`, userID, roleID).Error
	return shared.WrapError(err, "user_role")
}

func (r *Repository) Assign(ctx context.Context, userID, roleID string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_user_role (user_id, role_id, created_at)
		VALUES (?, ?, now())
		ON CONFLICT DO NOTHING
	`, userID, roleID).Error
	if err != nil {
		return shared.WrapError(err, "user_role")
	}
	return nil
}
