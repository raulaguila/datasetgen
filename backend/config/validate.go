package config

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/raulaguila/dataset/pkg/aescrypt"
)

const minSecretLength = 16

// ServiceMailer names the process that issues no token, so it needs no fixed key.
const ServiceMailer = "mailer"

func validateEnvironment(env *Environment) error {
	if env == nil {
		return errors.New("environment is required")
	}

	if _, err := aescrypt.ParseKey(env.SMTP.MasterKey); err != nil {
		return fmt.Errorf("SMTP_MASTER_KEY: %w", err)
	}
	if u, err := url.Parse(env.SMTP.AppURL); env.SMTP.AppURL != "" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "") {
		return errors.New("SMTP_APP_URL must be an http or https address")
	}

	if isProtectedEnvironment(env.Environment) {
		if origins := strings.TrimSpace(env.API.CorsOrigins); origins == "" || origins == "*" {
			return fmt.Errorf("CORS_ORIGINS cannot be empty or '*' in %s environment", env.Environment)
		}
		if env.Auth.AllowManualTokenDelivery {
			return fmt.Errorf("AUTH_MANUAL_TOKEN_DELIVERY cannot be enabled in %s environment", env.Environment)
		}
		if err := validateProtectedSecrets(env); err != nil {
			return err
		}
		if err := validateProtectedPostgres(env); err != nil {
			return err
		}
		if err := validateProtectedSMTP(env); err != nil {
			return err
		}
	}
	if err := validateAPIConfig(env.API); err != nil {
		return err
	}
	if err := validateAuthConfig(env.Auth); err != nil {
		return err
	}
	if env.Postgres.MaxConns <= 0 {
		return errors.New("POSTGRES_MAX_CONNS must be greater than 0")
	}
	return validateStorage(env.Storage)
}

func validateStorage(cfg StorageConfig) error {
	switch cfg.Driver {
	case "", "local": // empty only in hand-built configs; the loader defaults to local
		if cfg.Driver == "local" && cfg.LocalDir == "" {
			return errors.New("STORAGE_LOCAL_DIR is required when STORAGE_DRIVER is local")
		}
	case "s3":
		if u, err := url.Parse(cfg.S3Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("STORAGE_S3_ENDPOINT must be an http or https address")
		}
		if cfg.S3Bucket == "" || cfg.S3Region == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
			return errors.New("STORAGE_S3_BUCKET, STORAGE_S3_REGION, STORAGE_S3_ACCESS_KEY and STORAGE_S3_SECRET_KEY are required when STORAGE_DRIVER is s3")
		}
	default:
		return errors.New("STORAGE_DRIVER must be local or s3")
	}
	return nil
}

func validateAPIConfig(cfg APIConfig) error {
	if cfg.GlobalRateLimit <= 0 || cfg.AuthRateLimit <= 0 || cfg.PasswordRateLimit <= 0 {
		return errors.New("API_RATE_LIMIT_* must be greater than 0")
	}
	if cfg.RequestTimeout <= 0 {
		return errors.New("API_REQUEST_TIMEOUT must be greater than 0")
	}
	return nil
}

