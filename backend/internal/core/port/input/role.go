package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// RoleService manages roles at runtime. Protected roles are read-only here.
type RoleService interface {
	List(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, int64, error)
	Get(ctx context.Context, id string) (*domain.Role, error)
	Create(ctx context.Context, name string) (*domain.Role, error)
	// DeleteMany deletes every listed role or none.
	DeleteMany(ctx context.Context, ids []string) error
	Update(ctx context.Context, id string, patch domain.RolePatch) (*domain.Role, error)
	// SetPermissions replaces every permission of a role at once.
	SetPermissions(ctx context.Context, roleID string, permissions map[string]domain.Action) (*domain.Role, error)
	Members(ctx context.Context, roleID string, filter domain.UserFilter) ([]domain.User, int64, error) // Page and Search only
	AddMember(ctx context.Context, roleID, userID string) error
	RemoveMember(ctx context.Context, roleID, userID string) error
	// SetUserRoles replaces every role of a user at once.
	SetUserRoles(ctx context.Context, userID string, roleIDs []string) error
}
