package mapper_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestFiberErrorsAnswerJSON(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: mapper.ErrorHandler, BodyLimit: 8})
	app.Post("/known", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	for name, c := range map[string]struct {
		req    string
		path   string
		status int
		code   string
	}{
		"unknown route": {"GET", "/nope", fiber.StatusNotFound, "NOT_FOUND"},
		"wrong method":  {"GET", "/known", fiber.StatusMethodNotAllowed, "INVALID_INPUT"},
	} {
		resp, err := app.Test(httptest.NewRequest(c.req, c.path, strings.NewReader("")))
		if err != nil {
			t.Fatal(err)
		}
		var body struct{ Code string }
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if resp.StatusCode != c.status || body.Code != c.code {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, body.Code)
		}
	}
}

func TestRetryAfterDetailBecomesAHeader(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return mapper.WriteError(c, apperror.New(apperror.CodeTooManyRequests, "slow down").WithDetails(apperror.DetailRetryAfter, 900))
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests || resp.Header.Get("Retry-After") != "900" {
		t.Fatalf("%d %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}
