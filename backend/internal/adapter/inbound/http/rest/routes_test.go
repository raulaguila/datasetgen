package rest

import (
	"context"
	"io"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/logger"
)

// tokenOnly accepts any token without a session lookup; every other auth call panics.
type tokenOnly struct{ input.AuthService }

func (tokenOnly) ParseAccess(string) (*domain.Principal, error) {
	return &domain.Principal{UserID: "u1", SessionID: "s1"}, nil
}

type sessionGone struct{ input.Authorizer }

func (sessionGone) Authorize(context.Context, domain.Principal, string, domain.Action) error {
	return apperror.New(apperror.CodeUnauthorized, "session revoked")
}

// Groups behind Token check the session only in Require, so a route there
// without Require would serve a revoked session until the token expires.
func TestEveryTokenRouteChecksTheSession(t *testing.T) {
	cfg := &config.Environment{Environment: "production", API: config.APIConfig{CorsOrigins: "https://app.acme.io", RequestTimeout: time.Second, GlobalRateLimit: 1000}}
	hits := func(context.Context, string) (int64, error) { return 1, nil }
	app := New(cfg, &input.Application{Auth: tokenOnly{}, Authorizer: sessionGone{}}, hits, logger.New(logger.Config{Format: "json", Out: io.Discard}))
	param := regexp.MustCompile(`:[a-z_]+`)
	checked := 0
	for _, r := range app.GetRoutes(true) {
		if r.Method == fiber.MethodHead || !regexp.MustCompile(`^/v1/(permissions|roles|users|products|audit)(/|$)`).MatchString(r.Path) {
			continue
		}
		req := httptest.NewRequest(r.Method, param.ReplaceAllString(r.Path, "x"), strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer t")
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("%s %s answered %d with a revoked session", r.Method, r.Path, res.StatusCode)
		}
		checked++
	}
	if checked < 20 {
		t.Fatalf("only %d routes checked", checked)
	}
}
