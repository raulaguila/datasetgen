package output

import "time"

// AccessClaims is the parsed access JWT (no transport tags).
type AccessClaims struct {
	UserID    string
	SessionID string
	ExpiresAt time.Time
}

// RefreshClaims is the parsed refresh JWT.
type RefreshClaims struct {
	UserID    string
	SessionID string
	JTI       string
	ExpiresAt time.Time
}

// TokenIssuer issues and parses JWTs (adapter implements with RSA keys).
type TokenIssuer interface {
	IssueAccess(userID, sessionID string, now time.Time) (string, error)
	IssueRefresh(userID, sessionID string, now time.Time) (token string, jti string, err error)
	ParseAccess(token string) (*AccessClaims, error)
	ParseRefresh(token string) (*RefreshClaims, error)
	AccessTTL() time.Duration
	RefreshTTL() time.Duration
}
