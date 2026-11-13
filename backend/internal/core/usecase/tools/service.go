// Package tools runs the trash, the field history, saved views and attached files.
package tools

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxBatch      = 100
	trashPage     = 200
	historyPage   = 50
	maxViewName   = 80
	maxViewQuery  = 2000
	maxFileName   = 200
	fileKeyPrefix = "files/"
)

type Service struct {
	Store output.UnitOfWork
	Tx    output.TxManager
	Files output.FileStore
}

var _ input.ToolsService = (*Service)(nil)

func role(ctx context.Context, uow output.UnitOfWork, collectionID string, need domain.MemberRole) error {
	_, err := policy.CollectionRole(ctx, uow.Collections(), collectionID, need)
	return err
}

func checkIDs(ids []string, allowEmpty bool) ([]string, error) {
	if (len(ids) == 0 && !allowEmpty) || len(ids) > maxBatch {
		return nil, apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *Service) TrashedRecords(ctx context.Context, collectionID string) ([]domain.TrashedRecord, error) {
	if err := role(ctx, s.Store, collectionID, domain.RoleEditor); err != nil {
		return nil, err
	}
	return s.Store.Tools().TrashedRecords(ctx, collectionID, trashPage)
}

// RestoreRecords puts records back as they were; a unique value taken since refuses them all.
func (s *Service) RestoreRecords(ctx context.Context, collectionID string, ids []string) (int, error) {
	ids, err := checkIDs(ids, false)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := role(ctx, uow, collectionID, domain.RoleEditor); err != nil {
			return err
		}
		raw, err := uow.Collections().SchemaForWrite(ctx, collectionID)
		if err != nil {
			return err
		}
		back, err := uow.Tools().RestoreRecords(ctx, collectionID, ids)
		if err != nil {
			return err
		}
		if keys := schema.UniqueKeys(raw); len(keys) > 0 {
			if err := uow.Records().LockUnique(ctx, collectionID); err != nil {
				return err
			}
			if dup, err := uow.Records().DuplicatedKeys(ctx, collectionID, keys); err != nil || len(dup) > 0 {
				if err == nil {
					err = apperror.New(apperror.CodeConflict, "another record took a unique value of these records: "+strings.Join(dup, ", ")).WithDetails("fields", dup)
				}
				return err
			}
		}
		for id, data := range back {
			d := map[string]any{"collection_id": collectionID}
			if title := schema.Title(raw, data); title != "" {
				d["name"] = title
			}
			if err := policy.Audit(ctx, uow, "record.untrash", id, d); err != nil {
				return err
			}
			if err := policy.Notify(ctx, uow, collectionID, domain.EventRecordCreated, map[string]any{"id": id, "data": data}); err != nil {
				return err
			}
		}
		n = len(back)
		return nil
	})
	return n, err
}

func (s *Service) PurgeRecords(ctx context.Context, collectionID string, ids []string) (int64, error) {
	ids, err := checkIDs(ids, true)
	if err != nil {
		return 0, err
	}
	var n int64
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := role(ctx, uow, collectionID, domain.RoleOwner); err != nil {
			return err
		}
		if n, err = uow.Tools().PurgeRecords(ctx, collectionID, ids); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "record.purge", collectionID, map[string]any{"collection_id": collectionID, "count": n})
	})
	return n, err
}

func (s *Service) TrashedCollections(ctx context.Context) ([]domain.TrashedCollection, error) {
	actor := reqctx.Actor(ctx)
	if actor == "" {
		return nil, apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	return s.Store.Tools().TrashedCollections(ctx, actor)
}

func (s *Service) deletedOwner(ctx context.Context, uow output.UnitOfWork, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return apperror.New(apperror.CodeForbidden, "an API key cannot do this")
	}
	return uow.Tools().TrashedCollectionOwner(ctx, id, reqctx.Actor(ctx))
}

