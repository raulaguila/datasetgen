package domain

import "time"

// UserStatus codes match chk_usr_user_status.
const (
	UserStatusPending  = "pending"
	UserStatusActive   = "active"
	UserStatusLocked   = "locked"
	UserStatusDisabled = "disabled"
)

// Role groups permissions for its members.
type Role struct {
	ID          string
	Name        string
	Enabled     bool
	IsDefault   bool
	IsSuperuser bool
	Permissions map[string]Action
	CreatedAt   time.Time
	MemberCount int64 // members the API may see; filled on reads
}

// RolePatch holds the role fields to change; nil keeps the current value.
type RolePatch struct {
	Name    *string
	Enabled *bool
	// IsDefault marks a role every new account gets.
	IsDefault *bool
}

// RoleFilter selects one page of the roles the API may change.
type RoleFilter struct {
	Page   Page
	Search string
	Sort   string // a column the repository allows; checked by the service
	Order  string // asc or desc
}

// Protected reports whether the role is out of reach of the HTTP API: it holds
// full access, so only the operator commands of cmd/admin touch its members.
func (r Role) Protected() bool {
	_, all := r.Permissions[ResourceAll]
	return r.IsSuperuser || all
}

// User is the identity aggregate root (no transport tags).
type User struct {
	ID        string
	Name      string
	Email     string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserCounts sizes the user list: everyone it shows, and those waiting on an action.
type UserCounts struct {
	Total   int64
	Pending int64
	Locked  int64
}

// UserFilter narrows a user list; empty fields filter nothing.
type UserFilter struct {
	Page   Page
	Search string
	Status string
	RoleID string // only holders of this role
	Sort   string // a column the repository allows; checked by the service
	Order  string // asc or desc
}

// UserDetails is a user with the roles it holds.
type UserDetails struct {
	User
	Roles  []Role
	Access *UserAccess // only the detail screen loads it
}

// UserAccess says why a user may fail to sign in.
type UserAccess struct {
	LastLoginAt    *time.Time
	FailedLogins   int        // bad passwords toward the account lock
	LockedUntil    *time.Time // account lock; nil or past means none
	RecentFailures int        // counted failures inside the email window, from any client
	ActiveSessions int
	// ActivationExpiresAt is when the live activation link stops working; nil when there is none.
	ActivationExpiresAt *time.Time
}

// Credential is 1:1 with User.
type Credential struct {
	UserID              string
	PasswordHash        *string
	FailedLoginAttempts int
	LockedUntil         *time.Time
	LastLoginAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Session stores the current refresh jti hash and rotation state.
type Session struct {
	ID                       string
	UserID                   string
	ExpiresAt                time.Time
	RevokedAt                *time.Time
	RevocationReason         *string
	RefreshTokenHash         []byte
	PreviousRefreshTokenHash []byte
	IPAddress                *string
	UserAgent                *string
	CreatedAt                time.Time
	LastSeenAt               time.Time
}

// Principal is who an authenticated request acts as.
type Principal struct {
	UserID    string
	SessionID string
}

// EffectivePermissions is the authz snapshot of a user, loaded live on each check.
type EffectivePermissions struct {
	UserID      string
	IsSuperuser bool
	ByResource  map[string]Action // includes "*" if granted
}
