package output

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// PermissionReader loads effective RBAC state for a user (Postgres view +
// superuser) and the resource catalog.
type PermissionReader interface {
	LoadEffective(ctx context.Context, userID string) (*domain.EffectivePermissions, error)
	// Access reads the session and its user's action mask on one resource
	// (every bit for a superuser role or a '*' grant). A missing session is NotFound.
	Access(ctx context.Context, sessionID, resource string) (*domain.Session, domain.Action, error)
	ListResources(ctx context.Context) ([]domain.Resource, error)
}
