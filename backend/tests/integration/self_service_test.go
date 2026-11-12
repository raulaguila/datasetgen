package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/internal/core/usecase/role"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestRenameValidatesAndUpdates(t *testing.T) {
	e := newEnv(t)
	id := e.userID(t, e.activeUser(t))
	ctx := context.Background()

	_, err := e.account().Rename(ctx, id, "a")
	wantCode(t, err, apperror.CodeInvalidInput)

	p, err := e.account().Rename(ctx, id, "  Ada Lovelace  ")
	if err != nil {
		t.Fatal(err)
	}
	if p.User.Name != "Ada Lovelace" {
		t.Fatalf("name %q", p.User.Name)
	}
}

func TestChangePasswordKeepsOnlyTheCurrentSession(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	current := e.loginAt(t, addr, "203.0.113.1")
	other := e.loginAt(t, addr, "203.0.113.2")
	principal, err := e.svc.Authenticate(ctx, current.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	cmd := input.ChangePasswordCommand{UserID: principal.UserID, SessionID: principal.SessionID, CurrentPassword: "wrong password", NewPassword: "a brand new password"}

	wantCode(t, e.svc.ChangePassword(ctx, cmd), apperror.CodeInvalidInput)

	cmd.CurrentPassword = strongPassword
	if err := e.svc.ChangePassword(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: current.RefreshToken}); err != nil {
		t.Fatalf("current session ended: %v", err)
	}
	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: other.RefreshToken})
	wantCode(t, err, apperror.CodeUnauthorized)
	_, err = e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)
	if _, err := e.login(addr, cmd.NewPassword); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(e.titles(t, principal.UserID), "Password changed") {
		t.Fatal("no notification")
	}
}

func TestProfileListsEnabledRoles(t *testing.T) {
	e := newEnv(t)
	p, err := e.account().Profile(context.Background(), e.userID(t, e.activeUser(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Roles, []string{"user"}) {
		t.Fatalf("roles %v", p.Roles)
	}
}

// The count must match the members list, which hides protected users.
func TestRoleMemberCountSkipsProtectedUsers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created, err := svc.Create(ctx, "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	plain, boss := e.activeUser(t), e.activeUser(t)
	for _, addr := range []string{plain, boss} {
		if err := svc.AddMember(ctx, created.ID, e.userID(t, addr)); err != nil {
			t.Fatal(err)
		}
	}
	if err := admin.New(e.store, e.clock).Promote(ctx, boss); err != nil {
		t.Fatal(err)
	}
	if got := findRole(t, svc, created.Name).MemberCount; got != 1 {
		t.Fatalf("member count %d", got)
	}
	// Paths that never show the number must not pay for it.
	roles, err := e.store.Roles().ForUser(ctx, e.userID(t, plain))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.MemberCount != 0 {
			t.Fatalf("ForUser counted members of %s", r.Name)
		}
	}
}

func TestSetUserRolesReplacesTheSet(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	a, err := svc.Create(ctx, "a"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, "b"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	id := e.userID(t, e.activeUser(t))
	names := func() []string {
		roles, err := e.store.Roles().ForUser(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range roles {
			out = append(out, r.Name)
		}
		slices.Sort(out)
		return out
	}

	if err := svc.SetUserRoles(ctx, id, []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := names(); !slices.Equal(got, []string{a.Name, b.Name}) {
		t.Fatalf("roles %v", got)
	}
	if err := svc.SetUserRoles(ctx, id, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := names(); !slices.Equal(got, []string{b.Name}) {
		t.Fatalf("roles %v", got)
	}
	// A protected role is refused and nothing changes.
	wantCode(t, svc.SetUserRoles(ctx, id, []string{findRole(t, svc, "admin").ID}), apperror.CodeForbidden)
	if got := names(); !slices.Equal(got, []string{b.Name}) {
		t.Fatalf("roles after refusal %v", got)
	}
}
