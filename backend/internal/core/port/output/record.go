package output

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// RecordRepository persists app.record.
type RecordRepository interface {
	// List returns one page; each Data holds only top-level plain values, strings cut to 120 characters.
	List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error)
	FindByID(ctx context.Context, id string) (*domain.Record, error)
	Create(ctx context.Context, r *domain.Record) error
	// Update replaces data; with version set, a row changed since that version is a conflict.
	Update(ctx context.Context, id string, data json.RawMessage, version *int) (*domain.Record, error)
	// DeleteMany deletes every listed record or, when one is missing, none.
	DeleteMany(ctx context.Context, ids []string) error
	// CollectionsOf maps each listed record that exists to its collection id.
	CollectionsOf(ctx context.Context, ids []string) (map[string]string, error)
	// Scan returns up to limit records of the collection after afterID (by id),
	// with id, full data and updated_at only.
	Scan(ctx context.Context, collectionID, afterID string, limit int) ([]domain.Record, error)
}