func validateAuthConfig(cfg AuthConfig) error {
	if cfg.AccessExpiration <= 0 || cfg.RefreshExpiration <= 0 || cfg.RegistrationTokenTTL <= 0 || cfg.PasswordResetTokenTTL <= 0 {
		return errors.New("token expiration durations must be greater than 0")
	}
	if cfg.LoginAttemptEmailWindow <= 0 || cfg.LoginAttemptIPWindow <= 0 || cfg.EmailTokenCooldown < 0 || cfg.EmailTokenHourlyMax < 0 {
		return errors.New("login windows must be positive; email cooldown and cap cannot be negative")
	}
	if cfg.MaxFailedLoginAttempts <= 0 {
		return errors.New("AUTH_MAX_FAILED_LOGIN_ATTEMPTS must be greater than 0")
	}
	if cfg.LoginAttemptPairMaxFailures <= 0 {
		return errors.New("AUTH_LOGIN_ATTEMPT_PAIR_MAX must be greater than 0")
	}
	if cfg.LoginAttemptEmailMaxFailures <= 0 || cfg.LoginAttemptIPMaxFailures <= 0 { // 0 would refuse every login
		return errors.New("AUTH_LOGIN_ATTEMPT_EMAIL_MAX and AUTH_LOGIN_ATTEMPT_IP_MAX must be greater than 0")
	}
	if cfg.SessionMaxLifetime <= 0 {
		return errors.New("AUTH_SESSION_MAX_LIFETIME must be greater than 0")
	}
	if cfg.AccountLockDuration <= 0 {
		return errors.New("AUTH_ACCOUNT_LOCK_DURATION must be greater than 0")
	}
	// With one key an access token passes as a refresh token, and the reverse.
	for _, access := range []ed25519.PrivateKey{cfg.AccessPrivateKey, cfg.AccessPreviousKey} {
		for _, refresh := range []ed25519.PrivateKey{cfg.RefreshPrivateKey, cfg.RefreshPreviousKey} {
			if access != nil && refresh != nil && access.Equal(refresh) {
				return errors.New("the access keys (ACCESS_TOKEN, ACCESS_TOKEN_PREVIOUS) and the refresh keys (REFRESH_TOKEN, REFRESH_TOKEN_PREVIOUS) must differ")
			}
		}
	}
	return nil
}

// validateProtectedPostgres refuses a database reached over the network without
// a verified TLS connection. The compose network and loopback stay exempt.
func validateProtectedPostgres(env *Environment) error {
	cfg, err := pgconn.ParseConfig(env.Postgres.URL)
	if err != nil {
		return fmt.Errorf("POSTGRES_URL: %w", err)
	}
	if isLocalPostgres(cfg.Host) {
		return nil
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify {
		return fmt.Errorf("POSTGRES_URL must use sslmode=verify-full for host %s in %s environment", cfg.Host, env.Environment)
	}
	return nil
}

func isLocalPostgres(host string) bool {
	switch host {
	case "postgres", "localhost", "127.0.0.1", "::1":
		return true
	default:
		return strings.HasPrefix(host, "/") // unix socket
	}
}

func validateProtectedSecrets(env *Environment) error {
	// "new" generates a key per process: tokens fail across processes and restarts.
	if env.Service != ServiceMailer {
		for _, name := range []string{"ACCESS_TOKEN", "REFRESH_TOKEN"} {
			if value := strings.TrimSpace(os.Getenv(name)); value == "" || value == "new" {
				return fmt.Errorf("%s must hold a fixed key in %s environment", name, env.Environment)
			}
		}
	}
	if len(env.Postgres.Password) < minSecretLength {
		return fmt.Errorf("POSTGRES_PASS must have at least %d characters in %s environment", minSecretLength, env.Environment)
	}
	return nil
}

func validateProtectedSMTP(env *Environment) error {
	if !env.SMTP.Enabled {
		return fmt.Errorf("SMTP_ENABLED is required in %s environment: without it no account can be activated", env.Environment)
	}
	if !env.SMTP.Security.UsesTLS() {
		return fmt.Errorf("SMTP_SECURITY must be tls or starttls in %s environment", env.Environment)
	}

	required := map[string]string{
		"SMTP_HOST": env.SMTP.Host,
		"SMTP_USER": env.SMTP.User,
		"SMTP_PASS": env.SMTP.Password,
		"SMTP_FROM": env.SMTP.From,
		// Without it the emails carry a bare token and nobody knows where to use it.
		"SMTP_APP_URL": env.SMTP.AppURL,
	}

	for field, value := range required {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s is required when SMTP_ENABLED=1 in %s environment", field, env.Environment)
		}
		if isSMTPPlaceholder(trimmed) {
			return fmt.Errorf("%s cannot use placeholder value in %s environment", field, env.Environment)
		}
	}

	return nil
}

func isSMTPPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "change-me" || normalized == "changeme" || strings.Contains(normalized, "example.com")
}

// isProtectedEnvironment fails closed: any name not known as local, including
// a typo of "production", gets the production rules.
func isProtectedEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "development", "dev", "local", "test":
		return false
	default:
		return true
	}
}
