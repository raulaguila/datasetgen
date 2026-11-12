package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/godeh/dotenvgo"
	"github.com/golang-jwt/jwt/v5"

	"github.com/raulaguila/dataset/pkg/aescrypt"
	"github.com/raulaguila/dataset/pkg/smtpsecurity"
)

type LogConfig struct {
	Format string `env:"LOG_FORMAT" default:"json"`
	Level  string `env:"LOG_LEVEL" default:"info"`
}

type APIConfig struct {
	Port         int    `env:"API_PORT" default:"9977"`
	EnableLogger bool   `env:"API_LOGGER" default:"1"`
	CorsOrigins  string `env:"CORS_ORIGINS" default:"*"`

	// TrustedProxies lists comma-separated proxy IPs or CIDRs whose ProxyHeader is
	// believed. Empty means direct traffic. ProxyHeader is either one the proxy
	// overwrites, or X-Forwarded-For, read from the right past the trusted proxies.
	TrustedProxies string `env:"API_TRUSTED_PROXIES" default:""`
	ProxyHeader    string `env:"API_PROXY_HEADER" default:"X-Real-IP"`

	GlobalRateLimit   int `env:"API_RATE_LIMIT_GLOBAL" default:"300"` // per IP: behind NAT a whole office shares it
	AuthRateLimit     int `env:"API_RATE_LIMIT_AUTH" default:"10"`
	PasswordRateLimit int `env:"API_RATE_LIMIT_PASSWORD" default:"3"`

	// Timeouts applied to the Fiber/fasthttp server. Guards against slowloris
	// and leaked goroutines on stuck syscalls.
	ReadTimeout     time.Duration `env:"API_READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `env:"API_WRITE_TIMEOUT" default:"30s"`
	IdleTimeout     time.Duration `env:"API_IDLE_TIMEOUT" default:"120s"`
	ShutdownTimeout time.Duration `env:"API_SHUTDOWN_TIMEOUT" default:"30s"`
	RequestTimeout  time.Duration `env:"API_REQUEST_TIMEOUT" default:"10s"`
}

type AuthConfig struct {
	AccessPrivateKey ed25519.PrivateKey `env:"ACCESS_TOKEN" default:"new"`
	// Keys from before a rotation, verified but never used to sign. Empty: none.
	AccessPreviousKey            ed25519.PrivateKey `env:"ACCESS_TOKEN_PREVIOUS" default:""`
	RefreshPreviousKey           ed25519.PrivateKey `env:"REFRESH_TOKEN_PREVIOUS" default:""`
	AccessExpiration             time.Duration      `env:"ACCESS_TOKEN_EXPIRE" default:"15m"`
	RefreshPrivateKey            ed25519.PrivateKey `env:"REFRESH_TOKEN" default:"new"`
	RefreshExpiration            time.Duration      `env:"REFRESH_TOKEN_EXPIRE" default:"60m"`
	SessionMaxLifetime           time.Duration      `env:"AUTH_SESSION_MAX_LIFETIME" default:"720h"`
	RegistrationTokenTTL         time.Duration      `env:"REGISTRATION_TOKEN_TTL" default:"24h"`
	PasswordResetTokenTTL        time.Duration      `env:"PASSWORD_RESET_TOKEN_TTL" default:"2h"`
	MaxFailedLoginAttempts       int                `env:"AUTH_MAX_FAILED_LOGIN_ATTEMPTS" default:"50"`
	AccountLockDuration          time.Duration      `env:"AUTH_ACCOUNT_LOCK_DURATION" default:"15m"`
	AllowManualTokenDelivery     bool               `env:"AUTH_MANUAL_TOKEN_DELIVERY" default:"0"`
	AllowRegistration            bool               `env:"AUTH_ALLOW_REGISTRATION" default:"1"`
	LoginAttemptEmailWindow      time.Duration      `env:"AUTH_LOGIN_ATTEMPT_EMAIL_WINDOW" default:"15m"`
	LoginAttemptEmailMaxFailures int                `env:"AUTH_LOGIN_ATTEMPT_EMAIL_MAX" default:"50"`
	LoginAttemptPairMaxFailures  int                `env:"AUTH_LOGIN_ATTEMPT_PAIR_MAX" default:"5"`
	LoginAttemptIPWindow         time.Duration      `env:"AUTH_LOGIN_ATTEMPT_IP_WINDOW" default:"1m"`
	LoginAttemptIPMaxFailures    int                `env:"AUTH_LOGIN_ATTEMPT_IP_MAX" default:"20"`
	EmailTokenCooldown           time.Duration      `env:"AUTH_EMAIL_TOKEN_COOLDOWN" default:"60s"`
	EmailTokenHourlyMax          int                `env:"AUTH_EMAIL_TOKEN_HOURLY_MAX" default:"5"`
}

type PostgresConfig struct {
	Host     string `env:"POSTGRES_HOST" default:"postgres"`
	Port     int    `env:"POSTGRES_PORT" default:"5432"`
	User     string `env:"POSTGRES_USER" default:"root"`
	Password string `env:"POSTGRES_PASS" default:"root"`
	Base     string `env:"POSTGRES_BASE" default:"dataset"`
	// MaxConns must stay under the server's max_connections across all processes.
	MaxConns int    `env:"POSTGRES_MAX_CONNS" default:"40"`
	URL      string `env:"POSTGRES_URL" default:"host=${POSTGRES_HOST} user=${POSTGRES_USER} password=${POSTGRES_PASS} dbname=${POSTGRES_BASE} port=${POSTGRES_PORT} sslmode=disable TimeZone=${TZ}"`
}

type SMTPConfig struct {
	// MasterKey is the AES-256 key (64 hex chars) sealing email tokens in
	// sys.outbox. Changing it makes queued emails undeliverable.
	MasterKey string `env:"SMTP_MASTER_KEY" default:""`

	Enabled  bool              `env:"SMTP_ENABLED" default:"false"`
	Host     string            `env:"SMTP_HOST" default:""`
	Port     int               `env:"SMTP_PORT" default:"587"`
	User     string            `env:"SMTP_USER" default:""`
	Password string            `env:"SMTP_PASS" default:""`
	From     string            `env:"SMTP_FROM" default:""`
	ReplyTo  string            `env:"SMTP_REPLY_TO" default:""`
	Security smtpsecurity.Mode `env:"SMTP_SECURITY" default:"none"`
	// AppURL is the web app address; emails link to its /activate and
	// /reset-password pages. Empty sends the bare token only.
	AppURL string `env:"SMTP_APP_URL" default:""`
}

// TokenKey returns the key validated at load time.
func (c SMTPConfig) TokenKey() []byte {
	key, _ := aescrypt.ParseKey(c.MasterKey)
	return key
}

// StorageConfig picks where uploaded files live: a local directory, or an
// S3-compatible bucket (AWS S3, MinIO, Cloudflare R2) for more than one instance.
type StorageConfig struct {
	Driver   string `env:"STORAGE_DRIVER" default:"local"`
	LocalDir string `env:"STORAGE_LOCAL_DIR" default:"data/files"`
	// S3Endpoint is the bucket host without the bucket, e.g. https://s3.us-east-1.amazonaws.com
	// or http://minio:9000; requests use path style (endpoint/bucket/key).
	S3Endpoint  string `env:"STORAGE_S3_ENDPOINT" default:""`
	S3Region    string `env:"STORAGE_S3_REGION" default:"us-east-1"`
	S3Bucket    string `env:"STORAGE_S3_BUCKET" default:""`
	S3AccessKey string `env:"STORAGE_S3_ACCESS_KEY" default:""`
	S3SecretKey string `env:"STORAGE_S3_SECRET_KEY" default:""`
}

type Environment struct {
	DiagnosticsAddress string         `env:"DIAGNOSTICS_ADDRESS" default:"127.0.0.1:9091"`
	Timezone           *time.Location `env:"TZ" default:"America/Manaus"`
	Version            string         `env:"SYS_VERSION" default:"1.0.0"`
	Environment        string         `env:"SYS_ENVIRONMENT" default:"production"`
	Service            string         `env:"SYS_SERVICE" default:"backend"`

	Log      LogConfig
	API      APIConfig
	Auth     AuthConfig
	Postgres PostgresConfig
	SMTP     SMTPConfig
	Storage  StorageConfig
}

// Protected reports whether production rules apply.
func (e *Environment) Protected() bool {
	return isProtectedEnvironment(e.Environment)
}

func init() {
	dotenvgo.RegisterParser(smtpsecurity.Parse)

	dotenvgo.RegisterParser(func(s string) (string, error) {
		return strings.TrimSpace(s), nil
	})

	// A base64 PEM from `openssl genpkey -algorithm ed25519`, or "new" for a per-process key.
	dotenvgo.RegisterParser(func(s string) (ed25519.PrivateKey, error) {
		if s == "" {
			return nil, nil // an optional key that is not set
		}
		if s == "new" {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			return key, err
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, err
		}
		key, err := jwt.ParseEdPrivateKeyFromPEM(decoded)
		if err != nil {
			return nil, fmt.Errorf("want an Ed25519 key (openssl genpkey -algorithm ed25519 | base64 -w0): %w", err)
		}
		return key.(ed25519.PrivateKey), nil
	})
}

func loadDotEnv() error {
	paths := []string{path.Join("config", ".env")}

	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		paths = append(paths, path.Join(path.Dir(currentFile), "..", "config", ".env"))
	}

	for _, envPath := range paths {
		// Real environment variables win; containers have no file at all.
		if err := dotenvgo.LoadDotEnv(envPath, false); err == nil {
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to load environment file: %w", err)
		}
	}

	return nil
}

func Load() (*Environment, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	env := &Environment{}
	if err := dotenvgo.Load(env); err != nil {
		return nil, fmt.Errorf("failed to parse environments: %w", err)
	}

	time.Local = env.Timezone

	if err := validateEnvironment(env); err != nil {
		return nil, err
	}

	return env, nil
}

func MustLoad() *Environment {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}
