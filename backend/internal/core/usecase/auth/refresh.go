package auth

import (
	"bytes"
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/tokenhash"
)

// Two tabs may refresh together; older reuse revokes the session.
const refreshRaceWindow = 10 * time.Second

func (s *Service) Refresh(ctx context.Context, cmd input.RefreshCommand) (*domain.TokenPair, error) {
	claims, err := s.Tokens.ParseRefresh(cmd.RefreshToken)
	if err != nil {
		return nil, apperror.New(apperror.CodeUnauthorized, "invalid refresh token").WithCause(err)
	}
	jtiHash := tokenhash.SHA256Bytes(claims.JTI)
	now := s.Clock.Now()

	sess, err := s.refreshSession(ctx, claims, jtiHash, now)
	if err != nil {
		return nil, err
	}

	user, err := s.UOW.Users().FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	// A lock stops password guessing, not a session already proven: otherwise
	// anyone who knows the email could log the owner out at will.
	if user.Status != domain.UserStatusActive && user.Status != domain.UserStatusLocked {
		return nil, apperror.New(apperror.CodeForbidden, "login not allowed")
	}

	newRefresh, newJTI, err := s.Tokens.IssueRefresh(user.ID, sess.ID, now)
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token issue failed").WithCause(err)
	}
	access, err := s.Tokens.IssueAccess(user.ID, sess.ID, now)
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token issue failed").WithCause(err)
	}

	expiresAt := now.Add(s.Tokens.RefreshTTL())
	if limit := sess.CreatedAt.Add(s.Cfg.SessionMaxLifetime); expiresAt.After(limit) {
		expiresAt = limit
	}
	rotated, err := s.UOW.Sessions().Rotate(ctx, sess.ID, jtiHash, tokenhash.SHA256Bytes(newJTI), expiresAt, now)
	if err != nil {
		return nil, err
	}
	if !rotated {
		return nil, refreshRaceLost()
	}

	return &domain.TokenPair{
		UserID:       user.ID,
		AccessToken:  access,
		RefreshToken: newRefresh,
		ExpiresIn:    int64(s.Tokens.AccessTTL().Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// refreshRaceLost answers the loser of two simultaneous refreshes. It gets no
// token, so a thief gains nothing; the client reads the winner's token and retries.
func refreshRaceLost() error {
	return apperror.New(apperror.CodeConflict, "refresh already in progress")
}

func (s *Service) revokeForReuse(ctx context.Context, sess *domain.Session, now time.Time) error {
	if err := s.UOW.Sessions().Revoke(ctx, sess.ID, now, domain.RevokeTokenReuse); err != nil {
		return err
	}
	s.notify(ctx, domain.Notification{
		UserID: sess.UserID, Level: domain.NotificationCritical, Kind: domain.NotifySessionReused, Title: "Session ended",
		Message: "An old refresh token was used again, so that session was ended. If it was not you, change your password.",
	})
	return apperror.New(apperror.CodeUnauthorized, "refresh token reuse detected")
}

func (s *Service) refreshSession(ctx context.Context, claims *output.RefreshClaims, jtiHash []byte, now time.Time) (*domain.Session, error) {
	sess, err := s.UOW.Sessions().FindByID(ctx, claims.SessionID)
	if err != nil {
		if apperror.IsNotFound(err) {
			return nil, apperror.New(apperror.CodeUnauthorized, "invalid refresh token")
		}
		return nil, err
	}
	if sess.UserID != claims.UserID {
		return nil, apperror.New(apperror.CodeUnauthorized, "invalid refresh token")
	}
	if sess.RevokedAt != nil {
		return nil, apperror.New(apperror.CodeUnauthorized, "session revoked")
	}
	if !sess.ExpiresAt.After(now) {
		if err := s.UOW.Sessions().Revoke(ctx, sess.ID, now, domain.RevokeExpired); err != nil {
			return nil, err
		}
		return nil, apperror.New(apperror.CodeTokenExpired, "session expired")
	}

	// A genuinely signed token for this session that is not the live one has
	// already been spent, however many rotations ago.
	if !bytes.Equal(sess.RefreshTokenHash, jtiHash) {
		if bytes.Equal(sess.PreviousRefreshTokenHash, jtiHash) && now.Sub(sess.LastSeenAt) < refreshRaceWindow {
			return nil, refreshRaceLost()
		}
		return nil, s.revokeForReuse(ctx, sess, now)
	}

	return sess, nil
}
