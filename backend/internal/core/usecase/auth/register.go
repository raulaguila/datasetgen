package auth

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/policy"

	"github.com/google/uuid"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
	"github.com/raulaguila/dataset/pkg/tokenhash"
)

func (s *Service) Register(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	if !s.Cfg.AllowRegistration {
		return nil, apperror.New(apperror.CodeForbidden, "registration is closed; ask an administrator for an invitation")
	}
	name, email, err := policy.ValidateIdentity(cmd.Name, cmd.Email)
	if err != nil {
		return nil, err
	}

	plain, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token generation failed").WithCause(err)
	}

	// Known and new emails get the same answer, so this route reveals no account.
	res := &input.RegisterResult{UserID: uuid.NewString(), Status: domain.UserStatusPending}
	issued := false
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		u, findErr := uow.Users().FindByEmail(ctx, email)
		switch {
		case findErr == nil:
			if u.Status != domain.UserStatusPending {
				return nil
			}
			if cooling, err := s.emailThrottled(ctx, uow, u.ID, domain.TokenPurposeActivation); cooling || err != nil {
				return err
			}
			// Unactivated account: send a fresh token instead of a dead end.
		case apperror.IsNotFound(findErr):
			u = &domain.User{Name: name, Email: email, Status: domain.UserStatusPending}
			if err := uow.Users().Create(ctx, u); err != nil {
				return err
			}
			if err := uow.Credentials().Create(ctx, u.ID); err != nil {
				return err
			}
			res.UserID = u.ID
		default:
			return findErr
		}
		issued = true
		return s.issueEmailToken(ctx, uow, u, domain.TokenPurposeActivation, plain, s.Cfg.RegistrationTokenTTL)
	})
	if apperror.IsCode(err, apperror.CodeAlreadyExists) { // lost a race with the same signup
		return res, nil
	}
	if err != nil {
		return nil, err
	}

	if issued && s.Cfg.AllowManualTokenDelivery {
		res.ActivationToken = plain
		res.ManualDelivery = true
	}
	return res, nil
}

func (s *Service) Activate(ctx context.Context, cmd input.ActivateCommand) error {
	if err := policy.ValidatePassword(cmd.Password); err != nil {
		return err
	}
	hash := tokenhash.SHA256Bytes(cmd.Token)
	now := s.Clock.Now()

	_, target, err := activationTarget(ctx, s.UOW, hash, now)
	if err != nil {
		return err
	}
	if err := policy.ValidatePasswordFor(cmd.Password, target.Name, target.Email); err != nil {
		return err
	}

	// Hashed before the transaction: Argon2 must not hold a database connection.
	ph, err := password.Hash(ctx, cmd.Password)
	if err != nil {
		return hashError(ctx, err)
	}

	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		tok, user, err := activationTarget(ctx, uow, hash, now)
		if err != nil {
			return err
		}
		if err := uow.Credentials().SetPassword(ctx, user.ID, ph); err != nil {
			return err
		}
		if err := uow.Users().UpdateStatus(ctx, user.ID, domain.UserStatusActive); err != nil {
			return err
		}
		if err := policy.AssignDefaultRoles(ctx, uow, user.ID); err != nil {
			return err
		}
		if err := policy.GiveStarterCollection(ctx, uow, user.ID); err != nil {
			return err
		}
		return uow.Tokens().MarkUsed(ctx, tok.ID, now)
	})
	return err
}

// activationTarget checks an activation token.
func activationTarget(ctx context.Context, uow output.UnitOfWork, hash []byte, now time.Time) (*domain.EmailToken, *domain.User, error) {
	tok, err := uow.Tokens().FindActiveByHash(ctx, domain.TokenPurposeActivation, hash)
	if err != nil {
		if apperror.IsNotFound(err) {
			return nil, nil, apperror.New(apperror.CodeNotFound, "invalid or used activation token")
		}
		return nil, nil, err
	}
	if tok.UsedAt != nil {
		return nil, nil, apperror.New(apperror.CodeTokenExpired, "activation token already used")
	}
	if !tok.ExpiresAt.After(now) {
		return nil, nil, apperror.New(apperror.CodeTokenExpired, "activation token expired")
	}
	user, err := uow.Users().FindByID(ctx, tok.UserID)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != domain.UserStatusPending {
		return nil, nil, apperror.New(apperror.CodeConflict, "account cannot be activated")
	}
	return tok, user, nil
}

