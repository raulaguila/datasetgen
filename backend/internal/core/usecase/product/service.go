package product

import (
	"context"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	optionsCap     = 1000
	maxSearchBytes = 120
)

var sortable = []string{"name", "price", "created_at", "updated_at"}

// Fits NUMERIC(12,2). Strict on purpose: floats accept NaN, Inf and hex.
var priceFormat = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)

// Service reads through Repo. Every change runs in Tx with its audit record, so
// a module copied from this one is audited from the start.
type Service struct {
	Repo output.ProductRepository
	Tx   output.TxManager
}

var _ input.ProductService = (*Service)(nil)

func New(repo output.ProductRepository, tx output.TxManager) *Service {
	return &Service{Repo: repo, Tx: tx}
}

func (s *Service) List(ctx context.Context, filter domain.ProductFilter) ([]domain.Product, int64, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	if err := validate.Sort(&filter.Sort, &filter.Order, sortable, "created_at", "desc"); err != nil {
		return nil, 0, err
	}
	filter.Page = domain.NewPage(filter.Page.Number, filter.Page.Limit)
	if filter.Cursor {
		if filter.Sort != "created_at" || filter.Order != "desc" || filter.Page.Number != 1 {
			return nil, 0, apperror.New(apperror.CodeInvalidInput, "cursor requires created_at desc and no page offset").WithField("cursor")
		}
		if filter.After != nil {
			if err := validate.ID(filter.After.ID); err != nil {
				return nil, 0, err
			}
			if filter.After.CreatedAt.IsZero() {
				return nil, 0, apperror.New(apperror.CodeInvalidInput, "invalid cursor").WithField("cursor")
			}
		}
	}
	return s.Repo.List(ctx, filter)
}

func (s *Service) Options(ctx context.Context) ([]domain.Product, error) {
	return s.Repo.ListOptions(ctx, optionsCap)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Product, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	return s.Repo.FindByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, cmd input.CreateProductCommand) (*domain.Product, error) {
	name, price, err := validateNamePrice(cmd.Name, cmd.Price)
	if err != nil {
		return nil, err
	}
	enabled := true
	if cmd.Enabled != nil {
		enabled = *cmd.Enabled
	}
	p := &domain.Product{Name: name, Price: price, Enabled: enabled}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Products().Create(ctx, p); err != nil {
			return err
		}
		if err := policy.EnqueueJob(ctx, uow, JobCreated, createdJob{ID: p.ID}); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "product.create", p.ID, map[string]any{"name": p.Name, "price": p.Price, "enabled": p.Enabled})
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, cmd input.UpdateProductCommand) (*domain.Product, error) {
	if err := validate.ID(cmd.ID); err != nil {
		return nil, err
	}
	patch := domain.ProductPatch{Enabled: cmd.Enabled, Version: cmd.Version}
	if cmd.Name != nil {
		name, err := normalizeName(*cmd.Name)
		if err != nil {
			return nil, err
		}
		patch.Name = &name
	}
	if cmd.Price != nil {
		price, err := normalizePrice(*cmd.Price)
		if err != nil {
			return nil, err
		}
		patch.Price = &price
	}
	var updated *domain.Product
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		var err error
		if updated, err = uow.Products().Update(ctx, cmd.ID, patch); err != nil {
			return err
		}
		changes := map[string]any{}
		if patch.Name != nil {
			changes["name"] = *patch.Name
		}
		if patch.Price != nil {
			changes["price"] = *patch.Price
		}
		if patch.Enabled != nil {
			changes["enabled"] = *patch.Enabled
		}
		return policy.Audit(ctx, uow, "product.update", updated.ID, changes)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

const maxBatch = 100 // ids in one batch request; a page holds at most 100 rows

func (s *Service) DeleteMany(ctx context.Context, ids []string) error {
	if len(ids) == 0 || len(ids) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	ids = slices.Compact(slices.Sorted(slices.Values(ids))) // a repeated id would fail the count check
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		names, err := uow.Products().DeleteMany(ctx, ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			// The name stays readable in the log once the row is gone.
			if err := policy.Audit(ctx, uow, "product.delete", id, map[string]any{"name": names[id]}); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateNamePrice(name, price string) (string, string, error) {
	name, err := normalizeName(name)
	if err != nil {
		return "", "", err
	}
	p, err := normalizePrice(price)
	if err != nil {
		return "", "", err
	}
	return name, p, nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 120 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", apperror.New(apperror.CodeInvalidInput, "name must be 1-120 characters").WithField("name")
	}
	return name, nil
}

func normalizePrice(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !priceFormat.MatchString(raw) {
		return "", apperror.New(apperror.CodeInvalidInput, "price must be a decimal with up to 10 digits and 2 decimals").WithField("price")
	}
	price, _ := new(big.Rat).SetString(raw) // cannot fail: priceFormat matched
	return price.FloatString(2), nil
}
