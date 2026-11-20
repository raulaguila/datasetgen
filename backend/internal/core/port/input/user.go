package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// SetAdminCommand gives or takes the admin role. Password is the actor's own:
// a stolen session alone cannot change who administers the instance.
type SetAdminCommand struct {
	ActorID  string
	UserID   string
	Admin    bool
	Password string
}

// UserService manages the accounts of other people. Users with a protected role
// stay out of reach here, except to be read and to have the admin role set.
type UserService interface {
	List(ctx context.Context, filter domain.UserFilter) ([]domain.User, int64, error)
	Get(ctx context.Context, id string) (*domain.UserDetails, error)
	// DisableMany blocks users and ends their sessions; EnableMany lifts the
	// block. Both change every listed user or none. actorID is who asks.
	DisableMany(ctx context.Context, actorID string, ids []string) error
	EnableMany(ctx context.Context, ids []string) error
	// UnlockMany lifts the account lock and the recent failures of each user.
	UnlockMany(ctx context.Context, ids []string) error
	// RevokeSessions ends every session of another user.
	RevokeSessions(ctx context.Context, actorID, id string) error
	// SetAdmin gives the admin role to an active user or takes it from another admin.
	SetAdmin(ctx context.Context, cmd SetAdminCommand) error
}
