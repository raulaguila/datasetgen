package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestRenameValidatesAndUpdates(t *testing.T) {
	e := newEnv(t)
	id := e.userID(t, e.activeUser(t))
	ctx := context.Background()
	name := func(s string) *string { return &s }

	_, err := e.account().UpdateProfile(ctx, id, name("a"), nil)
	wantCode(t, err, apperror.CodeInvalidInput)

	p, err := e.account().UpdateProfile(ctx, id, name("  Ada Lovelace  "), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.User.Name != "Ada Lovelace" {
		t.Fatalf("name %q", p.User.Name)
	}
}

func TestUsernameComesFromTheEmailAndChangesOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first := e.userID(t, e.activeUser(t))
	p, err := e.account().Profile(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if p.User.Username == "" {
		t.Fatal("a new account has no username")
	}
	second := e.userID(t, e.activeUser(t))
	name := func(s string) *string { return &s }

	for _, bad := range []string{"admin", "Ana Silva", "-ana", "ana--silva"} {
		_, err := e.account().UpdateProfile(ctx, second, nil, name(bad))
		wantCode(t, err, apperror.CodeInvalidInput)
	}
	_, err = e.account().UpdateProfile(ctx, second, name("Somebody Else"), name(p.User.Username))
	wantCode(t, err, apperror.CodeAlreadyExists)
	if q, _ := e.account().Profile(ctx, second); q.User.Name == "Somebody Else" {
		t.Fatal("a taken username left the new name saved")
	}
	got, err := e.account().UpdateProfile(ctx, second, nil, name("  Second-User  "))
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Username != "second-user" {
		t.Fatalf("username %q", got.User.Username)
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
