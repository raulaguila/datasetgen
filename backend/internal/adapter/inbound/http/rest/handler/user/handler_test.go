package user_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/user"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// stubAuthz records what the route asked for and answers err.
type stubAuthz struct {
	err              error
	resource, action *string
}

func (s stubAuthz) Authorize(_ context.Context, _ domain.Principal, resource string, need domain.Action) error {
	*s.resource, *s.action = resource, strings.Join(domain.ActionNames(need), ",")
	return s.err
}

func newApp(svc input.UserService, authz input.Authorizer) *fiber.App {
	app := fiber.New()
	group := app.Group("/users", func(c fiber.Ctx) error {
		c.Locals(middleware.LocalUserID, "actor")
		return c.Next()
	})
	user.New(svc, nil).Routes(group, authz)
	return app
}

func TestEveryRouteNeedsItsPermission(t *testing.T) {
	for _, tc := range []struct{ method, path, action string }{
		{"GET", "/users", "read"},
		{"GET", "/users/u1", "read"},
		{"POST", "/users/disable", "update"},
		{"POST", "/users/enable", "update"},
		{"POST", "/users/unlock", "update"},
		{"DELETE", "/users/u1/sessions", "update"},
		{"POST", "/users", "create"},
		{"POST", "/users/u1/invite", "update"},
	} {
		var resource, action string
		denied := apperror.New(apperror.CodeForbidden, "forbidden")
		app := newApp(nil, stubAuthz{denied, &resource, &action}) // a nil service proves nothing runs past the check

		resp, err := app.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("%s %s: %d", tc.method, tc.path, resp.StatusCode)
		}
		if resource != domain.ResourceUsers || action != tc.action {
			t.Errorf("%s %s asked for %s:%s", tc.method, tc.path, resource, action)
		}
	}
}

// onlyUsers grants every users action and nothing else.
type onlyUsers struct{}

func (onlyUsers) Authorize(_ context.Context, _ domain.Principal, resource string, _ domain.Action) error {
	if resource == domain.ResourceUsers {
		return nil
	}
	return apperror.New(apperror.CodeForbidden, "forbidden")
}

// A pending account may already hold roles, and its link signs in as it: users
// rights alone must not hand that over.
func TestActivationLinkNeedsRoleRights(t *testing.T) {
	resp, err := newApp(nil, onlyUsers{}).Test(httptest.NewRequest("POST", "/users/u1/activation-link", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

type stubUsers struct {
	input.UserService
	filter domain.UserFilter
	actor  string
	ids    []string
}

func (s *stubUsers) List(_ context.Context, filter domain.UserFilter) ([]domain.User, int64, error) {
	s.filter = filter
	return []domain.User{{ID: "u1", Email: "a@acme.io"}}, 21, nil
}

func (s *stubUsers) Get(context.Context, string) (*domain.UserDetails, error) {
	return &domain.UserDetails{User: domain.User{ID: "u1", Email: "a@acme.io"}, Roles: []domain.Role{{
		ID: "r1", Name: "support", IsSuperuser: true,
	}}}, nil
}

func (s *stubUsers) DisableMany(_ context.Context, actorID string, ids []string) error {
	s.actor, s.ids = actorID, ids
	return nil
}

func call(t *testing.T, svc input.UserService, method, path string) (int, map[string]any) {
	t.Helper()
	var resource, action string
	resp, err := newApp(svc, stubAuthz{nil, &resource, &action}).Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out) // a 204 has no body
	return resp.StatusCode, out
}

func TestListPassesTheFilterAndPages(t *testing.T) {
	svc := &stubUsers{}
	status, out := call(t, svc, "GET", "/users?page=2&limit=10&search=ana&status=locked")
	if status != fiber.StatusOK {
		t.Fatalf("%d %v", status, out)
	}
	want := domain.UserFilter{Page: domain.NewPage(2, 10), Search: "ana", Status: "locked"}
	if svc.filter != want {
		t.Fatalf("filter %+v", svc.filter)
	}
	pagination, _ := out["pagination"].(map[string]any)
	if pagination["total_pages"] != float64(3) || len(out["items"].([]any)) != 1 {
		t.Fatalf("%v", out)
	}
}

func TestGetShowsOnlyTheRoleNames(t *testing.T) {
	status, out := call(t, &stubUsers{}, "GET", "/users/u1")
	if status != fiber.StatusOK || out["email"] != "a@acme.io" {
		t.Fatalf("%d %v", status, out)
	}
	roles, _ := out["roles"].([]any)
	if len(roles) != 1 {
		t.Fatalf("%v", out)
	}
	role, _ := roles[0].(map[string]any)
	if len(role) != 2 || role["id"] != "r1" || role["name"] != "support" {
		t.Fatalf("role %v", role)
	}
}

func TestDisableSaysWhoAsked(t *testing.T) {
	svc := &stubUsers{}
	req := httptest.NewRequest("POST", "/users/disable", strings.NewReader(`{"ids":["u1"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newApp(svc, stubAuthz{nil, new(string), new(string)}).Test(req)
	if err != nil || resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status %v %v", resp, err)
	}
	if svc.actor != "actor" || len(svc.ids) != 1 || svc.ids[0] != "u1" {
		t.Fatalf("%+v", svc)
	}
}
