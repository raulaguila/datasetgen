package output

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// CollectionRepository persists app.collection.
type CollectionRepository interface {
	// List returns one page, with record counts, and the count of all matches.
	List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error)
	// ListOptions returns collections with only id and name.
	ListOptions(ctx context.Context, limit int) ([]domain.Collection, error)
	FindByID(ctx context.Context, id string) (*domain.Collection, error)
	Create(ctx context.Context, c *domain.Collection) error
	// Update writes only the fields set in patch; with patch.Version set, a row
	// changed since that version is a conflict.
	Update(ctx context.Context, id string, patch domain.CollectionPatch) (*domain.Collection, error)
	// DeleteMany deletes every listed collection or, when one is missing, none,
	// and returns the deleted names by id for the audit log.
	DeleteMany(ctx context.Context, ids []string) (map[string]string, error)
}
