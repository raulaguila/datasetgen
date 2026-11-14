package permission_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/permission"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type stubAuthz struct{ allow string }

func (s stubAuthz) Authorize(_ context.Context, _ domain.Principal, resource string, need domain.Action) error {
	if resource == s.allow && need == domain.ActionRead {
		return nil
	}
	return apperror.New(apperror.CodeForbidden, "forbidden")
}

type stubCatalog struct{}

func (stubCatalog) ListResources(context.Context) ([]domain.Resource, error) {
	return []domain.Resource{{Code: "audit", Description: "Example", Actions: domain.ActionRead | domain.ActionUpdate}}, nil
}

func TestResourcesNeedRolesRead(t *testing.T) {
	for _, tc := range []struct {
		allow  string
		status int
	}{
		{domain.ResourceRoles, fiber.StatusOK},
		{domain.ResourceAudit, fiber.StatusForbidden},
	} {
		app := fiber.New()
		group := app.Group("/permissions", func(c fiber.Ctx) error {
			c.Locals(middleware.LocalUserID, "u1")
			return c.Next()
		})
		permission.New(stubCatalog{}).Routes(group, stubAuthz{allow: tc.allow})

		resp, err := app.Test(httptest.NewRequest("GET", "/permissions/resources", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != tc.status {
			t.Fatalf("allow %s: %d %s", tc.allow, resp.StatusCode, body)
		}
		if tc.status == fiber.StatusOK && (!strings.Contains(string(body), `"code":"audit"`) || !strings.Contains(string(body), `"actions":["read","update"]`)) {
			t.Fatalf("body %s", body)
		}
	}
}
