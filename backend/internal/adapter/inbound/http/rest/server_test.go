package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
)

func clientIP(t *testing.T, api config.APIConfig, header string) string {
	t.Helper()
	app := fiber.New(serverConfig(api))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(c.IP()) })

	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", header)
	req.Header.Set("X-Forwarded-For", header)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	buf := make([]byte, 64)
	n, _ := res.Body.Read(buf)
	return string(buf[:n])
}

func TestClientIPHeader(t *testing.T) {
	const peer = "0.0.0.0" // the peer address of app.Test
	behindProxy := config.APIConfig{TrustedProxies: "10.0.0.0/8, " + peer, ProxyHeader: "X-Real-IP"}
	xff := config.APIConfig{TrustedProxies: "10.0.0.0/8, " + peer, ProxyHeader: "X-Forwarded-For"}

	cases := []struct {
		name   string
		api    config.APIConfig
		header string
		want   string
	}{
		{"no proxy ignores a forged header", config.APIConfig{ProxyHeader: "X-Real-IP"}, "9.9.9.9", peer},
		{"untrusted peer ignores the header", config.APIConfig{TrustedProxies: "10.0.0.1", ProxyHeader: "X-Real-IP"}, "9.9.9.9", peer},
		{"trusted proxy gives the client", behindProxy, "9.9.9.9", "9.9.9.9"},
		{"junk never reaches the inet column", behindProxy, "not-an-ip", peer},
		{"forged left of a chain is skipped", xff, "6.6.6.6, 9.9.9.9, 10.0.0.1", "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIP(t, tc.api, tc.header); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBodyLimitAnswersBeforeHandlers(t *testing.T) {
	app := fiber.New(serverConfig(config.APIConfig{}))
	app.Post("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	post := func(size int) (*http.Response, error) {
		return app.Test(httptest.NewRequest(fiber.MethodPost, "/", bytes.NewReader(make([]byte, size))))
	}
	res, err := post(maxBodyBytes)
	if err != nil || res.StatusCode != fiber.StatusOK {
		t.Fatalf("at the limit: %v %v", res, err)
	}
	if _, err := post(maxBodyBytes + 1); err == nil || !strings.Contains(err.Error(), "body size") {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestRateLimitGroupsIPv6ByPrefix(t *testing.T) {
	app := fiber.New(serverConfig(config.APIConfig{TrustedProxies: "0.0.0.0", ProxyHeader: "X-Real-IP"}))
	app.Use(middleware.RateLimit("t", 1, memoryHits()))
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	get := func(ip string) int {
		req := httptest.NewRequest(fiber.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", ip)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	for _, step := range []struct {
		ip   string
		want int
	}{
		{"2001:db8:1:1::1", fiber.StatusOK},
		{"2001:db8:1:1::2", fiber.StatusTooManyRequests},
		{"2001:db8:1:2::1", fiber.StatusOK},
		{"::ffff:192.0.2.1", fiber.StatusOK},
		{"::ffff:192.0.2.2", fiber.StatusOK},
	} {
		if got := get(step.ip); got != step.want {
			t.Fatalf("%s: got %d, want %d", step.ip, got, step.want)
		}
	}
}

func TestRateLimitCountsEachClientBehindProxy(t *testing.T) {
	app := fiber.New(serverConfig(config.APIConfig{TrustedProxies: "0.0.0.0", ProxyHeader: "X-Real-IP"}))
	app.Use(middleware.RateLimit("t", 1, memoryHits()))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(c.IP()) })

	get := func(ip string) int {
		req := httptest.NewRequest(fiber.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", ip)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	if got := get("1.1.1.1"); got != fiber.StatusOK {
		t.Fatalf("first client: %d", got)
	}
	if got := get("1.1.1.1"); got != fiber.StatusTooManyRequests {
		t.Fatalf("same client again: %d", got)
	}
	if got := get("2.2.2.2"); got != fiber.StatusOK {
		t.Fatalf("other client shares the limit: %d", got)
	}
}

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
