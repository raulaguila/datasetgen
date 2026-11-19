package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// A collection lives at /<owner>/<slug>; renames, transfers and username changes
// move it, and the old path still opens it for its members.
func TestCollectionPathsFollowTheOwner(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	as := func(addr string) context.Context { return reqctx.WithActor(context.Background(), e.userID(t, addr)) }
	anaAddr, biaAddr, outsiderAddr := e.activeUser(t), e.activeUser(t), e.activeUser(t)
	ana, bia, outsider := as(anaAddr), as(biaAddr), as(outsiderAddr)
	anaID, biaID := e.userID(t, anaAddr), e.userID(t, biaAddr)
	username := func(id string) string {
		u, err := e.store.Users().FindByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return u.Username
	}
	open := func(ctx context.Context, owner, slug string) (*domain.Collection, error) {
		id, err := collections.Resolve(ctx, owner, slug)
		if err != nil {
			return nil, err
		}
		return collections.Get(ctx, id)
	}
	// opens checks that owner/slug leads ctx to the collection id.
	opens := func(ctx context.Context, owner, slug, id string) {
		t.Helper()
		if got, err := open(ctx, owner, slug); err != nil || got.ID != id {
			t.Fatalf("%s/%s: %+v, %v", owner, slug, got, err)
		}
	}
	create := func(ctx context.Context, name string) *domain.Collection {
		c, err := collections.Create(ctx, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(`{"type":"object"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	livros := create(ana, "Livros Lidos")
	if livros.Owner != username(anaID) || livros.Slug != "livros-lidos" {
		t.Fatalf("path %s/%s", livros.Owner, livros.Slug)
	}
	// The same name twice under one owner takes the next free slug; another owner keeps the plain one.
	if again := create(ana, "Livros  lidos!"); again.Slug != "livros-lidos-2" {
		t.Fatalf("second slug %q", again.Slug)
	}
	if hers := create(bia, "Livros Lidos"); hers.Slug != "livros-lidos" {
		t.Fatalf("other owner's slug %q", hers.Slug)
	}
	opens(ana, livros.Owner, "LIVROS-LIDOS", livros.ID)
	// A non-member cannot tell a hidden collection from a missing one.
	wantCode(t, ignore(open(outsider, livros.Owner, livros.Slug)), apperror.CodeNotFound)
	wantCode(t, ignore(open(ana, livros.Owner, "nao-existe")), apperror.CodeNotFound)

	// A rename moves the slug; the old path still opens it.
	renamed, err := collections.Update(ana, input.UpdateCollectionCommand{ID: livros.ID, Name: pointer.Of("Leituras")})
	if err != nil || renamed.Slug != "leituras" {
		t.Fatalf("rename: %+v, %v", renamed, err)
	}
	opens(ana, livros.Owner, "livros-lidos", livros.ID)

	// The owner in the URL keeps the owner role until a transfer, which needs an owner member.
	if _, err := collections.AddMember(ana, input.AddMemberCommand{CollectionID: livros.ID, Email: biaAddr, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(collections.Transfer(ana, livros.ID, biaID)), apperror.CodeInvalidInput)
	if err := collections.SetMemberRole(ana, livros.ID, biaID, domain.RoleOwner); err != nil {
		t.Fatal(err)
	}
	wantCode(t, collections.SetMemberRole(bia, livros.ID, anaID, domain.RoleEditor), apperror.CodeConflict)
	wantCode(t, collections.RemoveMember(bia, livros.ID, anaID), apperror.CodeConflict)
	wantCode(t, collections.Leave(ana, livros.ID), apperror.CodeConflict)
	moved, err := collections.Transfer(ana, livros.ID, biaID)
	if err != nil || moved.Owner != username(biaID) || moved.Slug != "leituras" {
		t.Fatalf("transfer: %+v, %v", moved, err)
	}
	opens(bia, username(anaID), "leituras", livros.ID)
	if err := collections.Leave(ana, livros.ID); err != nil {
		t.Fatalf("former URL owner leaves: %v", err)
	}

	// A new username moves every collection the account owns; the old one still leads there.
	before := username(biaID)
	if _, err := e.account().UpdateProfile(context.Background(), biaID, nil, pointer.Of("bia-nova")); err != nil {
		t.Fatal(err)
	}
	opens(bia, "bia-nova", "leituras", livros.ID)
	opens(bia, before, "leituras", livros.ID)

	// A deleted collection gives its path up; a restore takes a free one.
	if err := collections.DeleteMany(bia, []string{livros.ID}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(open(bia, "bia-nova", "leituras")), apperror.CodeNotFound)
	if fresh := create(bia, "Leituras"); fresh.Slug != "leituras" {
		t.Fatalf("slug of a deleted collection not freed: %q", fresh.Slug)
	}
	if err := e.store.Tools().RestoreCollection(context.Background(), livros.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := collections.Get(bia, livros.ID); err != nil || got.Slug != "leituras-2" {
		t.Fatalf("restored: %+v, %v", got, err)
	}
}