// Invite creates a pending account for someone and sends them the activation
// email. Unlike Register it tells the administrator when the email is taken.
func (s *Service) Invite(ctx context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	name, email, err := policy.ValidateIdentity(cmd.Name, cmd.Email)
	if err != nil {
		return nil, err
	}
	plain, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token generation failed").WithCause(err)
	}
	u := &domain.User{Name: name, Email: email, Status: domain.UserStatusPending}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := uow.Credentials().Create(ctx, u.ID); err != nil {
			return err
		}
		if err := s.issueEmailToken(ctx, uow, u, domain.TokenPurposeActivation, plain, s.Cfg.RegistrationTokenTTL); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.invite", u.ID, nil)
	})
	if apperror.IsCode(err, apperror.CodeAlreadyExists) {
		return nil, apperror.New(apperror.CodeAlreadyExists, "an account with this email already exists").WithField("email")
	}
	if err != nil {
		return nil, err
	}
	res := &input.RegisterResult{UserID: u.ID, Status: u.Status}
	if s.Cfg.AllowManualTokenDelivery {
		res.ActivationToken, res.ManualDelivery = plain, true
	}
	return res, nil
}

// Reinvite sends a fresh activation email to an account that never set a
// password, for when the first one was lost. The old token stops working.
func (s *Service) Reinvite(ctx context.Context, userID string) (*input.RegisterResult, error) {
	res, err := s.renewActivation(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	if !s.Cfg.AllowManualTokenDelivery {
		res.ActivationToken = ""
	}
	return res, nil
}

// ActivationLink is Reinvite without the email: the administrator gets the
// token to hand over (chat, phone) when the mail never arrived or the link
// expired. Whoever holds it sets the password, so it is audited apart and
// shares the email throttle.
func (s *Service) ActivationLink(ctx context.Context, userID string) (*input.RegisterResult, error) {
	return s.renewActivation(ctx, userID, false)
}

// renewActivation replaces a pending account's activation token, sending it by
// email or not, and returns it.
func (s *Service) renewActivation(ctx context.Context, userID string, send bool) (*input.RegisterResult, error) {
	plain, err := tokenhash.RandomURLSafe()
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "token generation failed").WithCause(err)
	}
	var expires time.Time
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		target, err := policy.Reachable(ctx, uow, userID)
		if err != nil {
			return err
		}
		if target.Status != domain.UserStatusPending {
			return apperror.New(apperror.CodeConflict, "the account is already activated")
		}
		cooling, err := s.emailThrottled(ctx, uow, userID, domain.TokenPurposeActivation)
		if err != nil {
			return err
		}
		if cooling {
			return apperror.New(apperror.CodeTooManyRequests, "an activation code was issued a moment ago").
				WithDetails(apperror.DetailRetryAfter, int(s.Cfg.EmailTokenCooldown.Seconds()))
		}
		action := "user.activation_link"
		if send {
			action = "user.reinvite"
			err = s.issueEmailToken(ctx, uow, &target.User, domain.TokenPurposeActivation, plain, s.Cfg.RegistrationTokenTTL)
			expires = s.Clock.Now().Add(s.Cfg.RegistrationTokenTTL)
		} else {
			expires, err = s.replaceToken(ctx, uow, userID, domain.TokenPurposeActivation, plain, s.Cfg.RegistrationTokenTTL)
		}
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, action, userID, nil)
	})
	if err != nil {
		return nil, err
	}
	return &input.RegisterResult{
		UserID: userID, Status: domain.UserStatusPending,
		ActivationToken: plain, ManualDelivery: send && s.Cfg.AllowManualTokenDelivery, ExpiresAt: expires,
	}, nil
}
