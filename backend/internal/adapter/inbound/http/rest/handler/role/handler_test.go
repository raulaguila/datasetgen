package role_test

import (
	"context"
	"io"
	"maps"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/role"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// stubAuthz denies everything and records what the route asked for.
type stubAuthz struct{ resource, action *string }

func (s stubAuthz) Authorize(_ context.Context, _ domain.Principal, resource string, need domain.Action) error {
	*s.resource, *s.action = resource, strings.Join(domain.ActionNames(need), ",")
	return apperror.New(apperror.CodeForbidden, "forbidden")
}

func newApp(svc input.RoleService, authz input.Authorizer) *fiber.App {
	app := fiber.New()
	group := app.Group("/roles", func(c fiber.Ctx) error {
		c.Locals(middleware.LocalUserID, "u1")
		return c.Next()
	})
	role.New(svc).Routes(group, authz)
	return app
}

func TestEveryRouteNeedsItsPermission(t *testing.T) {
	for _, tc := range []struct{ method, path, action string }{
		{"GET", "/roles", "read"},
		{"POST", "/roles", "create"},
		{"POST", "/roles/delete", "delete"},
		{"PATCH", "/roles/r1", "update"},
		{"PUT", "/roles/r1/permissions", "update"},
		{"GET", "/roles/r1/members", "read"},
		{"PUT", "/roles/r1/members/u2", "update"},
		{"DELETE", "/roles/r1/members/u2", "update"},
	} {
		var resource, action string
		app := newApp(nil, stubAuthz{&resource, &action}) // a nil service proves nothing runs past the check

		resp, err := app.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("%s %s: %d", tc.method, tc.path, resp.StatusCode)
		}
		if resource != domain.ResourceRoles || action != tc.action {
			t.Errorf("%s %s asked for %s:%s", tc.method, tc.path, resource, action)
		}
	}
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, domain.Principal, string, domain.Action) error { return nil }

type stubRoles struct {
	input.RoleService
	got map[string]domain.Action
}

func (s *stubRoles) SetPermissions(_ context.Context, _ string, permissions map[string]domain.Action) (*domain.Role, error) {
	s.got = permissions
	return &domain.Role{Permissions: permissions}, nil
}

func TestSetPermissionsReadsTheMap(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   map[string]domain.Action
	}{
		{`{"permissions":{"audit":["read","update"],"users":[]}}`, fiber.StatusOK, map[string]domain.Action{
			"audit": domain.ActionRead | domain.ActionUpdate, "users": 0,
		}},
		{`{"permissions":{}}`, fiber.StatusOK, map[string]domain.Action{}},
		{`{"permissions":{"audit":["fly"]}}`, fiber.StatusBadRequest, nil},
		{`{}`, fiber.StatusBadRequest, nil},
	} {
		svc := &stubRoles{}
		req := httptest.NewRequest("PUT", "/roles/r1/permissions", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := newApp(svc, allowAll{}).Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != tc.status || !maps.Equal(svc.got, tc.want) {
			t.Errorf("%s: %d %s, permissions %v", tc.body, resp.StatusCode, body, svc.got)
		}
	}
}
