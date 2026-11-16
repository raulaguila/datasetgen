package integration_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// fakeSSO vouches for email when the code, verifier and nonce are the ones it handed out.
type fakeSSO struct {
	email           string
	verifier, nonce string
}

func (f *fakeSSO) Name() string { return "Acme" }

func (f *fakeSSO) AuthURL(_ context.Context, state, nonce, verifier string) (string, error) {
	f.nonce, f.verifier = nonce, verifier
	return "https://idp.test/auth?state=" + url.QueryEscape(state), nil
}

func (f *fakeSSO) Email(_ context.Context, code, verifier, nonce string) (string, error) {
	if code != "good" || verifier != f.verifier || nonce != f.nonce {
		return "", errors.New("refused")
	}
	return f.email, nil
}

// Single sign-on logs in an existing active account by its verified email,
// only from the browser that started it, and never creates accounts.
func TestSSOLogsInExistingAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, _, err := e.svc.SSOStart(ctx)
	wantCode(t, err, apperror.CodeNotFound)

	idp := &fakeSSO{email: e.activeUser(t)}
	e.svc.SSO = idp
	start := func() (state, flow string) {
		t.Helper()
		u, flow, err := e.svc.SSOStart(ctx)
		if err != nil {
			t.Fatal(err)
		}
		parsed, _ := url.Parse(u)
		return parsed.Query().Get("state"), flow
	}
	login := func(code, state, flow string) error {
		_, err := e.svc.SSOLogin(ctx, input.SSOCommand{Code: code, State: state, Flow: flow})
		return err
	}

	state, flow := start()
	wantCode(t, login("good", "forged", flow), apperror.CodeInvalidInput)
	wantCode(t, login("good", state, "garbage"), apperror.CodeInvalidInput)
	wantCode(t, login("bad", state, flow), apperror.CodeInvalidCredentials)
	if err := login("good", state, flow); err != nil {
		t.Fatalf("sso login: %v", err)
	}

	state, flow = start()
	e.clock.now = e.clock.now.Add(11 * time.Minute)
	wantCode(t, login("good", state, flow), apperror.CodeInvalidInput)

	idp.email = email() // nobody has it
	state, flow = start()
	wantCode(t, login("good", state, flow), apperror.CodeForbidden)

	pending := email()
	e.register(t, pending)
	idp.email = pending
	state, flow = start()
	wantCode(t, login("good", state, flow), apperror.CodeInvalidCredentials)
}
