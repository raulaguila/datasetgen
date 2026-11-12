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
	Version     int       `gorm:"column:version"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Collection {
	return domain.Collection{
		ID: m.ID.String(), Name: m.Name, Description: m.Description, Schema: json.RawMessage(m.Schema),
		RecordCount: m.RecordCount, Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// sortColumns is the allowlist behind ORDER BY, which takes no bind parameters.
var sortColumns = map[string]string{"name": "name", "created_at": "created_at", "updated_at": "updated_at"}

const columns = "id, name, description, schema::text AS schema, version, created_at, updated_at"

// List leaves the schema out (the list screen does not show it) and counts records per row.
func (r *Repository) List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error) {
	matching := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("app.collection c")
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
			(SELECT count(*) FROM app.record rc WHERE rc.collection_id = c.id) AS record_count`).
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

func (r *Repository) ListOptions(ctx context.Context, limit int) ([]domain.Collection, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`SELECT id, name FROM app.collection ORDER BY name, id LIMIT ?`, limit).Find(&rows).Error
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

func (r *Repository) Create(ctx context.Context, c *domain.Collection) error {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.collection (name, description, schema) VALUES (?, ?, ?::jsonb)
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
		    schema = COALESCE(?::jsonb, schema),
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
