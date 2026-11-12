package collection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Repository struct {
	db *gorm.DB
}

var _ output.CollectionRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID          uuid.UUID `gorm:"column:id"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	Schema      []byte    `gorm:"column:schema"`
	RecordCount int64     `gorm:"column:record_count"`
	Role        string    `gorm:"column:role"`
	Version     int       `gorm:"column:version"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Collection {
	return domain.Collection{
		ID: m.ID.String(), Name: m.Name, Description: m.Description, Schema: json.RawMessage(m.Schema),
		RecordCount: m.RecordCount, Role: domain.MemberRole(m.Role), Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// sortColumns is the allowlist behind ORDER BY, which takes no bind parameters.
var sortColumns = map[string]string{"name": "name", "created_at": "created_at", "updated_at": "updated_at"}

// platformAdmin is true when the user (one bind) holds an enabled superuser or
// full-access ('*') role; such a user sees and owns every collection.
const platformAdmin = `EXISTS (
	SELECT 1 FROM usr.usr_user_role ur
	JOIN usr.usr_role ro ON ro.id = ur.role_id AND ro.enabled
	WHERE ur.user_id = ? AND (ro.is_superuser OR EXISTS (
		SELECT 1 FROM usr.usr_role_permission p WHERE p.role_id = ro.id AND p.resource = '*')))`

// visible joins the caller's membership and keeps the rows they may see.
func visible(q *gorm.DB, userID string) *gorm.DB {
	return q.Joins("LEFT JOIN app.collection_member m ON m.collection_id = c.id AND m.user_id = ?", userID).
		Where("(m.user_id IS NOT NULL OR "+platformAdmin+")", userID)
}

const columns = "id, name, description, schema::text AS schema, version, created_at, updated_at"

// List leaves the schema out (the list screen does not show it) and counts records per row.
func (r *Repository) List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error) {
	matching := func() *gorm.DB {
		q := visible(r.db.WithContext(ctx).Table("app.collection c"), filter.UserID)
		if filter.Search != "" {
			q = q.Where("c.name ILIKE ?", shared.Contains(filter.Search))
		}
		return q
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "collection")
	}
	column, ok := sortColumns[filter.Sort]
	if !ok {
		column = "name"
	}
	direction := "ASC"
	if filter.Order == "desc" {
		direction = "DESC"
	}
	var rows []row
	err := matching().
		Select(`c.id, c.name, c.description, c.version, c.created_at, c.updated_at,
			(SELECT count(*) FROM app.record rc WHERE rc.collection_id = c.id) AS record_count,
			CASE WHEN `+platformAdmin+` THEN 'owner' ELSE m.role END AS role`, filter.UserID).
		Order("c." + column + " " + direction + ", c.id").
		Offset(filter.Page.Offset()).Limit(filter.Page.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "collection")
	}
	out := make([]domain.Collection, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, total, nil
}

