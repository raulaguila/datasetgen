package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/auth"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type stubAuth struct {
	input.AuthService
	err   error
	login input.LoginCommand
}

func (s *stubAuth) pair() (*domain.TokenPair, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.TokenPair{UserID: "u1", AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900, TokenType: "Bearer"}, nil
}

func (s *stubAuth) Login(_ context.Context, cmd input.LoginCommand) (*domain.TokenPair, error) {
	s.login = cmd
	return s.pair()
}

func (s *stubAuth) Refresh(context.Context, input.RefreshCommand) (*domain.TokenPair, error) {
	return s.pair()
}

type stubAccount struct {
	input.AccountService
	err error
}

func (s stubAccount) Profile(_ context.Context, userID string) (*domain.Profile, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.Profile{User: domain.User{ID: userID}}, nil
}

func post(t *testing.T, svc *stubAuth, account stubAccount, path, body string) (int, map[string]any) {
	t.Helper()
	pass := func(c fiber.Ctx) error { return c.Next() }
	app := fiber.New()
	auth.New(svc, account).Routes(app, pass, pass, pass)

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "probe")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func TestLoginAnswersTokensAndProfile(t *testing.T) {
	svc := &stubAuth{}
	status, out := post(t, svc, stubAccount{}, "/login", `{"email":"a@acme.io","password":"secret"}`)
	if status != fiber.StatusOK || out["access_token"] != "access" || out["refresh_token"] != "refresh" || out["user"] == nil {
		t.Fatalf("%d %v", status, out)
	}
	cmd := svc.login
	if cmd.Email != "a@acme.io" || cmd.Password != "secret" || cmd.IPAddress == nil || cmd.UserAgent == nil || *cmd.UserAgent != "probe" {
		t.Fatalf("command %+v", cmd)
	}
}

// The old refresh token is already spent: losing the new one would end the session.
func TestRefreshKeepsTheTokensWhenTheProfileFails(t *testing.T) {
	status, out := post(t, &stubAuth{}, stubAccount{err: errors.New("db down")}, "/refresh", `{"refresh_token":"old"}`)
	if status != fiber.StatusOK || out["refresh_token"] != "refresh" {
		t.Fatalf("%d %v", status, out)
	}
	if _, found := out["user"]; found {
		t.Fatalf("user without a profile: %v", out)
	}
}

func TestErrorsBecomeTheirStatus(t *testing.T) {
	for _, tc := range []struct {
		code   apperror.Code
		status int
	}{
		{apperror.CodeInvalidCredentials, fiber.StatusUnauthorized},
		{apperror.CodeTooManyRequests, fiber.StatusTooManyRequests},
		{apperror.CodeTimedOut, fiber.StatusGatewayTimeout},
	} {
		status, out := post(t, &stubAuth{err: apperror.New(tc.code, "no")}, stubAccount{}, "/login", `{"email":"a@acme.io","password":"secret"}`)
		if status != tc.status || out["code"] != string(tc.code) || out["access_token"] != nil {
			t.Errorf("%s: %d %v", tc.code, status, out)
		}
	}
}

func TestBrokenBodyStopsBeforeTheService(t *testing.T) {
	for _, path := range []string{"/login", "/refresh"} {
		status, _ := post(t, nil, stubAccount{}, path, `{`) // a nil service proves nothing runs
		if status != fiber.StatusBadRequest {
			t.Errorf("%s: %d", path, status)
		}
	}
}
