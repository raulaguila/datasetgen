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
// transaction that writes it; the schema row stays share-locked until commit,
// so a concurrent schema change waits instead of slipping in between.
// Reading needs a viewer role in the record's collection, writing an editor role.
type Service struct {
	Repo        output.RecordRepository
	Collections output.CollectionRepository
	Tx          output.TxManager
}

var _ input.RecordService = (*Service)(nil)

func New(repo output.RecordRepository, collections output.CollectionRepository, tx output.TxManager) *Service {
	return &Service{Repo: repo, Collections: collections, Tx: tx}
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
	if _, err := policy.CollectionRole(ctx, s.Collections, filter.CollectionID, domain.RoleViewer); err != nil {
		return nil, 0, err
	}
	return s.Repo.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Record, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	r, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := policy.CollectionRole(ctx, s.Collections, r.CollectionID, domain.RoleViewer); err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, apperror.New(apperror.CodeNotFound, "record not found")
		}
		return nil, err
	}
	return r, nil
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
		if _, err := policy.CollectionRole(ctx, uow.Collections(), cmd.CollectionID, domain.RoleEditor); err != nil {
			return err
		}
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
		if err := editor(ctx, uow, current.CollectionID); err != nil {
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
		collections, err := uow.Records().CollectionIDs(ctx, ids)
		if err != nil {
			return err
		}
		for _, c := range collections {
			if err := editor(ctx, uow, c); err != nil {
				return err
			}
		}
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

// editor checks the caller may write records of the collection; a hidden
// collection reads as a missing record, so ids do not leak.
func editor(ctx context.Context, uow output.UnitOfWork, collectionID string) error {
	_, err := policy.CollectionRole(ctx, uow.Collections(), collectionID, domain.RoleEditor)
	if apperror.IsCode(err, apperror.CodeNotFound) {
		return apperror.New(apperror.CodeNotFound, "record not found")
	}
	return err
}

// checkData validates data against the collection schema.
// shortcut: compiles the schema on every write; cache by collection id and version
// when bulk imports make it show up in profiles.
func checkData(ctx context.Context, uow output.UnitOfWork, collectionID string, data []byte) error {
	raw, err := uow.Collections().SchemaForWrite(ctx, collectionID)
	if err != nil {
		return err
	}
	compiled, err := schema.Compile(raw)
	if err != nil {
		return err
	}
	return schema.Validate(compiled, data)
}