func (r *Repository) ListOptions(ctx context.Context, userID string, limit int) ([]domain.Collection, error) {
	var rows []row
	err := visible(r.db.WithContext(ctx).Table("app.collection c"), userID).
		Select("c.id, c.name").Order("c.name, c.id").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := make([]domain.Collection, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Collection, error) {
	var m row
	err := r.db.WithContext(ctx).Raw(`SELECT `+columns+` FROM app.collection WHERE id = ?`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	c := m.toDomain()
	return &c, nil
}

func (r *Repository) SchemaForWrite(ctx context.Context, id string) (json.RawMessage, error) {
	var m row
	err := r.db.WithContext(ctx).Raw(`SELECT schema::text AS schema FROM app.collection WHERE id = ? FOR SHARE`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	return json.RawMessage(m.Schema), nil
}

func (r *Repository) Create(ctx context.Context, c *domain.Collection) error {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.collection (name, description, schema) VALUES (?, ?, ?::json)
		RETURNING `+columns, c.Name, c.Description, string(c.Schema)).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "collection")
	}
	*c = m.toDomain()
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, patch domain.CollectionPatch) (*domain.Collection, error) {
	var schema *string
	if patch.Schema != nil {
		s := string(patch.Schema)
		schema = &s
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.collection
		SET name = COALESCE(?::text, name),
		    description = COALESCE(?::text, description),
		    schema = COALESCE(?::json, schema),
		    version = version + 1
		WHERE id = ? AND (?::integer IS NULL OR version = ?)
		RETURNING `+columns, patch.Name, patch.Description, schema, id, patch.Version, patch.Version).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	if len(rows) == 0 {
		return nil, missOrStale(ctx, r.db, "app.collection", "collection", id, patch.Version)
	}
	c := rows[0].toDomain()
	return &c, nil
}

// DeleteMany is one statement: the count check and the delete see the same snapshot.
// A collection that still has records fails on the foreign key (RESOURCE_IN_USE).
func (r *Repository) DeleteMany(ctx context.Context, ids []string) (map[string]string, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM app.collection WHERE id IN ?
		AND (SELECT count(*) FROM app.collection WHERE id IN ?) = ?
		RETURNING id, name
	`, ids, ids, len(ids)).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	if len(rows) == 0 {
		return nil, shared.WrapError(gorm.ErrRecordNotFound, "collection")
	}
	names := make(map[string]string, len(rows))
	for _, m := range rows {
		names[m.ID.String()] = m.Name
	}
	return names, nil
}

// missOrStale tells a missing row from one changed since the caller read it.
func missOrStale(ctx context.Context, db *gorm.DB, table, resource, id string, version *int) error {
	if version == nil {
		return shared.WrapError(gorm.ErrRecordNotFound, resource)
	}
	var found []int
	if err := db.WithContext(ctx).Raw(`SELECT 1 FROM `+table+` WHERE id = ?`, id).Find(&found).Error; err != nil {
		return shared.WrapError(err, resource)
	}
	if len(found) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, resource)
	}
	return apperror.New(apperror.CodeConflict, "the record was changed by someone else").WithField("version")
}

func (r *Repository) Access(ctx context.Context, id, userID string) (domain.MemberRole, error) {
	var m struct{ Role *string }
	err := r.db.WithContext(ctx).Raw(`
		SELECT CASE WHEN `+platformAdmin+` THEN 'owner'
		       ELSE (SELECT m.role FROM app.collection_member m WHERE m.collection_id = c.id AND m.user_id = ?) END AS role
		FROM app.collection c WHERE c.id = ?
	`, userID, userID, id).Take(&m).Error
	if err == nil && m.Role == nil {
		err = gorm.ErrRecordNotFound // not a member: the collection stays hidden
	}
	if err != nil {
		return "", shared.WrapError(err, "collection")
	}
	return domain.MemberRole(*m.Role), nil
}

type memberRow struct {
	UserID    uuid.UUID `gorm:"column:user_id"`
	Name      string    `gorm:"column:name"`
	Email     string    `gorm:"column:email"`
	Status    string    `gorm:"column:status"`
	Role      string    `gorm:"column:role"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (r *Repository) Members(ctx context.Context, id string) ([]domain.CollectionMember, error) {
	var rows []memberRow
	// shortcut: no paging; a collection has a handful of helpers. Page it past ~200.
	err := r.db.WithContext(ctx).Raw(`
		SELECT m.user_id, u.name, u.email::text AS email, u.status, m.role, m.created_at
		FROM app.collection_member m JOIN usr.usr_user u ON u.id = m.user_id
		WHERE m.collection_id = ?
		ORDER BY m.role = 'owner' DESC, u.name, m.user_id
	`, id).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "member")
	}
	out := make([]domain.CollectionMember, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.CollectionMember{
			UserID: m.UserID.String(), Name: m.Name, Email: m.Email, Status: m.Status,
			Role: domain.MemberRole(m.Role), CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

func (r *Repository) AddMember(ctx context.Context, id, userID string, role domain.MemberRole) error {
	err := r.db.WithContext(ctx).Exec(`INSERT INTO app.collection_member (collection_id, user_id, role) VALUES (?, ?, ?)`, id, userID, string(role)).Error
	return shared.WrapError(err, "member")
}

func (r *Repository) SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error {
	res := r.db.WithContext(ctx).Exec(`UPDATE app.collection_member SET role = ? WHERE collection_id = ? AND user_id = ?`, string(role), id, userID)
	return memberChanged(res)
}

func (r *Repository) RemoveMember(ctx context.Context, id, userID string) error {
	return memberChanged(r.db.WithContext(ctx).Exec(`DELETE FROM app.collection_member WHERE collection_id = ? AND user_id = ?`, id, userID))
}

func (r *Repository) Owners(ctx context.Context, id string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM app.collection_member WHERE collection_id = ? AND role = 'owner'`, id).Take(&n).Error
	return n, shared.WrapError(err, "member")
}

func memberChanged(res *gorm.DB) error {
	if res.Error == nil && res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "member")
	}
	return shared.WrapError(res.Error, "member")
}
