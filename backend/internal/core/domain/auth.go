package domain

import "time"

const (
	TokenPurposeActivation    = "activation"
	TokenPurposePasswordReset = "password_reset"

	OutboxPurposeActivation    = "activation"
	OutboxPurposePasswordReset = "password_reset"

	RevokeUserLogout      = "user_logout"
	RevokeTokenReuse      = "token_reuse"
	RevokePasswordChanged = "password_changed"
	RevokeExpired         = "expired"
	RevokeAccountDisabled = "account_disabled"
	RevokeAdmin           = "admin_revoked"

	FailureBadPassword      = "bad_password"
	FailureUserNotFound     = "user_not_found"
	FailureAccountLocked    = "account_locked"
	FailureAccountDisabled  = "account_disabled"
	FailureAccountPending   = "account_pending"
	FailureRateLimitedIP    = "rate_limited_ip"
	FailureRateLimitedEmail = "rate_limited_email"
)

// EmailToken is usr.usr_token (activation | password_reset).
type EmailToken struct {
	ID        string
	UserID    string
	Purpose   string
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// LoginAttempt is an append-only audit row.
type LoginAttempt struct {
	ID          string
	CreatedAt   time.Time
	Email       string
	IPAddress   *string
	UserAgent   *string
	Success     bool
	FailureKind *string
}

// TokenPair is issued on login/refresh.
type TokenPair struct {
	UserID       string
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // access TTL seconds
	TokenType    string
}
