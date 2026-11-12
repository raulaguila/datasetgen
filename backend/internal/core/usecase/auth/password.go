package auth

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/policy"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
	"github.com/raulaguila/dataset/pkg/tokenhash"
)

// RequestPasswordReset answers no sooner than Cfg.PasswordResetMinDuration: an
// unknown email does no work, and answering at once would tell it from a known one.
func (s *Service) RequestPasswordReset(ctx context.Context, cmd input.RequestPasswordResetCommand) (*input.RequestPasswordResetResult, error) {
	wait := time.NewTimer(s.Cfg.PasswordResetMinDuration)
	defer func() {
		select {
		case <-wait.C:
		case <-ctx.Done():
			wait.Stop()
		}
	}()
	email := policy.NormalizeEmail(cmd.Email)
	res := &input.RequestPasswordResetResult{}

	user, err := s.UOW.Users().FindByEmail(ctx, email)
	if err != nil {
		if apperror.IsNotFound(err) {
			// do not reveal existence
			return res, nil
		}
		return nil, err
	}
	if user.Status != domain.UserStatusActive && user.Status != domain.UserStatusLocked {
		return res, nil
	}

	plain, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token generation failed").WithCause(err)
	}

	issued := false
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if cooling, err := s.emailThrottled(ctx, uow, user.ID, domain.TokenPurposePasswordReset); cooling || err != nil {
			return err
		}
		issued = true
		return s.issueEmailToken(ctx, uow, user, domain.TokenPurposePasswordReset, plain, s.Cfg.PasswordResetTokenTTL)
	})
	if apperror.IsCode(err, apperror.CodeAlreadyExists) { // lost a race with the same request
		return res, nil
	}
	if err != nil {
		return nil, err
	}

	if issued && s.Cfg.AllowManualTokenDelivery {
		res.ResetToken = plain
		res.ManualDelivery = true
	}
	return res, nil
}

func (s *Service) ConfirmPasswordReset(ctx context.Context, cmd input.ConfirmPasswordResetCommand) error {
	if err := policy.ValidatePassword(cmd.Password); err != nil {
		return err
	}
	hash := tokenhash.SHA256Bytes(cmd.Token)
	now := s.Clock.Now()

	tok, err := resetToken(ctx, s.UOW, hash, now)
	if err != nil {
		return err
	}
	owner, err := s.UOW.Users().FindByID(ctx, tok.UserID)
	if err != nil {
		return err
	}
	if err := policy.ValidatePasswordFor(cmd.Password, owner.Name, owner.Email); err != nil {
		return err
	}

	// Hashed before the transaction: Argon2 must not hold a database connection.
	ph, err := password.Hash(ctx, cmd.Password)
	if err != nil {
		return hashError(ctx, err)
	}

	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		tok, err := resetToken(ctx, uow, hash, now)
		if err != nil {
			return err
		}
		if err := uow.Credentials().SetPassword(ctx, tok.UserID, ph); err != nil {
			return err
		}
		user, err := uow.Users().FindByID(ctx, tok.UserID)
		if err != nil {
			return err
		}
		if user.Status == domain.UserStatusLocked {
			if err := uow.Users().UpdateStatus(ctx, user.ID, domain.UserStatusActive); err != nil {
				return err
			}
		}
		if err := uow.Sessions().RevokeAllForUser(ctx, tok.UserID, now, domain.RevokePasswordChanged); err != nil {
			return err
		}
		if err := uow.Attempts().ClearFailures(ctx, user.Email, now.Add(-s.Cfg.LoginAttemptEmailWindow)); err != nil {
			return err
		}
		if err := uow.Notifications().Create(ctx, &domain.Notification{
			UserID:  tok.UserID,
			Level:   domain.NotificationWarning,
			Kind:    domain.NotifyPasswordReset,
			Title:   "Password changed",
			Message: "Your password was changed and every session was ended.",
		}); err != nil {
			return err
		}
		return uow.Tokens().MarkUsed(ctx, tok.ID, now)
	})
}

// ChangePassword replaces the password of a signed in user who knows the
// current one, and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, cmd input.ChangePasswordCommand) error {
	owner, err := s.UOW.Users().FindByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	if err := policy.ValidatePasswordFor(cmd.NewPassword, owner.Name, owner.Email); err != nil {
		return apperror.New(apperror.CodeInvalidInput, err.Error()).WithField("new_password")
	}
	cred, err := s.UOW.Credentials().FindByUserID(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	wrong := apperror.New(apperror.CodeInvalidInput, "current password is wrong").WithField("current_password")
	if cred.PasswordHash == nil {
		return wrong
	}
	ok, err := password.Verify(ctx, *cred.PasswordHash, cmd.CurrentPassword)
	if err != nil {
		return hashError(ctx, err)
	}
	if !ok {
		return wrong
	}
	ph, err := password.Hash(ctx, cmd.NewPassword)
	if err != nil {
		return hashError(ctx, err)
	}
	now := s.Clock.Now()
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Credentials().SetPassword(ctx, cmd.UserID, ph); err != nil {
			return err
		}
		if err := uow.Sessions().RevokeOthers(ctx, cmd.UserID, cmd.SessionID, now, domain.RevokePasswordChanged); err != nil {
			return err
		}
		return uow.Notifications().Create(ctx, &domain.Notification{
			UserID:  cmd.UserID,
			Level:   domain.NotificationWarning,
			Kind:    domain.NotifyPasswordChanged,
			Title:   "Password changed",
			Message: "Your password was changed and your other sessions were ended.",
		})
	})
}

// resetToken checks a reset token.
func resetToken(ctx context.Context, uow output.UnitOfWork, hash []byte, now time.Time) (*domain.EmailToken, error) {
	tok, err := uow.Tokens().FindActiveByHash(ctx, domain.TokenPurposePasswordReset, hash)
	if err != nil {
		if apperror.IsNotFound(err) {
			return nil, apperror.New(apperror.CodeNotFound, "invalid or used reset token")
		}
		return nil, err
	}
	if tok.UsedAt != nil || !tok.ExpiresAt.After(now) {
		return nil, apperror.New(apperror.CodeTokenExpired, "reset token expired or used")
	}
	return tok, nil
}

// hashError tells a request that ran out of time from a real hashing failure.
func hashError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return requestTimedOut()
	}
	return apperror.New(apperror.CodeInternal, "password hash failed").WithCause(err)
}

func requestTimedOut() error {
	return apperror.New(apperror.CodeTimedOut, "request timed out")
}
