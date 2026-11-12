package record

import (
	"context"
	"slices"
	"strings"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxSearchBytes = 120
	maxBatch       = 100
)

// Service checks every record against its collection's schema inside the
// transaction that writes it, so a schema change cannot slip in between.
type Service struct {
	Repo output.RecordRepository
	Tx   output.TxManager
}

var _ input.RecordService = (*Service)(nil)

func New(repo output.RecordRepository, tx output.TxManager) *Service {
	return &Service{Repo: repo, Tx: tx}
}

func (s *Service) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	if err := validate.ID(filter.CollectionID); err != nil {
		return nil, 0, apperror.New(apperror.CodeInvalidInput, "collection_id is required").WithField("collection_id")
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	sort := "created_at"
	if err := validate.Sort(&sort, &filter.Order, []string{"created_at"}, "created_at", "desc"); err != nil {
		return nil, 0, err
	}
	filter.Page = domain.NewPage(filter.Page.Number, filter.Page.Limit)
	return s.Repo.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Record, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	return s.Repo.FindByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, cmd input.CreateRecordCommand) (*domain.Record, error) {
	if err := validate.ID(cmd.CollectionID); err != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "collection_id is required").WithField("collection_id")
	}
	if len(cmd.Data) == 0 {
		return nil, apperror.New(apperror.CodeInvalidInput, "data is required").WithField("data")
	}
	r := &domain.Record{CollectionID: cmd.CollectionID, Data: cmd.Data}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := checkData(ctx, uow, cmd.CollectionID, cmd.Data); err != nil {
			return err
		}
		if err := uow.Records().Create(ctx, r); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "record.create", r.ID, map[string]any{"collection_id": r.CollectionID})
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) Update(ctx context.Context, cmd input.UpdateRecordCommand) (*domain.Record, error) {
	if err := validate.ID(cmd.ID); err != nil {
		return nil, err
	}
	if len(cmd.Data) == 0 {
		return nil, apperror.New(apperror.CodeInvalidInput, "data is required").WithField("data")
	}
	var updated *domain.Record
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		current, err := uow.Records().FindByID(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := checkData(ctx, uow, current.CollectionID, cmd.Data); err != nil {
			return err
		}
		if updated, err = uow.Records().Update(ctx, cmd.ID, cmd.Data, cmd.Version); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "record.update", updated.ID, map[string]any{"collection_id": updated.CollectionID})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteMany(ctx context.Context, ids []string) error {
	if len(ids) == 0 || len(ids) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Records().DeleteMany(ctx, ids); err != nil {
			return err
		}
		for _, id := range ids {
			if err := policy.Audit(ctx, uow, "record.delete", id, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// checkData validates data against the collection schema.
// shortcut: compiles the schema on every write; cache by collection id and version
// when bulk imports make it show up in profiles.
func checkData(ctx context.Context, uow output.UnitOfWork, collectionID string, data []byte) error {
	c, err := uow.Collections().FindByID(ctx, collectionID)
	if err != nil {
		return err
	}
	compiled, err := schema.Compile(c.Schema)
	if err != nil {
		return err
	}
	return schema.Validate(compiled, data)
}
