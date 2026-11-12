package output

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// PreferenceRepository persists usr.usr_preference.
type PreferenceRepository interface {
	// Find answers not found for a user who never saved preferences.
	Find(ctx context.Context, userID string) (*domain.Preferences, error)
	Save(ctx context.Context, p *domain.Preferences) error
}

// NotificationRepository persists usr.usr_notification. Reads and writes always
// filter by owner, so a notification id alone reaches nothing.
type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	// List returns one page, newest first, and the count of all the user's notifications.
	List(ctx context.Context, userID string, page domain.Page) ([]domain.Notification, int64, error)
	CountUnread(ctx context.Context, userID string) (int64, error)
	// MarkRead marks the listed notifications, or every unread one when ids is
	// nil. Idempotent. With ids it is all or none: false means one of them is
	// not this user's, and nothing changed.
	MarkRead(ctx context.Context, userID string, ids []string, now time.Time) (bool, error)
}
