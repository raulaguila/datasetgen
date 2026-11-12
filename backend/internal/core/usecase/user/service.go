// Package user manages other people's accounts at runtime. Users with a protected
// role stay out of reach here: they change only through cmd/admin.
package user

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/raulaguila/dataset/internal/core/policy"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxSearchBytes = 120
	maxBatch       = 100 // ids in one batch request; a page holds at most 100 rows
)

var sortable = []string{"name", "email", "status", "created_at"}

var statuses = []string{domain.UserStatusPending, domain.UserStatusActive, domain.UserStatusLocked, domain.UserStatusDisabled}

type Service struct {
	UOW   output.IdentityStore
	Tx    output.TxManager
	Clock output.Clock
	// FailureWindow is the login email window: the failures Unlock forgets and Get counts.
	FailureWindow time.Duration
}

var _ input.UserService = (*Service)(nil)

func New(uow output.IdentityStore, tx output.TxManager, clock output.Clock, failureWindow time.Duration) *Service {
	return &Service{UOW: uow, Tx: tx, Clock: clock, FailureWindow: failureWindow}
}

func (s *Service) Counts(ctx context.Context) (domain.UserCounts, error) {
	return s.UOW.Users().Counts(ctx)
}

func (s *Service) List(ctx context.Context, filter domain.UserFilter) ([]domain.User, int64, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	if filter.Status != "" && !slices.Contains(statuses, filter.Status) {
		return nil, 0, apperror.New(apperror.CodeInvalidInput, "status must be one of "+strings.Join(statuses, ", ")).WithField("status")
	}
	if filter.RoleID != "" {
		if err := validate.ID(filter.RoleID); err != nil {
			return nil, 0, err
		}
	}
	if err := validate.Sort(&filter.Sort, &filter.Order, sortable, "name", "asc"); err != nil {
		return nil, 0, err
	}
	return s.UOW.Users().List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.UserDetails, error) {
	details, err := policy.Reachable(ctx, s.UOW, id)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	if details.Access, err = s.UOW.Users().Access(ctx, details.ID, now.Add(-s.FailureWindow), now); err != nil {
		return nil, err
	}
	return details, nil
}

// UnlockMany lets users sign in again: it lifts the account lock and forgets the
// recent failures counted against their email. A pending or disabled user stays so.
func (s *Service) UnlockMany(ctx context.Context, ids []string) error {
	return s.each(ctx, ids, func(ctx context.Context, uow output.UnitOfWork, target *domain.UserDetails) error {
		if err := uow.Credentials().ClearLock(ctx, target.ID); err != nil {
			return err
		}
		if target.Status == domain.UserStatusLocked {
			if err := uow.Users().UpdateStatus(ctx, target.ID, domain.UserStatusActive); err != nil {
				return err
			}
		}
		if err := uow.Attempts().ClearFailures(ctx, target.Email, s.Clock.Now().Add(-s.FailureWindow)); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.unlock", target.ID, nil)
	})
}

// DisableMany refuses the caller's own account, compared on the stored id: the
// path may spell the same UUID in another case.
func (s *Service) DisableMany(ctx context.Context, actorID string, ids []string) error {
	return s.each(ctx, ids, func(ctx context.Context, uow output.UnitOfWork, target *domain.UserDetails) error {
		if target.ID == actorID {
			return apperror.New(apperror.CodeForbidden, "your own account cannot be disabled")
		}
		if err := policy.Disable(ctx, uow, target.ID, s.Clock.Now()); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.disable", target.ID, nil)
	})
}

// RevokeSessions ends every session of another user; they must sign in again.
func (s *Service) RevokeSessions(ctx context.Context, actorID, id string) error {
	return s.each(ctx, []string{id}, func(ctx context.Context, uow output.UnitOfWork, target *domain.UserDetails) error {
		if target.ID == actorID {
			return apperror.New(apperror.CodeForbidden, "end your own sessions from your profile")
		}
		if err := uow.Sessions().RevokeAllForUser(ctx, target.ID, s.Clock.Now(), domain.RevokeAdmin); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.sessions_revoke", target.ID, nil)
	})
}

func (s *Service) EnableMany(ctx context.Context, ids []string) error {
	return s.each(ctx, ids, func(ctx context.Context, uow output.UnitOfWork, target *domain.UserDetails) error {
		if err := uow.Users().Enable(ctx, target.ID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.enable", target.ID, nil)
	})
}

// each runs change on every reachable user in one transaction, so one refusal undoes all.
func (s *Service) each(ctx context.Context, ids []string, change func(context.Context, output.UnitOfWork, *domain.UserDetails) error) error {
	if len(ids) == 0 || len(ids) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		for _, id := range slices.Compact(slices.Sorted(slices.Values(ids))) {
			target, err := policy.Reachable(ctx, uow, id)
			if err != nil {
				return err
			}
			if err := change(ctx, uow, target); err != nil {
				return err
			}
		}
		return nil
	})
}