func (s *Service) RestoreCollection(ctx context.Context, id string) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.deletedOwner(ctx, uow, id); err != nil {
			return err
		}
		if err := uow.Tools().RestoreCollection(ctx, id); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.untrash", id, nil)
	})
}

func (s *Service) PurgeCollection(ctx context.Context, id string) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.deletedOwner(ctx, uow, id); err != nil {
			return err
		}
		if err := uow.Tools().PurgeCollection(ctx, id); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.purge", id, nil)
	})
}

func (s *Service) SchemaVersions(ctx context.Context, collectionID string) ([]domain.SchemaVersion, error) {
	if err := role(ctx, s.Store, collectionID, domain.RoleOwner); err != nil {
		return nil, err
	}
	return s.Store.Tools().SchemaVersions(ctx, collectionID, historyPage)
}

func (s *Service) Views(ctx context.Context, collectionID string) ([]domain.SavedView, error) {
	if err := role(ctx, s.Store, collectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	return s.Store.Tools().Views(ctx, collectionID, reqctx.Actor(ctx))
}

func (s *Service) CreateView(ctx context.Context, collectionID, name, query string, shared bool) (*domain.SavedView, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxViewName {
		return nil, apperror.New(apperror.CodeInvalidInput, "name must be 1-80 characters").WithField("name")
	}
	if len(query) > maxViewQuery || strings.ContainsRune(query, 0) {
		return nil, apperror.New(apperror.CodeInvalidInput, "query is too long").WithField("query")
	}
	need := domain.RoleViewer
	if shared { // a view everyone sees is the editors' to make
		need = domain.RoleEditor
	}
	if err := role(ctx, s.Store, collectionID, need); err != nil {
		return nil, err
	}
	v := &domain.SavedView{CollectionID: collectionID, UserID: reqctx.Actor(ctx), Name: name, Query: strings.TrimPrefix(query, "?"), Shared: shared}
	if err := s.Store.Tools().CreateView(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) DeleteView(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	if err := role(ctx, s.Store, collectionID, domain.RoleViewer); err != nil {
		return err
	}
	return s.Store.Tools().DeleteView(ctx, collectionID, id, reqctx.Actor(ctx))
}

func (s *Service) UploadFile(ctx context.Context, collectionID, name, contentType string, data []byte) (*domain.File, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxFileName || strings.ContainsAny(name, "\x00/\\") {
		return nil, apperror.New(apperror.CodeInvalidInput, "file name must be 1-200 characters").WithField("file")
	}
	if len(data) == 0 || len(data) > domain.FileMaxBytes {
		return nil, apperror.New(apperror.CodeInvalidInput, "a file holds 1 byte to 1 MB").WithField("file")
	}
	if contentType == "" || len(contentType) > 100 {
		contentType = "application/octet-stream"
	}
	f := &domain.File{CollectionID: collectionID, Name: name, ContentType: contentType, Size: len(data)}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := role(ctx, uow, collectionID, domain.RoleEditor); err != nil {
			return err
		}
		if err := uow.Tools().CreateFile(ctx, f, reqctx.Actor(ctx)); err != nil {
			return err
		}
		// Written before the commit: a failed write rolls the row back; a failed commit leaves an orphan file.
		// shortcut: no sweep of orphan files; add one when the store grows.
		return s.Files.Put(ctx, fileKeyPrefix+f.ID, data)
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Service) File(ctx context.Context, id string) (*domain.File, []byte, error) {
	if err := validate.ID(id); err != nil {
		return nil, nil, err
	}
	f, err := s.Store.Tools().File(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if err := role(ctx, s.Store, f.CollectionID, domain.RoleViewer); err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, nil, apperror.New(apperror.CodeNotFound, "file not found")
		}
		return nil, nil, err
	}
	data, err := s.Files.Get(ctx, fileKeyPrefix+id)
	if err != nil {
		return nil, nil, err
	}
	return f, data, nil
}
