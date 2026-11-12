package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/logger"
)

func server(environment string) *fiber.App {
	cfg := &config.Environment{Environment: environment}
	cfg.API.GlobalRateLimit, cfg.API.AuthRateLimit, cfg.API.PasswordRateLimit = 1000, 1000, 1000
	cfg.API.CorsOrigins = "https://app.test"
	return New(cfg, &input.Application{}, memoryHits(), logger.New(logger.Config{}))
}

func get(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	res, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

func TestSwaggerMatchesTheRoutes(t *testing.T) {
	app := server("development")
	status, body := get(t, app, "/swagger/doc.json")
	if status != fiber.StatusOK {
		t.Fatalf("doc.json: %d", status)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Tags []string `json:"tags"`
		} `json:"paths"`
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
		Groups []struct {
			Tags []string `json:"tags"`
		} `json:"x-tagGroups"`
	}
	if err := json.Unmarshal([]byte(body), &spec); err != nil {
		t.Fatal(err)
	}

	var order, grouped []string
	for _, tag := range spec.Tags {
		order = append(order, tag.Name)
	}
	for _, group := range spec.Groups {
		grouped = append(grouped, group.Tags...)
	}
	if !slices.Equal(order, grouped) {
		t.Errorf("x-tagGroups %v must list the tags in order %v", grouped, order)
	}

	param := regexp.MustCompile(`:(\w+)`)
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			if len(op.Tags) == 0 {
				t.Errorf("%s %s has no group", method, path)
			}
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	for _, route := range app.GetRoutes(true) {
		if route.Method == fiber.MethodHead || route.Method == fiber.MethodOptions || strings.HasPrefix(route.Path, "/swagger") {
			continue
		}
		path := param.ReplaceAllString(strings.TrimSuffix(route.Path, "/"), "{$1}")
		key := route.Method + " " + path
		if !documented[key] {
			t.Errorf("%s not documented: add annotations and run make swagger", key)
		}
		delete(documented, key)
	}
	for key := range documented {
		t.Errorf("%s documented but not routed", key)
	}
}

func TestSwaggerStaysOutOfProduction(t *testing.T) {
	if status, _ := get(t, server("production"), "/swagger/index.html"); status != fiber.StatusNotFound {
		t.Fatalf("production: %d", status)
	}
	status, body := get(t, server("development"), "/swagger/index.html")
	if status != fiber.StatusOK || !strings.Contains(body, "invert(") || !strings.Contains(body, "TagGroups") || !strings.Contains(body, `"validatorUrl":"none"`) {
		t.Fatalf("development: %d", status)
	}
}
