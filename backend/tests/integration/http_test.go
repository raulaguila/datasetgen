package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest"
	"github.com/raulaguila/dataset/internal/composition"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/internal/infra"
	"github.com/raulaguila/dataset/pkg/logger"
)

func (e *env) httpApp() *fiber.App {
	cfg := &config.Environment{Environment: "production", API: config.APIConfig{CorsOrigins: "https://app.acme.io", RequestTimeout: 10 * time.Second, GlobalRateLimit: 1 << 30, AuthRateLimit: 100000, PasswordRateLimit: 100000}}
	log := logger.New(logger.Config{Format: "json", Out: io.Discard})
	app := composition.NewApplication(cfg, &infra.Resources{PostgresDB: testDB}, log)
	app.Auth = e.svc
	return rest.New(cfg, app, composition.NewRateLimit(&infra.Resources{PostgresDB: testDB}), log)
}

func TestHTTPAccountOwnershipAndPermissions(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	other := e.activeUser(t)
	ctx := context.Background()
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := e.login(other, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	otherClaims, err := e.svc.Tokens.ParseAccess(foreign.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	note := &domain.Notification{UserID: e.userID(t, other), Level: domain.NotificationInfo, Title: "Private", Message: "Private"}
	if err := e.store.Notifications().Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	request := func(method, path, body, token string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.StatusCode, want, data)
		}
		var out map[string]any
		if len(data) > 0 && want != 204 {
			_ = json.Unmarshal(data, &out)
		}
		return out
	}
	request("GET", "/v1/me/preferences", "", "", 401)
	request("GET", "/v1/me/sessions", "", pair.AccessToken, 200)
	request("DELETE", "/v1/me/sessions/"+otherClaims.SessionID, "", pair.AccessToken, 404)
	request("GET", "/v1/me/preferences", "", pair.AccessToken, 200)
	request("PUT", "/v1/me/preferences", `{"theme":"dark","language":"pt-BR"}`, pair.AccessToken, 200)
	request("PUT", "/v1/me/preferences", `{"theme":"invalid","language":"pt-BR"}`, pair.AccessToken, 400)
	request("PUT", "/v1/me/preferences", `{`, pair.AccessToken, 400)
	request("GET", "/v1/me/notifications", "", pair.AccessToken, 200)
	request("GET", "/v1/me/notifications/count", "", pair.AccessToken, 200)
	request("PUT", "/v1/me/notifications/read", `{"ids":["`+note.ID+`"]}`, pair.AccessToken, 404)
	request("PUT", "/v1/me/notifications/read", "", pair.AccessToken, 204)
	request("GET", "/v1/audit/export?action=user.", "", pair.AccessToken, 403)
	if err := admin.New(e.store, e.clock).Promote(ctx, addr); err != nil {
		t.Fatal(err)
	}
	// Permission changes are effective with the existing access token.
	request("GET", "/v1/audit/export?action=bad%25", "", pair.AccessToken, 400)
	request("GET", "/v1/audit/export?action=user.", "", pair.AccessToken, 200)
	if _, err := e.svc.Authenticate(ctx, foreign.AccessToken); err != nil {
		t.Fatalf("other user's session was revoked: %v", err)
	}
}
