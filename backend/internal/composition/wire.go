package composition

import (
	"maps"
	"time"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/adapter/outbound/emailpayload"
	"github.com/raulaguila/dataset/internal/adapter/outbound/files"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/job"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/outbox"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/ratelimit"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	smtpmail "github.com/raulaguila/dataset/internal/adapter/outbound/smtp"
	"github.com/raulaguila/dataset/internal/adapter/outbound/tokens"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/internal/core/usecase/account"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	auditsvc "github.com/raulaguila/dataset/internal/core/usecase/audit"
	"github.com/raulaguila/dataset/internal/core/usecase/auth"
	"github.com/raulaguila/dataset/internal/core/usecase/authorize"
	"github.com/raulaguila/dataset/internal/core/usecase/collection"
	"github.com/raulaguila/dataset/internal/core/usecase/health"
	"github.com/raulaguila/dataset/internal/core/usecase/jobs"
	"github.com/raulaguila/dataset/internal/core/usecase/mailer"
	"github.com/raulaguila/dataset/internal/core/usecase/product"
	"github.com/raulaguila/dataset/internal/core/usecase/record"
	rolesvc "github.com/raulaguila/dataset/internal/core/usecase/role"
	usersvc "github.com/raulaguila/dataset/internal/core/usecase/user"
	"github.com/raulaguila/dataset/internal/infra"
	"github.com/raulaguila/dataset/pkg/logger"
)

// NewAdmin wires the operator use cases of cmd/admin.
func NewAdmin(res *infra.Resources) input.AdminService {
	return admin.New(postgres.NewStore(res.PostgresDB), output.RealClock{})
}

// NewRateLimit counts HTTP requests in Postgres, shared by every backend instance.
func NewRateLimit(res *infra.Resources) middleware.Hits {
	return ratelimit.NewRepository(res.PostgresDB).Hit
}

// NewMailer wires the worker of cmd/mailer.
func NewMailer(cfg *config.Environment, res *infra.Resources, log *logger.Logger) *mailer.Worker {
	return &mailer.Worker{
		Outbox: outbox.NewRepository(res.PostgresDB),
		Mail:   smtpmail.NewSender(cfg.SMTP),
		Log:    log,
		Clock:  output.RealClock{},
		Codec:  emailpayload.New(cfg.SMTP.TokenKey()),
		AppURL: cfg.SMTP.AppURL,
	}
}

// NewFiles picks the file store named by STORAGE_DRIVER, validated at load time.
func NewFiles(cfg config.StorageConfig) output.FileStore {
	if cfg.Driver == "s3" {
		return &files.S3{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey}
	}
	return &files.Local{Dir: cfg.LocalDir}
}

// NewJobs wires the background job worker that cmd/backend runs. Each module
// adds its handlers here.
func NewJobs(res *infra.Resources, log *logger.Logger) *jobs.Worker {
	handlers := map[string]policy.JobHandler{}
	maps.Copy(handlers, product.Jobs(log))
	return &jobs.Worker{
		Repo:     job.NewRepository(res.PostgresDB),
		Handlers: handlers,
		Log:      log,
		Clock:    output.RealClock{},
	}
}

// NewApplication wires concrete adapters into use cases / inbound ports.
func NewApplication(cfg *config.Environment, res *infra.Resources, log *logger.Logger) *input.Application {
	pgHealth := postgres.NewHealthChecker(res.PostgresDB)

	store := postgres.NewStore(res.PostgresDB)
	permReader := rbac.NewRepository(res.PostgresDB)
	issuer := &tokens.Issuer{
		AccessKey:       cfg.Auth.AccessPrivateKey,
		RefreshKey:      cfg.Auth.RefreshPrivateKey,
		AccessPrevious:  cfg.Auth.AccessPreviousKey,
		RefreshPrevious: cfg.Auth.RefreshPreviousKey,
		AccessExpire:    cfg.Auth.AccessExpiration,
		RefreshExpire:   cfg.Auth.RefreshExpiration,
	}

	authSvc := auth.New(store, store, issuer, output.RealClock{}, auth.Config{
		SessionMaxLifetime:           cfg.Auth.SessionMaxLifetime,
		RegistrationTokenTTL:         cfg.Auth.RegistrationTokenTTL,
		PasswordResetTokenTTL:        cfg.Auth.PasswordResetTokenTTL,
		MaxFailedLoginAttempts:       cfg.Auth.MaxFailedLoginAttempts,
		AccountLockDuration:          cfg.Auth.AccountLockDuration,
		AllowManualTokenDelivery:     cfg.Auth.AllowManualTokenDelivery,
		AllowRegistration:            cfg.Auth.AllowRegistration,
		PasswordResetMinDuration:     300 * time.Millisecond, // above the slowest path: one transaction and two inserts
		LoginAttemptEmailWindow:      cfg.Auth.LoginAttemptEmailWindow,
		LoginAttemptEmailMaxFailures: cfg.Auth.LoginAttemptEmailMaxFailures,
		LoginAttemptPairMaxFailures:  cfg.Auth.LoginAttemptPairMaxFailures,
		LoginAttemptIPWindow:         cfg.Auth.LoginAttemptIPWindow,
		LoginAttemptIPMaxFailures:    cfg.Auth.LoginAttemptIPMaxFailures,
		EmailTokenCooldown:           cfg.Auth.EmailTokenCooldown,
		EmailTokenHourlyMax:          cfg.Auth.EmailTokenHourlyMax,
	}, emailpayload.New(cfg.SMTP.TokenKey()))

	authSvc.Log = log

	authorizer := authorize.New(permReader, output.RealClock{})
	audit := auditsvc.New(store.Audit())

	return &input.Application{
		Health:      health.New(pgHealth),
		Authorizer:  authorizer,
		Resources:   authorizer,
		Roles:       rolesvc.New(store, store, permReader),
		Users:       usersvc.New(store, store, output.RealClock{}, cfg.Auth.LoginAttemptEmailWindow),
		Auth:        authSvc,
		Account:     account.New(store, permReader, output.RealClock{}, NewFiles(cfg.Storage)),
		Products:    product.New(store.Products(), store),
		Records:     record.New(store.Records(), store.Collections(), store),
		Collections: collection.New(store.Collections(), store.Records(), store, authSvc, audit),
		Audit:       audit,
	}
}
