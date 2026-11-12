package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
)

func TestLoginCannotRestoreOldPassword(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	u, err := e.store.Users().FindByEmail(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.store.Credentials().FindByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The interleaving: a login verified the old hash, then a reset committed.
	newHash, err := password.Hash(ctx, "a brand new password")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.Credentials().SetPassword(ctx, u.ID, newHash); err != nil {
		t.Fatal(err)
	}

	applied, err := e.store.Credentials().RecordSuccess(ctx, u.ID, *before.PasswordHash, e.clock.now)
	if err != nil || applied {
		t.Fatalf("stale login accepted: %v %v", applied, err)
	}
	if _, err := e.store.Credentials().RecordFailure(ctx, u.ID, 3, e.clock.now.Add(time.Hour), e.clock.now); err != nil {
		t.Fatal(err)
	}
	after, err := e.store.Credentials().FindByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *after.PasswordHash != newHash {
		t.Fatal("old password restored")
	}
}

func TestLockedAccountCanResetPassword(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	for range 3 {
		_, _ = e.login(addr, "wrong-password")
	}

	res, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil || res.ResetToken == "" {
		t.Fatalf("no reset for locked account: %+v %v", res, err)
	}
	const newPassword = "a brand new password"
	if err := e.svc.ConfirmPasswordReset(ctx, input.ConfirmPasswordResetCommand{Token: res.ResetToken, Password: newPassword}); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
	if _, err := e.login(addr, newPassword); err != nil {
		t.Fatal(err)
	}
}

func TestResetRevokesSessionsAndTokenIsSingleUse(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	cmd := input.ConfirmPasswordResetCommand{Token: res.ResetToken, Password: "a brand new password"}
	if err := e.svc.ConfirmPasswordReset(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.ConfirmPasswordReset(ctx, cmd), apperror.CodeTokenExpired)

	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: pair.RefreshToken})
	wantCode(t, err, apperror.CodeUnauthorized)
}

// Strangers on many addresses can fill the account limit. A reset proves the
// owner, who gets back in from anywhere.
func TestResetLetsTheOwnerBackIn(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	e.svc.Cfg.MaxFailedLoginAttempts = 50
	for i := 1; i <= 5; i++ {
		_, err := e.loginFrom(addr, "wrong password", fmt.Sprintf("198.51.100.%d", i))
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	_, err := e.loginFrom(addr, strongPassword, "203.0.113.9")
	wantCode(t, err, apperror.CodeTooManyRequests)

	res, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	changed := strongPassword + " 2"
	if err := e.svc.ConfirmPasswordReset(ctx, input.ConfirmPasswordResetCommand{Token: res.ResetToken, Password: changed}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.loginFrom(addr, changed, "203.0.113.9"); err != nil {
		t.Fatal(err)
	}
}
