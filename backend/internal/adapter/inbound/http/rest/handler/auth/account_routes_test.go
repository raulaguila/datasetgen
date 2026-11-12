package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/auth"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

// recorder keeps what each route handed to its service.
type recorder struct {
	input.AuthService
	input.AccountService
	manual bool
	got    any
}

func (r *recorder) Register(_ context.Context, cmd input.RegisterCommand) (*input.RegisterResult, error) {
	r.got = cmd
	return &input.RegisterResult{UserID: "u9", Status: domain.UserStatusPending, ActivationToken: "act", ManualDelivery: r.manual}, nil
}

func (r *recorder) Activate(_ context.Context, cmd input.ActivateCommand) error {
	r.got = cmd
	return nil
}

func (r *recorder) Logout(_ context.Context, sessionID string) error {
	r.got = sessionID
	return nil
}

func (r *recorder) ChangePassword(_ context.Context, cmd input.ChangePasswordCommand) error {
	r.got = cmd
	return nil
}

func (r *recorder) RequestPasswordReset(_ context.Context, cmd input.RequestPasswordResetCommand) (*input.RequestPasswordResetResult, error) {
	r.got = cmd
	return &input.RequestPasswordResetResult{ResetToken: "rst", ManualDelivery: r.manual}, nil
}

func (r *recorder) ConfirmPasswordReset(_ context.Context, cmd input.ConfirmPasswordResetCommand) error {
	r.got = cmd
	return nil
}

func (r *recorder) Profile(_ context.Context, userID string) (*domain.Profile, error) {
	r.got = userID
	return &domain.Profile{User: domain.User{ID: userID}}, nil
}

func (r *recorder) Rename(_ context.Context, userID, name string) (*domain.Profile, error) {
	r.got = [2]string{userID, name}
	return &domain.Profile{User: domain.User{ID: userID, Name: name}}, nil
}

func call(t *testing.T, rec *recorder, method, path, body string) (int, map[string]any) {
	t.Helper()
	signedIn := func(c fiber.Ctx) error {
		c.Locals(middleware.LocalUserID, "u1")
		c.Locals(middleware.LocalSessionID, "s1")
		return c.Next()
	}
	pass := func(c fiber.Ctx) error { return c.Next() }
	app := fiber.New()
	h := auth.New(rec, rec)
	h.Routes(app.Group("/auth"), signedIn, pass, pass)
	h.MeRoutes(app.Group("/me", signedIn), pass)

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s %s answered %q", method, path, raw)
		}
	}
	return resp.StatusCode, out
}

// Each route must hand its service the body fields and, where signed in, the
// user and session of the token: a mix-up here ends the wrong session.
func TestRoutesPassTheBodyAndThePrincipal(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
		want               any
	}{
		{"POST", "/auth/register", `{"name":"Ana","email":"a@acme.io"}`, 201, input.RegisterCommand{Name: "Ana", Email: "a@acme.io"}},
		{"POST", "/auth/activate", `{"token":"t","password":"p"}`, 204, input.ActivateCommand{Token: "t", Password: "p"}},
		{"POST", "/auth/logout", ``, 204, "s1"},
		{"POST", "/auth/password-reset/request", `{"email":"a@acme.io"}`, 200, input.RequestPasswordResetCommand{Email: "a@acme.io"}},
		{"POST", "/auth/password-reset/confirm", `{"token":"t","password":"p"}`, 204, input.ConfirmPasswordResetCommand{Token: "t", Password: "p"}},
		{"GET", "/me", ``, 200, "u1"},
		{"PATCH", "/me", `{"name":"Bia"}`, 200, [2]string{"u1", "Bia"}},
		{"POST", "/me/password", `{"current_password":"old","new_password":"new"}`, 204, input.ChangePasswordCommand{UserID: "u1", SessionID: "s1", CurrentPassword: "old", NewPassword: "new"}},
	} {
		rec := &recorder{}
		status, out := call(t, rec, tc.method, tc.path, tc.body)
		if status != tc.status || !reflect.DeepEqual(rec.got, tc.want) {
			t.Errorf("%s %s: %d %v, service got %+v", tc.method, tc.path, status, out, rec.got)
		}
	}
	if _, out := call(t, &recorder{}, "GET", "/me", ``); out["session_id"] != "s1" {
		t.Errorf("/me session: %v", out)
	}
}

func TestTokensLeaveTheAPIOnlyWithManualDelivery(t *testing.T) {
	for _, manual := range []bool{false, true} {
		_, reg := call(t, &recorder{manual: manual}, "POST", "/auth/register", `{"name":"Ana","email":"a@acme.io"}`)
		_, rst := call(t, &recorder{manual: manual}, "POST", "/auth/password-reset/request", `{"email":"a@acme.io"}`)
		if (reg["activation_token"] != nil) != manual || (rst["reset_token"] != nil) != manual {
			t.Errorf("manual=%v: %v %v", manual, reg, rst)
		}
	}
}

func TestBrokenBodiesStopBeforeTheService(t *testing.T) {
	for _, r := range [][2]string{{"POST", "/auth/register"}, {"POST", "/auth/activate"}, {"POST", "/auth/password-reset/request"}, {"POST", "/auth/password-reset/confirm"}, {"PATCH", "/me"}, {"POST", "/me/password"}} {
		rec := &recorder{}
		if status, _ := call(t, rec, r[0], r[1], `{`); status != 400 || rec.got != nil {
			t.Errorf("%s %s: %d, service got %v", r[0], r[1], status, rec.got)
		}
	}
}
