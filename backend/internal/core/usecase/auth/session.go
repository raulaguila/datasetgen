package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/tokenhash"
)

// issueSession returns the failure kind to store when it refuses the login.
func (s *Service) issueSession(ctx context.Context, userID, verifiedHash string, ip, ua *string, now time.Time) (*domain.TokenPair, string, error) {
	// Pre-generate session id so JWT sid matches the row on insert.
	sessionID := uuid.NewString()
	refresh, jti, err := s.Tokens.IssueRefresh(userID, sessionID, now)
	if err != nil {
		return nil, "", apperror.New(apperror.CodeInternal, "token issue failed").WithCause(err)
	}
	access, err := s.Tokens.IssueAccess(userID, sessionID, now)
	if err != nil {
		return nil, "", apperror.New(apperror.CodeInternal, "token issue failed").WithCause(err)
	}

	expiresAt := now.Add(min(s.Tokens.RefreshTTL(), s.Cfg.SessionMaxLifetime))
	sess := &domain.Session{
		ID:               sessionID,
		UserID:           userID,
		ExpiresAt:        expiresAt,
		RefreshTokenHash: tokenhash.SHA256Bytes(jti),
		IPAddress:        ip,
		UserAgent:        ua,
		CreatedAt:        now,
		LastSeenAt:       now,
	}
	// RecordSuccess locks the credential row until the session commits. Reset
	// updates that row before revoking sessions, so it wins first (the hash check
	// fails) or revokes this session afterwards. LockStatus does the same with
	// Disable, which updates the status before revoking sessions.
	var refused string
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		applied, err := uow.Credentials().RecordSuccess(ctx, userID, verifiedHash, now)
		if err != nil {
			return err
		}
		if !applied {
			refused = domain.FailureBadPassword
			return invalidCredentials()
		}
		status, err := uow.Users().LockStatus(ctx, userID)
		if err != nil {
			return err
		}
		if status == domain.UserStatusDisabled {
			refused = domain.FailureAccountDisabled
			return invalidCredentials()
		}
		return uow.Sessions().Create(ctx, sess)
	})
	if err != nil {
		return nil, refused, err
	}

	return &domain.TokenPair{
		UserID:       userID,
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.Tokens.AccessTTL().Seconds()),
		TokenType:    "Bearer",
	}, "", nil
}
