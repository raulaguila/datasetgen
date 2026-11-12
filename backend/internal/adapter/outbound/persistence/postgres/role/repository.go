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
	MemberCount int64          `gorm:"column:member_count"`
}

// memberCount counts the members of role s the API may see. It visits every
// member, and the default role holds nearly every user: only screens that show
// the number ask for it.
const memberCount = `(SELECT count(*) FROM usr.usr_user_role m
	WHERE m.role_id = s.id AND m.user_id NOT IN (` + shared.ProtectedIDs + `))`

// load reads the roles of source with their permissions in one round trip, in
// the given order of the alias r. MemberCount stays 0 unless members is set.
func (r *Repository) load(ctx context.Context, members bool, source, order string, args ...any) ([]domain.Role, error) {
	count := "0"
	if members {
		count = memberCount
	}
	var rows []roleRow
	err := r.db.WithContext(ctx).Raw(`
		WITH r AS MATERIALIZED ( -- one count per role, not one per permission row
			SELECT s.id, s.name, s.enabled, s.is_default, s.is_superuser, s.created_at, `+count+` AS member_count
			FROM `+source+` s
		)
		SELECT r.id::text AS id, r.name, r.enabled, r.is_default, r.is_superuser, r.created_at, p.resource, p.actions, r.member_count
		FROM r
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
				MemberCount: row.MemberCount,
			})
		}
		if row.Resource != nil && row.Actions != nil {
			roles[len(roles)-1].Permissions[*row.Resource] = *row.Actions
		}
	}
	return roles, nil
}

// unprotectedRole matches roles the API may change (alias r0); mirrors domain.Role.Protected.
const unprotectedRole = `NOT r0.is_superuser AND NOT EXISTS (
	SELECT 1 FROM usr.usr_role_permission p0 WHERE p0.role_id = r0.id AND p0.resource = '*')`

// shortcut: OFFSET and leading-wildcard ILIKE, fine for hundreds of roles.
func (r *Repository) Page(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, int64, error) {
	where, args := unprotectedRole, []any{}
	if filter.Search != "" {
		where += " AND r0.name ILIKE ?"
		args = append(args, shared.Contains(filter.Search))
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM usr.usr_role r0 WHERE `+where, args...).Find(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "role")
	}
	args = append(args, filter.Page.Limit, filter.Page.Offset())
	order := shared.OrderBy("r0", filter.Sort, filter.Order)
	roles, err := r.load(ctx, true, `(SELECT r0.* FROM usr.usr_role r0 WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?)`, shared.OrderBy("r", filter.Sort, filter.Order), args...)
	if err != nil {
		return nil, 0, err
	}
	return roles, total, nil
}

func (r *Repository) ForUser(ctx context.Context, userID string) ([]domain.Role, error) {
	return r.load(ctx, false, `(SELECT r.* FROM usr.usr_role r JOIN usr.usr_user_role ur ON ur.role_id = r.id WHERE ur.user_id = ?)`, "r.name", userID)
}

func (r *Repository) FindByID(ctx context.Context, id string, members bool) (*domain.Role, error) {
	roles, err := r.load(ctx, members, `(SELECT * FROM usr.usr_role WHERE id = ?)`, "r.name", id)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, shared.WrapError(gorm.ErrRecordNotFound, "role")
	}
	return &roles[0], nil
}

func (r *Repository) Create(ctx context.Context, name string) (*domain.Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO usr.usr_role (name) VALUES (?)
		RETURNING id::text AS id, name, enabled, is_default, is_superuser, created_at
	`, name).Find(&row).Error
	if err != nil {
		return nil, shared.WrapError(err, "role")
	}
	return &domain.Role{
		ID: row.ID, Name: row.Name, Enabled: row.Enabled, IsDefault: row.IsDefault,
		IsSuperuser: row.IsSuperuser, CreatedAt: row.CreatedAt, Permissions: map[string]domain.Action{},
	}, nil
}

// Delete fails with RESOURCE_IN_USE while the role has members.
func (r *Repository) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM usr.usr_role WHERE id = ?`, id)
	if res.Error != nil {
		return shared.WrapError(res.Error, "role")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "role")
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, patch domain.RolePatch) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_role
		SET name = COALESCE(?, name), enabled = COALESCE(?::boolean, enabled), is_default = COALESCE(?::boolean, is_default)
		WHERE id = ?
	`, patch.Name, patch.Enabled, patch.IsDefault, id)
	if res.Error != nil {
		return shared.WrapError(res.Error, "role")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "role")
	}
	return nil
}

func (r *Repository) Grant(ctx context.Context, roleID, resource string, actions domain.Action) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, ?, ?)
	`, roleID, resource, int32(actions)).Error
	if err != nil {
		return shared.WrapError(err, "role_permission")
	}
	return nil
}

func (r *Repository) ClearPermissions(ctx context.Context, roleID string) error {
	err := r.db.WithContext(ctx).Exec(`
		DELETE FROM usr.usr_role_permission WHERE role_id = ?
	`, roleID).Error
	if err != nil {
		return shared.WrapError(err, "role_permission")
	}
	return nil
}

func (r *Repository) Members(ctx context.Context, roleID string, filter domain.UserFilter) ([]domain.User, int64, error) {
	where, args := `m.role_id = ? AND `+shared.Unprotected, []any{roleID}
	if filter.Search != "" {
		pattern := shared.Contains(filter.Search)
		where += " AND (u.name ILIKE ? OR u.email::text ILIKE ?)"
		args = append(args, pattern, pattern)
	}
	from := ` FROM usr.usr_user u JOIN usr.usr_user_role m ON m.user_id = u.id WHERE ` + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*)`+from, args...).Find(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "user_role")
	}
	var users []domain.User
	err := r.db.WithContext(ctx).Raw(`
		SELECT u.id::text AS id, u.name, u.email::text AS email, u.status, u.created_at, u.updated_at`+from+`
		ORDER BY u.name, u.id
		LIMIT ? OFFSET ?
	`, append(args, filter.Page.Limit, filter.Page.Offset())...).Find(&users).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "user_role")
	}
	return users, total, nil
}

func (r *Repository) Unassign(ctx context.Context, userID, roleID string) error {
	err := r.db.WithContext(ctx).Exec(`
		DELETE FROM usr.usr_user_role WHERE user_id = ? AND role_id = ?
	`, userID, roleID).Error
	if err != nil {
		return shared.WrapError(err, "user_role")
	}
	return nil
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
