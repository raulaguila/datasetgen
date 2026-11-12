package record

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

var _ output.RecordRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID           uuid.UUID `gorm:"column:id"`
	CollectionID uuid.UUID `gorm:"column:collection_id"`
	Data         []byte    `gorm:"column:data"`
	Version      int       `gorm:"column:version"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Record {
	return domain.Record{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), Data: json.RawMessage(m.Data),
		Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

const columns = "id, collection_id, data::text AS data, version, created_at, updated_at"

// shortcut: search is a trigram ILIKE over the whole JSON text, so keys match too;
// move to full-text or per-field filters when collections hold tens of thousands of records.
func (r *Repository) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	matching := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("app.record").Where("collection_id = ?", filter.CollectionID)
		if filter.Search != "" {
			q = q.Where("data::text ILIKE ?", shared.Contains(filter.Search))
		}
		return q
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "record")
	}
	direction := "DESC"
	if filter.Order == "asc" {
		direction = "ASC"
	}
	var rows []row
	err := matching().Select(columns).Order("created_at " + direction + ", id " + direction).
		Offset(filter.Page.Offset()).Limit(filter.Page.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "record")
	}
	out := make([]domain.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, total, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Record, error) {
	var m row
	if err := r.db.WithContext(ctx).Raw(`SELECT `+columns+` FROM app.record WHERE id = ?`, id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "record")
	}
	rec := m.toDomain()
	return &rec, nil
}

func (r *Repository) Create(ctx context.Context, rec *domain.Record) error {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.record (collection_id, data) VALUES (?, ?::jsonb) RETURNING `+columns,
		rec.CollectionID, string(rec.Data)).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "record")
	}
	*rec = m.toDomain()
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, data json.RawMessage, version *int) (*domain.Record, error) {
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.record SET data = ?::jsonb, version = version + 1
		WHERE id = ? AND (?::integer IS NULL OR version = ?)
		RETURNING `+columns, string(data), id, version, version).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	if len(rows) == 0 {
		if version == nil {
			return nil, shared.WrapError(gorm.ErrRecordNotFound, "record")
		}
		// The service read the row in this transaction, so a miss here is a newer version.
		return nil, apperror.New(apperror.CodeConflict, "the record was changed by someone else").WithField("version")
	}
	rec := rows[0].toDomain()
	return &rec, nil
}

func (r *Repository) DeleteMany(ctx context.Context, ids []string) error {
	var deleted []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM app.record WHERE id IN ?
		AND (SELECT count(*) FROM app.record WHERE id IN ?) = ?
		RETURNING id
	`, ids, ids, len(ids)).Find(&deleted).Error
	if err != nil {
		return shared.WrapError(err, "record")
	}
	if len(deleted) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "record")
	}
	return nil
}
