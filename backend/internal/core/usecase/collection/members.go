package collection

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

func (s *Service) Members(ctx context.Context, id string) ([]domain.CollectionMember, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleOwner); err != nil {
		return nil, err
	}
	return s.Repo.Members(ctx, id)
}

// AddMember adds an account by email, inviting the email first when it has none.
// shortcut: the invite and the membership are two transactions; when the second
// fails the pending account stays, and adding the same email again picks it up.
func (s *Service) AddMember(ctx context.Context, cmd input.AddMemberCommand) (*input.AddMemberResult, error) {
	if !cmd.Role.Valid() {
		return nil, apperror.New(apperror.CodeInvalidInput, "role must be owner, editor or viewer").WithField("role")
	}
	email := policy.NormalizeEmail(cmd.Email)
	if _, err := policy.CollectionRole(ctx, s.Repo, cmd.CollectionID, domain.RoleOwner); err != nil {
		return nil, err
	}
	res := &input.AddMemberResult{}
	var userID string
	if u, err := s.lookup(ctx, email); err != nil {
		return nil, err
	} else if u != nil {
		userID = u.ID
	} else {
		if cmd.Name == "" {
			return nil, apperror.New(apperror.CodeInvalidInput, "no account has this email; give a name to invite it").WithField("name")
		}
		inv, err := s.Inviter.Invite(ctx, input.RegisterCommand{Name: cmd.Name, Email: email})
		if err != nil {
			return nil, err
		}
		userID, res.Invited, res.ActivationToken = inv.UserID, true, inv.ActivationToken
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Collections().AddMember(ctx, cmd.CollectionID, userID, cmd.Role); err != nil {
			if apperror.IsCode(err, apperror.CodeAlreadyExists) {
				return apperror.New(apperror.CodeAlreadyExists, "this person is already a member").WithField("email")
			}
			return err
		}
		if err := policy.Audit(ctx, uow, "collection.member_add", cmd.CollectionID, map[string]any{"user_id": userID, "role": cmd.Role}); err != nil {
			return err
		}
		if userID == reqctx.Actor(ctx) {
			return nil
		}
		c, err := uow.Collections().FindByID(ctx, cmd.CollectionID)
		if err != nil {
			return err
		}
		// Shown at the next sign-in for an invited account.
		return uow.Notifications().Create(ctx, &domain.Notification{
			UserID: userID, Level: domain.NotificationInfo, Kind: domain.NotifyCollectionAdded,
			Params: map[string]string{"collection": c.Name, "collection_id": c.ID, "role": string(cmd.Role)},
			Title:  "Added to a collection", Message: "You were added to " + c.Name + " as " + string(cmd.Role) + ".",
		})
	})
	if err != nil {
		return nil, err
	}
	members, err := s.Repo.Members(ctx, cmd.CollectionID)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if m.UserID == userID {
			res.Member = m
		}
	}
	return res, nil
}

// lookup finds the account behind email, nil when there is none.
func (s *Service) lookup(ctx context.Context, email string) (*domain.User, error) {
	var found *domain.User
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		u, err := uow.Users().FindByEmail(ctx, email)
		if apperror.IsNotFound(err) {
			return nil
		}
		found = u
		return err
	})
	return found, err
}

func (s *Service) SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error {
	if !role.Valid() {
		return apperror.New(apperror.CodeInvalidInput, "role must be owner, editor or viewer").WithField("role")
	}
	return s.changeMember(ctx, id, userID, "collection.member_role", map[string]any{"user_id": userID, "role": role},
		func(repo output.CollectionRepository) error { return repo.SetMemberRole(ctx, id, userID, role) })
}

func (s *Service) RemoveMember(ctx context.Context, id, userID string) error {
	return s.changeMember(ctx, id, userID, "collection.member_remove", map[string]any{"user_id": userID},
		func(repo output.CollectionRepository) error { return repo.RemoveMember(ctx, id, userID) })
}

// changeMember runs one owner-only member change; the lock serializes changes to
// the collection's members, so two owners demoting each other cannot leave none.
func (s *Service) changeMember(ctx context.Context, id, userID, action string, details map[string]any, change func(output.CollectionRepository) error) error {
	if err := validate.ID(userID); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), id, domain.RoleOwner); err != nil {
			return err
		}
		if err := uow.Lock(ctx, "collection.members:"+id); err != nil {
			return err
		}
		before, err := uow.Collections().Owners(ctx, id)
		if err != nil {
			return err
		}
		if err := change(uow.Collections()); err != nil {
			return err
		}
		after, err := uow.Collections().Owners(ctx, id)
		if err != nil {
			return err
		}
		if before > 0 && after == 0 {
			return apperror.New(apperror.CodeConflict, "a collection keeps at least one owner").WithField("role")
		}
		return policy.Audit(ctx, uow, action, id, details)
	})
}

// Leave takes the caller out of the collection, under the same last-owner rule.
func (s *Service) Leave(ctx context.Context, id string) error {
	actor := reqctx.Actor(ctx)
	if actor == "" {
		return apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), id, domain.RoleViewer); err != nil {
			return err
		}
		if err := uow.Lock(ctx, "collection.members:"+id); err != nil {
			return err
		}
		before, err := uow.Collections().Owners(ctx, id)
		if err != nil {
			return err
		}
		if err := uow.Collections().RemoveMember(ctx, id, actor); err != nil {
			return err
		}
		if after, err := uow.Collections().Owners(ctx, id); err != nil {
			return err
		} else if before > 0 && after == 0 {
			return apperror.New(apperror.CodeConflict, "a collection keeps at least one owner: make someone else owner first")
		}
		return policy.Audit(ctx, uow, "collection.member_leave", id, map[string]any{"user_id": actor})
	})
}
