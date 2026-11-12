package collection

import (
	"context"
	"slices"
	"strings"
	"unicode"
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
	optionsCap     = 1000
	maxSearchBytes = 120
	maxDescription = 2000
	maxBatch       = 100 // ids in one batch request; a page holds at most 100 rows
)

var sortable = []string{"name", "created_at", "updated_at"}

// Inviter creates a pending account for an email that has none.
type Inviter interface {
	Invite(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error)
}

// Service reads through Repo. Every change runs in Tx with its audit record.
// Access follows the caller's role in each collection (policy.CollectionRole).
type Service struct {
	Repo    output.CollectionRepository
	Records output.RecordRepository
	Tx      output.TxManager
	Inviter Inviter
	Audit   AuditReader
}

// AuditReader lists the audit log; the audit use case checks its filters.
type AuditReader interface {
	List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error)
}

var _ input.CollectionService = (*Service)(nil)

func New(repo output.CollectionRepository, records output.RecordRepository, tx output.TxManager, inviter Inviter, audit AuditReader) *Service {
	return &Service{Repo: repo, Records: records, Tx: tx, Inviter: inviter, Audit: audit}
}

func (s *Service) List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	if err := validate.Sort(&filter.Sort, &filter.Order, sortable, "name", "asc"); err != nil {
		return nil, 0, err
	}
	filter.Page = domain.NewPage(filter.Page.Number, filter.Page.Limit)
	if filter.UserID = reqctx.Actor(ctx); filter.UserID == "" {
		return nil, 0, apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	return s.Repo.List(ctx, filter)
}

func (s *Service) Options(ctx context.Context) ([]domain.Collection, error) {
	actor := reqctx.Actor(ctx)
	if actor == "" {
		return nil, apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	return s.Repo.ListOptions(ctx, actor, optionsCap)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Collection, error) {
	role, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleViewer)
	if err != nil {
		return nil, err
	}
	c, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Role = role
	return c, nil
}

func (s *Service) Create(ctx context.Context, cmd input.CreateCollectionCommand) (*domain.Collection, error) {
	name, err := normalizeName(cmd.Name)
	if err != nil {
		return nil, err
	}
	description, err := normalizeDescription(cmd.Description)
	if err != nil {
		return nil, err
	}
	if _, err := schema.Compile(cmd.Schema); err != nil {
		return nil, err
	}
	actor := reqctx.Actor(ctx)
	if actor == "" {
		return nil, apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	if !domain.ValidCollectionIcon(cmd.Icon) {
		return nil, invalidIcon()
	}
	c := &domain.Collection{Name: name, Description: description, Icon: cmd.Icon, Schema: cmd.Schema}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Collections().Create(ctx, c); err != nil {
			return err
		}
		// The creator owns it, so it shows in their list and they can add helpers.
		if err := uow.Collections().AddMember(ctx, c.ID, actor, domain.RoleOwner); err != nil {
			return err
		}
		c.Role = domain.RoleOwner
		return policy.Audit(ctx, uow, "collection.create", c.ID, map[string]any{"name": c.Name})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Update accepts a new schema without checking the stored records against it.
// shortcut: records that no longer match are only caught on their next edit; add a
// "validate all" job when collections grow past what one person reviews by hand.
func (s *Service) Update(ctx context.Context, cmd input.UpdateCollectionCommand) (*domain.Collection, error) {
	if err := validate.ID(cmd.ID); err != nil {
		return nil, err
	}
	patch := domain.CollectionPatch{Version: cmd.Version}
	changes := map[string]any{}
	if cmd.Name != nil {
		name, err := normalizeName(*cmd.Name)
		if err != nil {
			return nil, err
		}
		patch.Name = &name
		changes["name"] = name
	}
	if cmd.Description != nil {
		description, err := normalizeDescription(*cmd.Description)
		if err != nil {
			return nil, err
		}
		patch.Description = &description
	}
	if cmd.Icon != nil {
		if !domain.ValidCollectionIcon(*cmd.Icon) {
			return nil, invalidIcon()
		}
		patch.Icon = cmd.Icon
		changes["icon"] = *cmd.Icon
	}
	if cmd.Schema != nil {
		if _, err := schema.Compile(cmd.Schema); err != nil {
			return nil, err
		}
		patch.Schema = cmd.Schema
		changes["schema"] = true
	}
	var updated *domain.Collection
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		var err error
		role, err := policy.CollectionRole(ctx, uow.Collections(), cmd.ID, domain.RoleOwner)
		if err != nil {
			return err
		}
		if updated, err = uow.Collections().Update(ctx, cmd.ID, patch); err != nil {
			return err
		}
		updated.Role = role
		changes["name"] = updated.Name // the audit log labels the record by name
		return policy.Audit(ctx, uow, "collection.update", updated.ID, changes)
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
	ids = slices.Compact(slices.Sorted(slices.Values(ids))) // a repeated id would fail the count check
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		for _, id := range ids {
			if _, err := policy.CollectionRole(ctx, uow.Collections(), id, domain.RoleOwner); err != nil {
				return err
			}
		}
		names, err := uow.Collections().DeleteMany(ctx, ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := policy.Audit(ctx, uow, "collection.delete", id, map[string]any{"name": names[id]}); err != nil {
				return err
			}
		}
		return nil
	})
}

func invalidIcon() error {
	return apperror.New(apperror.CodeInvalidInput, "icon is not one of the offered icons").WithField("icon")
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 120 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", apperror.New(apperror.CodeInvalidInput, "name must be 1-120 characters").WithField("name")
	}
	return name, nil
}

func normalizeDescription(d string) (string, error) {
	d = strings.TrimSpace(d)
	if utf8.RuneCountInString(d) > maxDescription {
		return "", apperror.New(apperror.CodeInvalidInput, "description must be at most 2000 characters").WithField("description")
	}
	return d, nil
}
