package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/totp"
)

// Two-step login: after setup and a confirming code, the password alone gets no
// session; each code works once, and so does each recovery code.
func TestTwoStepLogin(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	me := reqctx.WithActor(context.Background(), e.userID(t, addr))
	code := func(secret string) string {
		t.Helper()
		c, err := totp.Code(secret, totp.Step(e.clock.now))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	withCode := func(c string) error {
		_, err := e.svc.Login(context.Background(), input.LoginCommand{Email: addr, Password: strongPassword, Code: c})
		return err
	}

	secret, uri, err := e.svc.SetupMFA(me)
	if err != nil || secret == "" || uri == "" {
		t.Fatalf("setup: %q %q %v", secret, uri, err)
	}
	// Setup alone changes nothing at login.
	if err := withCode(""); err != nil {
		t.Fatalf("login before enable: %v", err)
	}
	_, err = e.svc.EnableMFA(me, "000000")
	wantCode(t, err, apperror.CodeInvalidInput)
	enabling := code(secret)
	recovery, err := e.svc.EnableMFA(me, enabling)
	if err != nil || len(recovery) != 10 {
		t.Fatalf("enable: %v %v", recovery, err)
	}
	// The admin's user screen sees it.
	if a, err := e.store.Users().Access(context.Background(), e.userID(t, addr), e.clock.now, e.clock.now); err != nil || !a.MFAEnabled {
		t.Fatalf("access after enable: %+v %v", a, err)
	}
	if st, _ := e.svc.MFAStatus(me); !st.Enabled || st.RecoveryLeft != 10 {
		t.Fatalf("status: %+v", st)
	}
	_, _, err = e.svc.SetupMFA(me)
	wantCode(t, err, apperror.CodeConflict)

	wantCode(t, withCode(""), apperror.CodeMFARequired)
	// The code that turned it on is spent.
	wantCode(t, withCode(enabling), apperror.CodeInvalidCredentials)
	e.clock.now = e.clock.now.Add(30 * time.Second)
	next := code(secret)
	if err := withCode(next); err != nil {
		t.Fatalf("login with code: %v", err)
	}
	wantCode(t, withCode(next), apperror.CodeInvalidCredentials)

	if err := withCode(recovery[0]); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	wantCode(t, withCode(recovery[0]), apperror.CodeInvalidCredentials)
	if st, _ := e.svc.MFAStatus(me); st.RecoveryLeft != 9 {
		t.Fatalf("recovery left: %+v", st)
	}

	wantCode(t, e.svc.DisableMFA(me, "wrong-password"), apperror.CodeInvalidCredentials)
	if err := e.svc.DisableMFA(me, strongPassword); err != nil {
		t.Fatal(err)
	}
	if err := withCode(""); err != nil {
		t.Fatalf("login after disable: %v", err)
	}

	// An operator can turn it off for a user who lost the phone.
	secret, _, _ = e.svc.SetupMFA(me)
	e.clock.now = e.clock.now.Add(30 * time.Second)
	if _, err := e.svc.EnableMFA(me, code(secret)); err != nil {
		t.Fatal(err)
	}
	wantCode(t, withCode(""), apperror.CodeMFARequired)
	if err := admin.New(e.store, e.clock).ResetMFA(context.Background(), addr); err != nil {
		t.Fatal(err)
	}
	if err := withCode(""); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}
