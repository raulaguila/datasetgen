package policy

import (
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// CheckSession refuses a session that is not the token user's, was revoked or
// has expired at now. Every authenticated request runs it.
func CheckSession(sess *domain.Session, userID string, now time.Time) error {
	switch {
	case sess.UserID != userID:
		return apperror.New(apperror.CodeUnauthorized, "session mismatch")
	case sess.RevokedAt != nil:
		return apperror.New(apperror.CodeUnauthorized, "session revoked")
	case !sess.ExpiresAt.After(now):
		return apperror.New(apperror.CodeTokenExpired, "session expired")
	}
	return nil
}
