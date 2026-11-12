// Package admin holds operator actions that no HTTP route exposes yet.
package admin

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
)

const adminRole = "admin"

// Service implements the commands of cmd/admin.
type Service struct {
	Tx    output.TxManager
	Clock output.Clock
}

func New(tx output.TxManager, clock output.Clock) *Service {
	return &Service{Tx: tx, Clock: clock}
}

// Bootstrap creates the first admin, active and ready to log in, with no email
// involved. Once any admin exists it does nothing and returns false. An email
// already in use is an error: someone else may have registered it.
func (s *Service) Bootstrap(ctx context.Context, cmd input.BootstrapCommand) (bool, error) {
	name, email, err := policy.ValidateIdentity(cmd.Name, cmd.Email)
	if err != nil {
		return false, err
	}
	if err := policy.ValidatePassword(cmd.Password); err != nil {
		return false, err
	}
	// Hashed before the transaction: Argon2 must not hold a database connection.
	hash, err := password.Hash(ctx, cmd.Password)
	if err != nil {
		return false, apperror.New(apperror.CodeInternal, "password hash failed").WithCause(err)
	}
	created := false
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Lock(ctx, "admin:bootstrap"); err != nil { // two runs at once make one admin
			return err
		}
		adminID, err := uow.Roles().FindIDByName(ctx, adminRole)
		if err != nil {
			return err
		}
		exists, err := uow.Roles().HasMembers(ctx, adminID)
		if exists || err != nil {
			return err
		}
		switch _, err := uow.Users().FindByEmail(ctx, email); {
		case err == nil:
			return apperror.New(apperror.CodeConflict, "email already in use").WithField("email")
		case !apperror.IsNotFound(err):
			return err
		}
		user := &domain.User{Name: name, Email: email, Status: domain.UserStatusActive}
		if err := uow.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := uow.Credentials().Create(ctx, user.ID); err != nil {
			return err
		}
		if err := uow.Credentials().SetPassword(ctx, user.ID, hash); err != nil {
			return err
		}
		if err := policy.AssignDefaultRoles(ctx, uow, user.ID); err != nil {
			return err
		}
		created = true
		if err := uow.Roles().Assign(ctx, user.ID, adminID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "admin.bootstrap", user.ID, nil)
	})
	return created && err == nil, err
}

// Promote gives an active user the admin role.
func (s *Service) Promote(ctx context.Context, email string) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		user, err := uow.Users().FindByEmail(ctx, policy.NormalizeEmail(email))
		if err != nil {
			return err
		}
		if user.Status != domain.UserStatusActive {
			return apperror.New(apperror.CodeConflict, "user is "+user.Status+", not active")
		}
		roleID, err := uow.Roles().FindIDByName(ctx, adminRole)
		if err != nil {
			return err
		}
		if err := uow.Roles().Assign(ctx, user.ID, roleID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "admin.promote", user.ID, nil)
	})
}

// Disable blocks a user and revokes their sessions, so access tokens stop at
// the next request: Bearer checks the session every time.
func (s *Service) Disable(ctx context.Context, email string) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		user, err := uow.Users().FindByEmail(ctx, policy.NormalizeEmail(email))
		if err != nil {
			return err
		}
		if err := policy.Disable(ctx, uow, user.ID, s.Clock.Now()); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.disable", user.ID, nil)
	})
}

// Enable lifts a disable. It is the only way back for a user with a protected
// role, which the API does not touch.
func (s *Service) Enable(ctx context.Context, email string) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		user, err := uow.Users().FindByEmail(ctx, policy.NormalizeEmail(email))
		if err != nil {
			return err
		}
		if err := uow.Users().Enable(ctx, user.ID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "user.enable", user.ID, nil)
	})
}

var _ input.AdminService = (*Service)(nil)
