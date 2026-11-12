package auth

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.UOW.Sessions().Revoke(ctx, sessionID, s.Clock.Now(), domain.RevokeUserLogout)
}

// Authenticate checks the token and its session on every request, so a revoked
// session stops at once instead of when the token expires.
func (s *Service) Authenticate(ctx context.Context, accessToken string) (*domain.Principal, error) {
	p, err := s.ParseAccess(accessToken)
	if err != nil {
		return nil, err
	}
	sess, err := s.UOW.Sessions().FindLive(ctx, p.SessionID)
	if apperror.IsNotFound(err) {
		return nil, apperror.New(apperror.CodeUnauthorized, "session not found")
	}
	if err != nil {
		return nil, err
	}
	if err := policy.CheckSession(sess, p.UserID, s.Clock.Now()); err != nil {
		return nil, err
	}
	return p, nil
}

// ParseAccess checks the token alone, without the database. Routes behind
// Require check the session in the same query as the permission.
func (s *Service) ParseAccess(accessToken string) (*domain.Principal, error) {
	claims, err := s.Tokens.ParseAccess(accessToken)
	if err != nil {
		return nil, apperror.New(apperror.CodeUnauthorized, "invalid access token").WithCause(err)
	}
	if claims.UserID == "" || claims.SessionID == "" {
		return nil, apperror.New(apperror.CodeUnauthorized, "invalid access token")
	}
	if !claims.ExpiresAt.IsZero() && !claims.ExpiresAt.After(s.Clock.Now()) {
		return nil, apperror.New(apperror.CodeTokenExpired, "access token expired")
	}
	return &domain.Principal{UserID: claims.UserID, SessionID: claims.SessionID}, nil
}
