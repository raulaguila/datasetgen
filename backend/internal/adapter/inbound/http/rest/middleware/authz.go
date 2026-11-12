package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

const (
	LocalUserID    = "auth.user_id"
	LocalSessionID = "auth.session_id"
)

// Bearer authenticates the request's access token and its session.
func Bearer(authn input.Authenticator) fiber.Handler {
	return bearer(func(ctx context.Context, token string) (*domain.Principal, error) {
		return authn.Authenticate(ctx, token)
	})
}

// Token checks the access token alone. Only for groups whose every route runs
// Require, which checks the session with the permission in one query.
func Token(authn input.Authenticator) fiber.Handler {
	return bearer(func(_ context.Context, token string) (*domain.Principal, error) { return authn.ParseAccess(token) })
}

func bearer(check func(context.Context, string) (*domain.Principal, error)) fiber.Handler {
	return func(c fiber.Ctx) error {
		const prefix = "Bearer "
		header := c.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		if !strings.HasPrefix(header, prefix) || token == "" {
			return mapper.WriteError(c, apperror.New(apperror.CodeUnauthorized, "missing bearer token"))
		}
		principal, err := check(c.Context(), token)
		if err != nil {
			return mapper.WriteError(c, err)
		}
		c.Locals(LocalUserID, principal.UserID)
		c.Locals(LocalSessionID, principal.SessionID)
		c.SetContext(reqctx.WithActor(c.Context(), principal.UserID))
		return c.Next()
	}
}

// Require checks the session and enforces a CRUD action on a resource type
// (ADR-004). Must run after Token or Bearer.
func Require(authz input.Authorizer, resource string, need domain.Action) fiber.Handler {
	return func(c fiber.Ctx) error {
		p := domain.Principal{UserID: UserID(c), SessionID: SessionID(c)}
		if p.UserID == "" {
			return mapper.WriteError(c, apperror.New(apperror.CodeUnauthorized, "unauthenticated"))
		}
		if err := authz.Authorize(c.Context(), p, resource, need); err != nil {
			return mapper.WriteError(c, err)
		}
		return c.Next()
	}
}

// UserID returns the authenticated user id from locals.
func UserID(c fiber.Ctx) string {
	userID, _ := c.Locals(LocalUserID).(string)
	return userID
}

// SessionID returns the authenticated session id from locals.
func SessionID(c fiber.Ctx) string {
	sessionID, _ := c.Locals(LocalSessionID).(string)
	return sessionID
}
