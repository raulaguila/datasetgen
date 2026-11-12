package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

type CreateProductCommand struct {
	Name    string
	Price   string
	Enabled *bool
}

type UpdateProductCommand struct {
	ID      string
	Name    *string
	Price   *string
	Enabled *bool
	Version *int // the version the caller read; nil skips the check
}

// ProductService is the inbound port for product CRUD.
type ProductService interface {
	List(ctx context.Context, filter domain.ProductFilter) ([]domain.Product, int64, error)
	Options(ctx context.Context) ([]domain.Product, error)
	Get(ctx context.Context, id string) (*domain.Product, error)
	Create(ctx context.Context, cmd CreateProductCommand) (*domain.Product, error)
	Update(ctx context.Context, cmd UpdateProductCommand) (*domain.Product, error)
	// DeleteMany deletes every listed product or none.
	DeleteMany(ctx context.Context, ids []string) error
}
