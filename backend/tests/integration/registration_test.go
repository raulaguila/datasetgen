package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestRegisterRecoversAnExpiredActivation(t *testing.T) {
	e := newEnv(t)
	addr := email()
	first := e.register(t, addr)

	e.clock.now = e.clock.now.Add(25 * time.Hour)
	err := e.svc.Activate(context.Background(), input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword})
	wantCode(t, err, apperror.CodeTokenExpired)
	if got := e.userStatus(t, addr); got != domain.UserStatusPending {
		t.Fatalf("status %s", got)
	}

	second := e.register(t, addr)
	if second.ActivationToken == "" || second.ActivationToken == first.ActivationToken {
		t.Fatal("no fresh token issued")
	}
	if err := e.svc.Activate(context.Background(), input.ActivateCommand{Token: second.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.login(addr, strongPassword); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterOfActiveEmailLooksLikeNewAndSendsNothing(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	if err := testDB.Exec("DELETE FROM sys.outbox").Error; err != nil {
		t.Fatal(err)
	}
	res := e.register(t, addr)
	if res.Status != domain.UserStatusPending || res.UserID == "" {
		t.Fatalf("distinguishable answer: %+v", res)
	}
	var queued int64
	if err := testDB.Table("sys.outbox").Count(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("%d emails queued for an existing account", queued)
	}
}

func TestEmailCooldownLimitsTokensPerAccount(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.EmailTokenCooldown = time.Minute
	ctx := context.Background()
	addr := email()
	first := e.register(t, addr)

	if again := e.register(t, addr); again.ActivationToken != "" {
		t.Fatal("second token inside the cooldown")
	}
	if err := testDB.Exec(`UPDATE usr.usr_token SET created_at = now() - INTERVAL '2 minutes'`).Error; err != nil {
		t.Fatal(err)
	}
	if later := e.register(t, addr); later.ActivationToken == "" {
		t.Fatal("no token after the cooldown")
	}
	err := e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword})
	if err == nil {
		t.Fatal("replaced token still works")
	}

	active := e.activeUser(t)
	res, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: active})
	if err != nil || res.ResetToken == "" {
		t.Fatalf("first reset: %+v %v", res, err)
	}
	if res, err = e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: active}); err != nil || res.ResetToken != "" {
		t.Fatalf("second reset inside the cooldown: %+v %v", res, err)
	}
}

func TestActivationDoesNotHashForBadTokens(t *testing.T) {
	e := newEnv(t)
	err := e.svc.Activate(context.Background(), input.ActivateCommand{Token: "unknown", Password: strongPassword})
	wantCode(t, err, apperror.CodeNotFound)
}

func TestHourlyCapKeepsTheLinkAlive(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.EmailTokenHourlyMax = 2
	e.svc.Cfg.EmailTokenCooldown = 0
	ctx := context.Background()
	addr := email()

	first := e.register(t, addr)
	e.clock.now = e.clock.now.Add(time.Second)
	second := e.register(t, addr)
	if second.ActivationToken == "" {
		t.Fatal("second token within the cap refused")
	}
	e.clock.now = e.clock.now.Add(time.Second)
	if third := e.register(t, addr); third.ActivationToken != "" {
		t.Fatal("third token over the cap")
	}
	err := e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword})
	wantCode(t, err, apperror.CodeTokenExpired)
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: second.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatalf("link died: %v", err)
	}
}

func TestHostileEmailsNeverReachTheDatabaseRaw(t *testing.T) {
	e := newEnv(t)
	ip := "198.51.100.77"
	for _, addr := range []string{strings.Repeat("a", 5000) + "@acme.io", "nul\x00byte@acme.io"} {
		_, err := e.loginFrom(addr, "wrong-password", ip)
		wantCode(t, err, apperror.CodeInvalidCredentials)
		_, err = e.svc.Register(context.Background(), input.RegisterCommand{Name: "Ana", Email: addr})
		wantCode(t, err, apperror.CodeInvalidInput)
		if _, err := e.svc.RequestPasswordReset(context.Background(), input.RequestPasswordResetCommand{Email: addr}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := countByIP(context.Background(), ip, time.Now().Add(-time.Minute))
	if err != nil || n < 2 {
		t.Fatalf("attempts not recorded: %d %v", n, err)
	}
}

func TestClosedRegistrationLeavesOnlyInvitations(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.AllowRegistration = false
	ctx := context.Background()
	_, err := e.svc.Register(ctx, input.RegisterCommand{Name: "Ana", Email: email()})
	wantCode(t, err, apperror.CodeForbidden)
	if _, err := e.svc.Invite(ctx, input.RegisterCommand{Name: "Ana", Email: email()}); err != nil {
		t.Fatal(err)
	}
}

func TestResetRequestTakesTheSameTimeForAnyEmail(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.PasswordResetMinDuration = 150 * time.Millisecond
	for _, addr := range []string{email(), e.activeUser(t)} { // unknown, then known
		start := time.Now()
		if _, err := e.svc.RequestPasswordReset(context.Background(), input.RequestPasswordResetCommand{Email: addr}); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took < 150*time.Millisecond {
			t.Fatalf("%s answered in %s", addr, took)
		}
	}
}
