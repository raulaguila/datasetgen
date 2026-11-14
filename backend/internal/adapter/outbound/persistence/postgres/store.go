package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/attempt"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/audit"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/collection"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/credential"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/integration"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/job"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/notification"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/outbox"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/preference"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/record"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/role"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/session"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/token"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/tools"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/user"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

// Store is the root UnitOfWork / TxManager backed by GORM.
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Users() output.UserRepository             { return user.NewRepository(s.db) }
func (s *Store) Credentials() output.CredentialRepository { return credential.NewRepository(s.db) }
func (s *Store) Sessions() output.SessionRepository       { return session.NewRepository(s.db) }
func (s *Store) Tokens() output.TokenRepository           { return token.NewRepository(s.db) }
func (s *Store) Attempts() output.LoginAttemptRepository  { return attempt.NewRepository(s.db) }
func (s *Store) Roles() output.RoleRepository             { return role.NewRepository(s.db) }
func (s *Store) Outbox() output.OutboxRepository          { return outbox.NewRepository(s.db) }
func (s *Store) Preferences() output.PreferenceRepository { return preference.NewRepository(s.db) }
func (s *Store) Audit() output.AuditRepository            { return audit.NewRepository(s.db) }
func (s *Store) Collections() output.CollectionRepository { return collection.NewRepository(s.db) }
func (s *Store) Records() output.RecordRepository         { return record.NewRepository(s.db) }
func (s *Store) Jobs() output.JobRepository               { return job.NewRepository(s.db) }
func (s *Store) Tools() output.ToolsRepository            { return tools.NewRepository(s.db) }
func (s *Store) Integrations() output.IntegrationRepository {
	return integration.NewRepository(s.db)
}
func (s *Store) Notifications() output.NotificationRepository {
	return notification.NewRepository(s.db)
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, uow output.UnitOfWork) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &Store{db: tx})
	})
}

var (
	_ output.TxManager  = (*Store)(nil)
	_ output.UnitOfWork = (*Store)(nil)
)

func (s *Store) Lock(ctx context.Context, key string) error {
	return shared.WrapError(s.db.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error, "transaction lock")
}
