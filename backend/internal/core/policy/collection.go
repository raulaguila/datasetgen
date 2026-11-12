package policy

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

// CollectionAccessor answers a user's role in a collection.
type CollectionAccessor interface {
	Access(ctx context.Context, id, userID string) (domain.MemberRole, error)
}

// CollectionRole checks that the caller holds at least need in the collection
// and returns their role. A non-member gets NotFound, so a collection's
// existence does not leak; a member with too small a role gets Forbidden.
func CollectionRole(ctx context.Context, repo CollectionAccessor, id string, need domain.MemberRole) (domain.MemberRole, error) {
	if err := validate.ID(id); err != nil {
		return "", err
	}
	actor := reqctx.Actor(ctx)
	if actor == "" {
		return "", apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	role, err := repo.Access(ctx, id, actor)
	if err != nil {
		return "", err
	}
	if !role.Allows(need) {
		return "", apperror.New(apperror.CodeForbidden, "your role in this collection does not allow this")
	}
	return role, nil
}
