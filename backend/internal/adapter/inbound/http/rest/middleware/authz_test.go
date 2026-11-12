package middleware_test

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type stubAuthz struct {
	deny bool
}

func (s stubAuthz) Authorize(_ context.Context, _ domain.Principal, _ string, _ domain.Action) error {
	if s.deny {
		return apperror.New(apperror.CodeForbidden, "forbidden")
	}
	return nil
}

func withUser(c fiber.Ctx) error {
	c.Locals(middleware.LocalUserID, "u1")
	return c.Next()
}

func TestRequireAllowsAndDenies(t *testing.T) {
	app := fiber.New()
	app.Get("/ok", withUser, middleware.Require(stubAuthz{deny: false}, "products", domain.ActionRead), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Get("/deny", withUser, middleware.Require(stubAuthz{deny: true}, "products", domain.ActionRead), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("allow status %d", resp.StatusCode)
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/deny", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("deny status %d body %s", resp.StatusCode, body)
	}
}

type stubAuthn struct {
	calls *[]string
	err   error
}

func (s stubAuthn) Authenticate(_ context.Context, token string) (*domain.Principal, error) {
	*s.calls = append(*s.calls, token)
	if s.err != nil {
		return nil, s.err
	}
	return &domain.Principal{UserID: "u1", SessionID: "s1"}, nil
}

func (s stubAuthn) ParseAccess(token string) (*domain.Principal, error) {
	return s.Authenticate(context.Background(), token)
}

func TestBearer(t *testing.T) {
	cases := []struct {
		name   string
		header string
		err    error
		want   int
		called bool
	}{
		{"valid", "Bearer good", nil, fiber.StatusOK, true},
		{"no header", "", nil, fiber.StatusUnauthorized, false},
		{"wrong scheme", "Basic good", nil, fiber.StatusUnauthorized, false},
		{"empty token", "Bearer  ", nil, fiber.StatusUnauthorized, false},
		{"refused", "Bearer bad", apperror.New(apperror.CodeUnauthorized, "x"), fiber.StatusUnauthorized, true},
		{"database down", "Bearer good", apperror.New(apperror.CodeDatabaseError, "x"), fiber.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			app := fiber.New()
			app.Get("/", middleware.Bearer(stubAuthn{calls: &calls, err: tc.err}), func(c fiber.Ctx) error {
				if middleware.UserID(c) != "u1" || middleware.SessionID(c) != "s1" {
					return c.SendStatus(fiber.StatusTeapot)
				}
				return c.SendStatus(fiber.StatusOK)
			})
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tc.header)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if (len(calls) == 1) != tc.called {
				t.Fatalf("authenticator calls: %v", calls)
			}
		})
	}
}
