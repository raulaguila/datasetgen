package output

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// ProductRepository persists app.product.
type ProductRepository interface {
	// List returns one page and the count of all matches (-1 in cursor mode).
	List(ctx context.Context, filter domain.ProductFilter) ([]domain.Product, int64, error)
	// ListOptions returns enabled products with only id and name.
	ListOptions(ctx context.Context, limit int) ([]domain.Product, error)
	FindByID(ctx context.Context, id string) (*domain.Product, error)
	Create(ctx context.Context, p *domain.Product) error
	// Update writes only the fields set in patch, so two edits of different fields both land.
	// With patch.Version set, a row changed since that version is a conflict.
	Update(ctx context.Context, id string, patch domain.ProductPatch) (*domain.Product, error)
	// DeleteMany deletes every listed product or, when one is missing, none, and
	// returns the deleted names by id for the audit log.
	DeleteMany(ctx context.Context, ids []string) (map[string]string, error)
}
