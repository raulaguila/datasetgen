package policy

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// GiveStarterCollection creates the starter collection, owned by userID, when
// the account belongs to none: a self sign-up or an invite from the users
// screen. Someone invited into a collection already has a place to start.
func GiveStarterCollection(ctx context.Context, uow output.UnitOfWork, userID string) error {
	member, err := uow.Collections().HasMembership(ctx, userID)
	if err != nil || member {
		return err
	}
	lang := domain.DefaultLanguage
	if prefs, err := uow.Preferences().Find(ctx, userID); err == nil {
		lang = prefs.Language
	} else if !apperror.IsNotFound(err) {
		return err
	}
	c := domain.StarterCollection(lang)
	if err := uow.Collections().Create(ctx, &c); err != nil {
		return err
	}
	if err := uow.Collections().AddMember(ctx, c.ID, userID, domain.RoleOwner); err != nil {
		return err
	}
	// The new account is the actor: activation runs before any session exists.
	return Audit(reqctx.WithActor(ctx, userID), uow, "collection.create", c.ID, map[string]any{"name": c.Name, "starter": true})
}
