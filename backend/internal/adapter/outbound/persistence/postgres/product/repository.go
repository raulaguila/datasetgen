package product

import (
	"context"
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

var _ output.ProductRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID        uuid.UUID `gorm:"column:id"`
	Name      string    `gorm:"column:name"`
	Price     string    `gorm:"column:price"`
	Enabled   bool      `gorm:"column:enabled"`
	Version   int       `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Product {
	return domain.Product{
		ID:        m.ID.String(),
		Name:      m.Name,
		Price:     m.Price,
		Enabled:   m.Enabled,
		Version:   m.Version,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// sortColumns is the allowlist behind ORDER BY, which takes no bind parameters.
var sortColumns = map[string]string{
	"name": "name", "price": "price", "created_at": "created_at", "updated_at": "updated_at",
}

// List keeps numbered pages compatible; cursor mode avoids OFFSET and COUNT.
func (r *Repository) List(ctx context.Context, filter domain.ProductFilter) ([]domain.Product, int64, error) {
	matching := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("app.product")
		if filter.Search != "" {
			q = q.Where("name ILIKE ?", shared.Contains(filter.Search))
		}
		if filter.Enabled != nil {
			q = q.Where("enabled = ?", *filter.Enabled)
		}
		return q
	}
	total := int64(-1) // cursor pages deliberately do not calculate a total
	if !filter.Cursor {
		if err := matching().Count(&total).Error; err != nil {
			return nil, 0, shared.WrapError(err, "product")
		}
	}
	column, ok := sortColumns[filter.Sort]
	if !ok {
		column = "created_at"
	}
	direction := "DESC"
	if filter.Order == "asc" {
		direction = "ASC"
	}
	q := matching()
	order := "app.product." + column + " " + direction + ", app.product.id"
	if filter.Cursor {
		order = "app.product.created_at DESC, app.product.id DESC"
		if filter.After != nil {
			q = q.Where("(app.product.created_at, app.product.id) < (?, ?::uuid)", filter.After.CreatedAt, filter.After.ID)
		}
	} else {
		q = q.Offset(filter.Page.Offset())
	}
	var rows []row
	err := q.Select("id, name, price::text AS price, enabled, version, created_at, updated_at").Order(order).Limit(filter.Page.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "product")
	}
	out := make([]domain.Product, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, total, nil
}

func (r *Repository) ListOptions(ctx context.Context, limit int) ([]domain.Product, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, name FROM app.product WHERE enabled ORDER BY name, id LIMIT ?
	`, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "product")
	}
	out := make([]domain.Product, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Product, error) {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, name, price::text, enabled, version, created_at, updated_at
		FROM app.product WHERE id = ?
	`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "product")
	}
	p := m.toDomain()
	return &p, nil
}

func (r *Repository) Create(ctx context.Context, p *domain.Product) error {
	id := uuid.New()
	now := time.Now()
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO app.product (id, name, price, enabled, created_at, updated_at)
		VALUES (?, ?, ?::numeric, ?, ?, ?)
	`, id, p.Name, p.Price, p.Enabled, now, now).Error
	if err != nil {
		return shared.WrapError(err, "product")
	}
	p.ID = id.String()
	p.Version = 1
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, patch domain.ProductPatch) (*domain.Product, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.product
		SET name = COALESCE(?::text, name),
		    price = COALESCE(?::numeric, price),
		    enabled = COALESCE(?::boolean, enabled),
		    version = version + 1,
		    updated_at = now()
		WHERE id = ? AND (?::integer IS NULL OR version = ?)
		RETURNING id, name, price::text AS price, enabled, version, created_at, updated_at
	`, patch.Name, patch.Price, patch.Enabled, id, patch.Version, patch.Version).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "product")
	}
	if len(rows) == 0 {
		return nil, r.missOrStale(ctx, id, patch.Version)
	}
	p := rows[0].toDomain()
	return &p, nil
}

// missOrStale tells a missing row from one changed since the caller read it.
// It runs only after an update matched nothing, so the happy path stays one statement.
func (r *Repository) missOrStale(ctx context.Context, id string, version *int) error {
	if version == nil {
		return shared.WrapError(gorm.ErrRecordNotFound, "product")
	}
	var found []int
	if err := r.db.WithContext(ctx).Raw(`SELECT 1 FROM app.product WHERE id = ?`, id).Find(&found).Error; err != nil {
		return shared.WrapError(err, "product")
	}
	if len(found) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "product")
	}
	return apperror.New(apperror.CodeConflict, "the record was changed by someone else").WithField("version")
}

// DeleteMany is one statement: the count check and the delete see the same snapshot.
func (r *Repository) DeleteMany(ctx context.Context, ids []string) (map[string]string, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM app.product WHERE id IN ?
		AND (SELECT count(*) FROM app.product WHERE id IN ?) = ?
		RETURNING id, name
	`, ids, ids, len(ids)).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "product")
	}
	if len(rows) == 0 {
		return nil, shared.WrapError(gorm.ErrRecordNotFound, "product")
	}
	names := make(map[string]string, len(rows))
	for _, m := range rows {
		names[m.ID.String()] = m.Name
	}
	return names, nil
}
