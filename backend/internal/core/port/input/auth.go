package input

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

type RegisterCommand struct {
	Name  string
	Email string
}

type RegisterResult struct {
	UserID          string
	Status          string
	ActivationToken string // only when AllowManualTokenDelivery, or from ActivationLink
	ManualDelivery  bool
	ExpiresAt       time.Time // set by ActivationLink
}

type ActivateCommand struct {
	Token    string
	Password string
}

type LoginCommand struct {
	Email     string
	Password  string
	IPAddress *string
	UserAgent *string
}

type RefreshCommand struct {
	RefreshToken string
}

type RequestPasswordResetCommand struct {
	Email string
}

type RequestPasswordResetResult struct {
	ResetToken     string
	ManualDelivery bool
}

type ConfirmPasswordResetCommand struct {
	Token    string
	Password string
}

type ChangePasswordCommand struct {
	UserID          string
	SessionID       string // stays signed in; every other session ends
	CurrentPassword string
	NewPassword     string
}

// Authenticator turns an access token into who is calling, refusing dead sessions.
type Authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (*domain.Principal, error)
	ParseAccess(accessToken string) (*domain.Principal, error)
}

// AuthService is the inbound port for identity flows.
type AuthService interface {
	Authenticator
	Register(ctx context.Context, cmd RegisterCommand) (*RegisterResult, error)
	// Invite is Register on behalf of an administrator; a taken email is a conflict.
	Invite(ctx context.Context, cmd RegisterCommand) (*RegisterResult, error)
	// Reinvite sends a pending account a new activation email.
	Reinvite(ctx context.Context, userID string) (*RegisterResult, error)
	// ActivationLink replaces a pending account's activation token without an
	// email and always returns it, for the administrator to hand over.
	ActivationLink(ctx context.Context, userID string) (*RegisterResult, error)
	Activate(ctx context.Context, cmd ActivateCommand) error
	Login(ctx context.Context, cmd LoginCommand) (*domain.TokenPair, error)
	Refresh(ctx context.Context, cmd RefreshCommand) (*domain.TokenPair, error)
	Logout(ctx context.Context, sessionID string) error
	RequestPasswordReset(ctx context.Context, cmd RequestPasswordResetCommand) (*RequestPasswordResetResult, error)
	ConfirmPasswordReset(ctx context.Context, cmd ConfirmPasswordResetCommand) error
	ChangePassword(ctx context.Context, cmd ChangePasswordCommand) error
}
