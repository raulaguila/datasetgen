package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/logger"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// memoryHits counts like the Postgres limiter, without the minute window.
func memoryHits() middleware.Hits {
	var mu sync.Mutex
	counts := map[string]int64{}
	return func(_ context.Context, key string) (int64, error) {
		mu.Lock()
		defer mu.Unlock()
		counts[key]++
		return counts[key], nil
	}
}

func TestRequestIDReachesTheResponseAndTheLog(t *testing.T) {
	var out bytes.Buffer
	log := logger.New(logger.Config{Format: "json", Level: "info", Out: &out})
	app := fiber.New()
	app.Use(middleware.RequestID(), middleware.RequestLog(log, true))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(reqctx.RequestID(c.Context())) })

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(fiber.HeaderXRequestID, "chosen-by-client") // no trusted proxy: ignored
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	id := resp.Header.Get(fiber.HeaderXRequestID)
	if id == "" || id == "chosen-by-client" || string(body) != id {
		t.Fatalf("header %q body %q", id, body)
	}
	if !strings.Contains(out.String(), `"request_id":"`+id+`"`) {
		t.Fatalf("log %s", out.String())
	}
}

func TestRateLimitAnswers429AsJSON(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RateLimit("t", 2, memoryHits()))
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	var last int
	var body []byte
	for range 3 {
		resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		last = resp.StatusCode
		body, _ = io.ReadAll(resp.Body)
	}
	if last != fiber.StatusTooManyRequests {
		t.Fatalf("status %d", last)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil || parsed["code"] != string(apperror.CodeTooManyRequests) {
		t.Fatalf("body %s", body)
	}
}

func TestRequestLogRecordsPanics(t *testing.T) {
	var out bytes.Buffer
	log := logger.New(logger.Config{Format: "json", Level: "info", Out: &out})
	app := fiber.New()
	app.Use(middleware.RequestLog(log, false))
	app.Use(recoverer.New())
	app.Get("/boom", func(fiber.Ctx) error { panic("handler exploded") })

	resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(out.String(), "handler exploded") {
		t.Fatalf("log: %s", out.String())
	}
}

func TestTimeoutReachesTheHandler(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.Timeout(time.Hour))
	app.Get("/", func(c fiber.Ctx) error {
		if _, ok := c.Context().Deadline(); !ok {
			return c.SendStatus(fiber.StatusTeapot)
		}
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("%v %v", resp, err)
	}
}

func TestRequestLogKeepsCauseFromClientAndLog(t *testing.T) {
	var out bytes.Buffer
	log := logger.New(logger.Config{Format: "json", Level: "info", Out: &out})
	app := fiber.New()
	app.Use(middleware.RequestLog(log, false))
	app.Get("/boom", func(c fiber.Ctx) error {
		return mapper.WriteError(c, apperror.New(apperror.CodeDatabaseError, "db error").WithCause(errors.New("pg is on fire")))
	})
	app.Get("/fine", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	resp, err := app.Test(httptest.NewRequest("GET", "/boom?token=secret", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "pg is on fire") {
		t.Fatalf("cause leaked to client: %s", body)
	}
	logged := out.String()
	if !strings.Contains(logged, "pg is on fire") || strings.Contains(logged, "secret") {
		t.Fatalf("log: %s", logged)
	}

	out.Reset()
	if _, err := app.Test(httptest.NewRequest("GET", "/fine", nil)); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("2xx logged with all=false: %s", out.String())
	}
}

func TestRequestLogReportsDegradedAnswers(t *testing.T) {
	var out bytes.Buffer
	log := logger.New(logger.Config{Format: "json", Level: "info", Out: &out})
	app := fiber.New()
	app.Use(middleware.RequestLog(log, false))
	app.Get("/", func(c fiber.Ctx) error {
		c.Locals(mapper.LocalCause, errors.New("profile lost"))
		return c.SendStatus(fiber.StatusOK)
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/", nil)); err != nil {
		t.Fatal(err)
	}
	if line := out.String(); !strings.Contains(line, "Request degraded") || !strings.Contains(line, "profile lost") {
		t.Fatalf("log: %s", line)
	}
}

func TestRequestLogModes(t *testing.T) {
	for _, all := range []bool{false, true} {
		mode := "errors only"
		if all {
			mode = "all requests"
		}
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				status  int
				level   string
				message string
				handler fiber.Handler
			}{
				{"success", 200, "info", "HTTP Request", func(c fiber.Ctx) error { return c.SendStatus(200) }},
				{"client error", 400, "warn", "Client error", func(c fiber.Ctx) error { return c.SendStatus(400) }},
				{"mapped error", 500, "error", "Server error", func(c fiber.Ctx) error {
					return mapper.WriteError(c, apperror.New(apperror.CodeDatabaseError, "db error").WithCause(errors.New("database unavailable")))
				}},
				{"returned error", 500, "error", "Server error", func(fiber.Ctx) error {
					return errors.New("database unavailable")
				}},
				{"degraded", 200, "warn", "Request degraded", func(c fiber.Ctx) error {
					c.Locals(mapper.LocalCause, errors.New("database unavailable"))
					return c.SendStatus(200)
				}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var out bytes.Buffer
					log := logger.New(logger.Config{Format: "json", Level: "info", Out: &out})
					app := fiber.New(fiber.Config{ErrorHandler: mapper.ErrorHandler})
					app.Use(middleware.RequestLog(log, all))
					app.Get("/check", tc.handler)
					req := httptest.NewRequest("GET", "/check?token=query-secret", nil)
					req.Header.Set("Authorization", "Bearer header-secret")
					resp, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = resp.Body.Close() }()
					if resp.StatusCode != tc.status {
						t.Fatalf("status %d, want %d", resp.StatusCode, tc.status)
					}
					body, _ := io.ReadAll(resp.Body)
					if strings.Contains(string(body), "database unavailable") {
						t.Fatalf("cause leaked to client: %s", body)
					}
					if !all && tc.status < 500 && tc.name != "degraded" {
						if out.Len() != 0 {
							t.Fatalf("unexpected log: %s", out.String())
						}
						return
					}
					if strings.Contains(out.String(), "secret") {
						t.Fatalf("request secrets leaked: %s", out.String())
					}
					var entry map[string]any
					// Unmarshal also rejects duplicate log entries for one request.
					if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
						t.Fatalf("expected one JSON log: %v; %s", err, out.String())
					}
					if entry["level"] != tc.level || entry["message"] != tc.message || entry["path"] != "/check" || entry["status"] != float64(tc.status) {
						t.Fatalf("unexpected log: %s", out.String())
					}
					if (tc.status == 500 || tc.name == "degraded") && !strings.Contains(out.String(), "database unavailable") {
						t.Fatalf("missing cause: %s", out.String())
					}
				})
			}
		})
	}
}
