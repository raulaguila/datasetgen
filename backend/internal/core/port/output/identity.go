package output

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// UserRepository persists usr.usr_user.
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*domain.User, error)
	// Access reads the sign-in state of a user: failures since failuresSince, sessions live at now.
	Access(ctx context.Context, id string, failuresSince, now time.Time) (*domain.UserAccess, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	// LockStatus reads the status and holds off any change to it until the
	// transaction ends. Inside a transaction only.
	LockStatus(ctx context.Context, id string) (string, error)
	Create(ctx context.Context, user *domain.User) error
	UpdateName(ctx context.Context, id, name string) error
	// UpdateUsername fails with AlreadyExists when another account holds username.
	UpdateUsername(ctx context.Context, id, username string) error
	// AvatarAt reads when the user last set a picture; nil when there is none.
	AvatarAt(ctx context.Context, id string) (*time.Time, error)
	SetAvatarAt(ctx context.Context, id string, at *time.Time) error
	// UpdateStatus leaves a disabled account as it is: a login or activation already
	// running when the account was disabled must not undo that.
	UpdateStatus(ctx context.Context, id, status string) error
	// Enable lifts a disable: the account goes back to active, or to pending if it
	// never set a password. Any other status stays.
	Enable(ctx context.Context, id string) error
	// List pages through the users without a protected role.
	List(ctx context.Context, filter domain.UserFilter) ([]domain.User, int64, error)
}

// CredentialRepository persists usr.usr_credential.
type CredentialRepository interface {
	FindByUserID(ctx context.Context, userID string) (*domain.Credential, error)
	// Create inserts the empty credential of a new user.
	Create(ctx context.Context, userID string) error
	// ClearLock lifts the account lock and its failure count, keeping the password.
	ClearLock(ctx context.Context, userID string) error
	// SetPassword stores a new hash and clears the lockout.
	SetPassword(ctx context.Context, userID, hash string) error
	// RecordFailure atomically counts a bad password, restarting the count after
	// a lapsed lock, and locks until lockUntil once maxAttempts is reached.
	RecordFailure(ctx context.Context, userID string, maxAttempts int, lockUntil, now time.Time) (lockedUntil *time.Time, err error)
	// RecordSuccess clears the counters only if the verified hash is still current
	// and no lock started meanwhile. False means the login must be refused.
	RecordSuccess(ctx context.Context, userID, verifiedHash string, now time.Time) (bool, error)

	// MFA is the user's second step, enabled or being set up; NOT_FOUND when none.
	MFA(ctx context.Context, userID string) (*domain.MFA, error)
	// SetupMFA stores a new secret to confirm, unless one is enabled (false then).
	SetupMFA(ctx context.Context, userID, sealedSecret string) (bool, error)
	// EnableMFA turns the stored secret on, taking step as used, and replaces
	// the recovery codes; false when there is none to enable.
	EnableMFA(ctx context.Context, userID string, step int64, codeHashes [][]byte, now time.Time) (bool, error)
	// UseMFAStep takes a step's code once: false for that step or an earlier one.
	UseMFAStep(ctx context.Context, userID string, step int64) (bool, error)
	// UseRecoveryCode spends an unused recovery code; false when it is not one.
	UseRecoveryCode(ctx context.Context, userID string, hash []byte, now time.Time) (bool, error)
	RecoveryCodesLeft(ctx context.Context, userID string) (int, error)
	// DeleteMFA removes the second step and its recovery codes.
	DeleteMFA(ctx context.Context, userID string) error
}

// SessionRepository persists usr.usr_session.
type SessionRepository interface {
	FindByID(ctx context.Context, id string) (*domain.Session, error)
	// FindLive fills only ID, UserID, ExpiresAt and RevokedAt.
	FindLive(ctx context.Context, id string) (*domain.Session, error)
	Create(ctx context.Context, session *domain.Session) error
	// Rotate swaps the refresh hash only if currentHash is still the live one.
	// False means another request already rotated or revoked the session.
	Rotate(ctx context.Context, id string, currentHash, newHash []byte, expiresAt, now time.Time) (bool, error)
	// Revoke is idempotent: an unknown or already revoked session is not an error.
	Revoke(ctx context.Context, id string, now time.Time, reason string) error
	RevokeAllForUser(ctx context.Context, userID string, now time.Time, reason string) error
	// ListActive returns the live sessions of a user, most recently used first.
	ListActive(ctx context.Context, userID string, now time.Time, limit int) ([]domain.Session, error)
	// RevokeOwned revokes a live session of this user. False means there is none.
	RevokeOwned(ctx context.Context, id, userID string, now time.Time, reason string) (bool, error)
	// RevokeOthers revokes every session of the user except keepID.
	RevokeOthers(ctx context.Context, userID, keepID string, now time.Time, reason string) error
	// KnownClient reports whether this client already logged in to the account.
	// An account with no session yet knows every client: its first login is no news.
	KnownClient(ctx context.Context, userID, ip string) (bool, error)
}

// TokenRepository persists usr.usr_token.
type TokenRepository interface {
	Create(ctx context.Context, token *domain.EmailToken) error
	FindActiveByHash(ctx context.Context, purpose string, hash []byte) (*domain.EmailToken, error)
	MarkUsed(ctx context.Context, id string, usedAt time.Time) error
	InvalidateActive(ctx context.Context, userID, purpose string) error
	// IssuedSince reports whether a live token of this purpose was created at or after since.
	IssuedSince(ctx context.Context, userID, purpose string, since time.Time) (bool, error)
	// CountIssuedSince counts tokens of this purpose created at or after since, spent ones too.
	CountIssuedSince(ctx context.Context, userID, purpose string, since time.Time) (int, error)
}

// FailureCounts are the recent failures against an account, against it from
// one client, and from that client overall.
type FailureCounts struct {
	Email int `gorm:"column:email"`
	Pair  int `gorm:"column:pair"`
	IP    int `gorm:"column:ip"`
	// The oldest failure of each count: when it leaves the window, a full limit opens.
	EmailOldest *time.Time `gorm:"column:email_oldest"`
	PairOldest  *time.Time `gorm:"column:pair_oldest"`
	IPOldest    *time.Time `gorm:"column:ip_oldest"`
}

// LoginAttemptRepository appends login audits.
type LoginAttemptRepository interface {
	Create(ctx context.Context, attempt *domain.LoginAttempt) error
	// CountRecentFailures counts the failures that count against a login. The
	// pair uses the email window; an IPv6 address counts its whole /64.
	CountRecentFailures(ctx context.Context, email string, ip *string, emailSince, ipSince time.Time) (FailureCounts, error)
	// ClearFailures forgets what counts against an account since a moment, older
	// history stays. A password reset proves the owner, who must get back in.
	ClearFailures(ctx context.Context, email string, since time.Time) error
	// Settle replaces the outcome of a reserved attempt.
	Settle(ctx context.Context, id string, success bool, failureKind *string) error
	// Release drops a reserved attempt that tested no password.
	Release(ctx context.Context, id string) error
}

// RoleRepository resolves roles and assigns them to users.
type RoleRepository interface {
	// DefaultRoleIDs lists the enabled default roles a new account gets.
	DefaultRoleIDs(ctx context.Context) ([]string, error)
	FindIDByName(ctx context.Context, name string) (string, error)
	Assign(ctx context.Context, userID, roleID string) error
	// HasMembers reports whether any user holds the role.
	HasMembers(ctx context.Context, roleID string) (bool, error)
	// ForUser returns every role of a user, enabled or not, without member counts.
	ForUser(ctx context.Context, userID string) ([]domain.Role, error)
}
