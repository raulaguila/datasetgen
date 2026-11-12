package input

import (
	"context"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// AccountService is what a logged in user does with their own account. Every
// method acts on userID only, which callers take from the verified token.
type AccountService interface {
	Profile(ctx context.Context, userID string) (*domain.Profile, error)
	Rename(ctx context.Context, userID, name string) (*domain.Profile, error)

	// SetAvatar stores a PNG, JPEG or WebP picture of at most domain.AvatarMaxBytes.
	SetAvatar(ctx context.Context, userID string, image []byte) error
	RemoveAvatar(ctx context.Context, userID string) error
	// Avatar returns the picture and its content type.
	Avatar(ctx context.Context, userID string) ([]byte, string, error)

	Sessions(ctx context.Context, userID string) ([]domain.Session, error)
	RevokeSession(ctx context.Context, userID, sessionID string) error
	RevokeOtherSessions(ctx context.Context, userID, currentSessionID string) error

	Preferences(ctx context.Context, userID string) (*domain.Preferences, error)
	UpdatePreferences(ctx context.Context, p domain.Preferences) (*domain.Preferences, error)

	Notifications(ctx context.Context, userID string, page domain.Page) ([]domain.Notification, int64, error)
	CountUnread(ctx context.Context, userID string) (int64, error)
	// MarkRead marks the listed notifications as read, or all of them when ids is nil.
	MarkRead(ctx context.Context, userID string, ids []string) error
}
