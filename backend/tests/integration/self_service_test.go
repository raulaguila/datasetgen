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
