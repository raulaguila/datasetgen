package policy

import (
	"context"
	"slices"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

// Audit records an administrative change in the caller's transaction, with the
// actor and request id the context carries.
func Audit(ctx context.Context, uow output.UnitOfWork, action, targetID string, details map[string]any) error {
	return uow.Audit().Record(ctx, domain.AuditEntry{
		ActorID:   reqctx.Actor(ctx),
		RequestID: reqctx.RequestID(ctx),
		Action:    action,
		TargetID:  targetID,
		Details:   details,
	})
}

// Reachable loads a user the API may see or change. Nothing in the API can give
// a user a protected role, so the answer cannot go stale before a write.
func Reachable(ctx context.Context, uow output.IdentityStore, id string) (*domain.UserDetails, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	user, err := uow.Users().FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	roles, err := uow.Roles().ForUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(roles, domain.Role.Protected) {
		return nil, apperror.New(apperror.CodeForbidden, "a user with a superuser role cannot be managed here")
	}
	return &domain.UserDetails{User: *user, Roles: roles}, nil
}

// Disable blocks a user and revokes their sessions, so access tokens stop at the
// next request: Bearer checks the session every time.
func Disable(ctx context.Context, uow output.UnitOfWork, id string, now time.Time) error {
	if err := uow.Users().UpdateStatus(ctx, id, domain.UserStatusDisabled); err != nil {
		return err
	}
	return uow.Sessions().RevokeAllForUser(ctx, id, now, domain.RevokeAccountDisabled)
}

// AssignDefaultRoles gives a new account every enabled default role.
func AssignDefaultRoles(ctx context.Context, uow output.IdentityStore, userID string) error {
	ids, err := uow.Roles().DefaultRoleIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := uow.Roles().Assign(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}
