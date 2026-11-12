package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/pkg/smtpsecurity"
)

const validKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func productionEnv(t *testing.T) *Environment {
	t.Helper()
	t.Setenv("ACCESS_TOKEN", "fixed-key")
	t.Setenv("REFRESH_TOKEN", "fixed-key")
	return &Environment{
		Environment: "production",
		API:         APIConfig{CorsOrigins: "https://app.acme.io", GlobalRateLimit: 100, AuthRateLimit: 10, PasswordRateLimit: 3, RequestTimeout: 1},
		Auth:        AuthConfig{AccessExpiration: 1, RefreshExpiration: 1, RegistrationTokenTTL: 1, PasswordResetTokenTTL: 1, LoginAttemptEmailWindow: 1, LoginAttemptIPWindow: 1, SessionMaxLifetime: 1, MaxFailedLoginAttempts: 5, AccountLockDuration: 1, LoginAttemptPairMaxFailures: 5, LoginAttemptEmailMaxFailures: 5, LoginAttemptIPMaxFailures: 5},
		Postgres:    PostgresConfig{Password: strings.Repeat("p", minSecretLength), MaxConns: 40},
		SMTP: SMTPConfig{
			MasterKey: validKey, Enabled: true, Security: smtpsecurity.ModeStartTLS,
			Host: "smtp.acme.io", User: "u", Password: "p", From: "no-reply@acme.io", AppURL: "https://id.acme.io",
		},
	}
}

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestValidProductionPasses(t *testing.T) {
	for _, url := range []string{
		"", // default compose host
		"host=postgres user=u dbname=d sslmode=disable",
		"postgres://u:p@db.acme.io/d?sslmode=verify-full",
	} {
		env := productionEnv(t)
		env.Postgres.URL = url
		env.Auth.AccessPrivateKey, env.Auth.RefreshPrivateKey = testKey(t), testKey(t)
		if err := validateEnvironment(env); err != nil {
			t.Fatalf("%q: %v", url, err)
		}
	}
}

func TestProductionRefuses(t *testing.T) {
	cases := map[string]func(*Environment){
		"mistyped env keeps manual token off": func(e *Environment) {
			e.Environment = "producao"
			e.Auth.AllowManualTokenDelivery = true
		},
		"manual token":  func(e *Environment) { e.Auth.AllowManualTokenDelivery = true },
		"wildcard cors": func(e *Environment) { e.API.CorsOrigins = "*" },
		"weak db pass":  func(e *Environment) { e.Postgres.Password = "root" },
		"smtp off":      func(e *Environment) { e.SMTP.Enabled = false },
		"smtp no tls":   func(e *Environment) { e.SMTP.Security = smtpsecurity.ModeNone },
		"bad token key": func(e *Environment) { e.SMTP.MasterKey = "short" },
		"no app url":    func(e *Environment) { e.SMTP.AppURL = "" },
		"bad app url":   func(e *Environment) { e.SMTP.AppURL = "id.acme.io" },
		"no rate limit": func(e *Environment) { e.API.AuthRateLimit = 0 },
		"no timeout":    func(e *Environment) { e.API.RequestTimeout = 0 },
		"remote db without tls": func(e *Environment) {
			e.Postgres.URL = "host=db.acme.io user=u dbname=d sslmode=disable"
		},
		"remote db without verification": func(e *Environment) {
			e.Postgres.URL = "postgres://u:p@db.acme.io/d?sslmode=require"
		},
		"no db pool": func(e *Environment) { e.Postgres.MaxConns = 0 },
		"same jwt key": func(e *Environment) {
			e.Auth.AccessPrivateKey = testKey(t)
			e.Auth.RefreshPrivateKey = e.Auth.AccessPrivateKey
		},
		"previous access key is the refresh key": func(e *Environment) {
			e.Auth.AccessPrivateKey, e.Auth.RefreshPrivateKey = testKey(t), testKey(t)
			e.Auth.AccessPreviousKey = e.Auth.RefreshPrivateKey
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			env := productionEnv(t)
			breakIt(env)
			if err := validateEnvironment(env); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestProductionRefusesPerProcessKey(t *testing.T) {
	env := productionEnv(t)
	t.Setenv("ACCESS_TOKEN", "new")
	if err := validateEnvironment(env); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestLocalAllowsManualToken(t *testing.T) {
	env := productionEnv(t)
	env.Environment = "development"
	env.Auth.AllowManualTokenDelivery = true
	env.API.CorsOrigins = "*"
	if err := validateEnvironment(env); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyTheMailerRunsWithoutFixedKeys(t *testing.T) {
	env := productionEnv(t)
	t.Setenv("ACCESS_TOKEN", "new")
	if err := validateEnvironment(env); err == nil || !strings.Contains(err.Error(), "ACCESS_TOKEN") {
		t.Fatalf("backend: %v", err)
	}
	env.Service = ServiceMailer
	if err := validateEnvironment(env); err != nil {
		t.Fatalf("mailer: %v", err)
	}
}

// Zero would refuse every login, while elsewhere it can read as "no limit".
func TestZeroLoginLimitIsRefused(t *testing.T) {
	for _, set := range []func(*AuthConfig){
		func(a *AuthConfig) { a.LoginAttemptEmailMaxFailures = 0 },
		func(a *AuthConfig) { a.LoginAttemptIPMaxFailures = 0 },
	} {
		env := productionEnv(t)
		set(&env.Auth)
		if err := validateEnvironment(env); err == nil {
			t.Fatal("a zero login limit must not start")
		}
	}
}
