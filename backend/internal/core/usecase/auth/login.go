package auth

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
)

const settleTimeout = 2 * time.Second

func (s *Service) Login(ctx context.Context, cmd input.LoginCommand) (*domain.TokenPair, error) {
	email := policy.NormalizeEmail(cmd.Email)
	now := s.Clock.Now()

	attemptID, err := s.reserveAttempt(ctx, email, cmd.IPAddress, cmd.UserAgent, now)
	if err != nil {
		return nil, err
	}

	pair, failure, err := s.authenticate(ctx, email, cmd, now)

	// The request may be out of time, and the outcome must still land.
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	switch {
	case pair != nil:
		s.warn("Login attempt not settled", s.UOW.Attempts().Settle(settleCtx, attemptID, true, nil))
	case failure != "":
		s.warn("Login attempt not settled", s.UOW.Attempts().Settle(settleCtx, attemptID, false, &failure))
	default:
		s.warn("Login attempt not released", s.UOW.Attempts().Release(settleCtx, attemptID))
	}
	return pair, err
}

// authenticate returns the failure kind to store for a judged attempt; none
// means no verdict was reached, as on a timeout.
func (s *Service) authenticate(ctx context.Context, email string, cmd input.LoginCommand, now time.Time) (*domain.TokenPair, string, error) {
	// Every refusal without a password check costs one Argon2 run and says the
	// same thing, so neither the answer nor its timing reveals the account state.
	refuse := func(kind string) (*domain.TokenPair, string, error) {
		password.Waste(ctx, cmd.Password)
		if ctx.Err() != nil {
			return nil, "", requestTimedOut()
		}
		return nil, kind, invalidCredentials()
	}

	user, err := s.UOW.Users().FindByEmail(ctx, email)
	if err != nil {
		if apperror.IsNotFound(err) {
			return refuse(domain.FailureUserNotFound)
		}
		return nil, "", err
	}

	if reason, err := s.loginState(ctx, user, now); err != nil {
		return nil, "", err
	} else if reason != "" {
		return refuse(reason)
	}

	cred, err := s.UOW.Credentials().FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	if cred.LockedUntil != nil && cred.LockedUntil.After(now) {
		return refuse(domain.FailureAccountLocked)
	}
	if cred.PasswordHash == nil {
		return refuse(domain.FailureBadPassword)
	}

	ok, err := password.Verify(ctx, *cred.PasswordHash, cmd.Password)
	if ctx.Err() != nil { // running out of time says nothing about the password
		return nil, "", requestTimedOut()
	}
	if err != nil || !ok {
		return s.failedPassword(ctx, user.ID, now)
	}

	if cred.LastLoginAt != nil { // a first login has no client to compare with
		s.noticeNewClient(ctx, user.ID, cmd.IPAddress) // before the session of this login exists
	}
	return s.issueSession(ctx, user.ID, *cred.PasswordHash, cmd.IPAddress, cmd.UserAgent, now)
}

// settleLock reports whether user is under a running lock. A lapsed lock turns
// the account active again, so it needs no login to recover.
func (s *Service) settleLock(ctx context.Context, user *domain.User, now time.Time) (bool, error) {
	cred, err := s.UOW.Credentials().FindByUserID(ctx, user.ID)
	if err != nil {
		return false, err
	}
	if cred.LockedUntil != nil && cred.LockedUntil.After(now) {
		return true, nil
	}
	if err := s.UOW.Users().UpdateStatus(ctx, user.ID, domain.UserStatusActive); err != nil {
		return false, err
	}
	user.Status = domain.UserStatusActive
	return false, nil
}

func invalidCredentials() error {
	return apperror.New(apperror.CodeInvalidCredentials, "invalid credentials")
}

func (s *Service) loginState(ctx context.Context, user *domain.User, now time.Time) (string, error) {
	switch user.Status {
	case domain.UserStatusPending:
		return domain.FailureAccountPending, nil
	case domain.UserStatusDisabled:
		return domain.FailureAccountDisabled, nil
	case domain.UserStatusLocked:
		isLocked, err := s.settleLock(ctx, user, now)
		if err != nil {
			return "", err
		}
		if isLocked {
			return domain.FailureAccountLocked, nil
		}
	case domain.UserStatusActive:
	default:
		return domain.FailureAccountDisabled, nil
	}

	return "", nil
}

func (s *Service) failedPassword(ctx context.Context, userID string, now time.Time) (*domain.TokenPair, string, error) {
	lockedUntil, err := s.UOW.Credentials().RecordFailure(ctx, userID, s.Cfg.MaxFailedLoginAttempts, now.Add(s.Cfg.AccountLockDuration), now)
	if err != nil {
		return nil, "", err
	}
	if lockedUntil != nil {
		if err := s.UOW.Users().UpdateStatus(ctx, userID, domain.UserStatusLocked); err != nil {
			return nil, domain.FailureBadPassword, err
		}
		until := lockedUntil.UTC().Format(time.RFC3339)
		s.notify(ctx, domain.Notification{
			UserID: userID, Level: domain.NotificationWarning, Kind: domain.NotifyAccountLocked, Params: map[string]string{"until": until},
			Title: "Account locked", Message: "Too many failed logins. Login stays blocked until " + until + ".",
		})
	}
	return nil, domain.FailureBadPassword, invalidCredentials()
}
