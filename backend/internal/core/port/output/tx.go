package output

import (
	"context"
)

// TxManager runs fn inside a database transaction. Repositories obtained from the
// returned UnitOfWork participate in the same transaction.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error
}

// UnitOfWork exposes transactional repositories.
type UnitOfWork interface {
	// Lock serializes a named operation until the enclosing transaction ends.
	Lock(ctx context.Context, key string) error
	Users() UserRepository
	Credentials() CredentialRepository
	Sessions() SessionRepository
	Tokens() TokenRepository
	Attempts() LoginAttemptRepository
	Roles() RoleRepository
	Outbox() OutboxRepository
	Preferences() PreferenceRepository
	Notifications() NotificationRepository
	Audit() AuditRepository
	Products() ProductRepository
	Records() RecordRepository
	Collections() CollectionRepository
	Jobs() JobRepository
}

// AccountStore exposes only the repositories this application service reads or updates.
type AccountStore interface {
	Users() UserRepository
	Sessions() SessionRepository
	Preferences() PreferenceRepository
	Notifications() NotificationRepository
	Roles() RoleRepository
}

// IdentityStore exposes only the repositories the user and role services read or update.
type IdentityStore interface {
	Users() UserRepository
	Roles() RoleRepository
}
