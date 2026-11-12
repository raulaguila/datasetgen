package domain

import "time"

// Profile is what a client needs to know about the logged in user.
type Profile struct {
	User        User
	IsSuperuser bool
	Permissions map[string]Action
	Preferences Preferences
	Roles       []string   // names of the enabled roles
	AvatarAt    *time.Time // when the picture was last set; nil when there is none
}

// Theme values match usr.usr_preference.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"

	DefaultLanguage = "pt-BR"

	// AvatarMaxBytes caps a profile picture; the web app sends a 256px square of a few KB.
	AvatarMaxBytes = 100 * 1024
)

// Preferences holds a user's interface choices. A user with no row gets the defaults.
type Preferences struct {
	UserID   string
	Theme    string
	Language string
}

// Notification levels match usr.usr_notification.
const (
	NotificationInfo     = "info"
	NotificationWarning  = "warning"
	NotificationCritical = "critical"
)

// Notification kinds: the codes clients translate.
const (
	NotifyNewLogin        = "new_login"        // params: ip
	NotifyAccountLocked   = "account_locked"   // params: until (RFC 3339)
	NotifySessionReused   = "session_reused"   // no params
	NotifyPasswordReset   = "password_reset"   // no params
	NotifyPasswordChanged = "password_changed" // no params
)

// Notification is a notice shown to one user.
type Notification struct {
	ID        string
	UserID    string
	Level     string
	Kind      string            // stable code a client translates; empty for free text
	Params    map[string]string // values the translated text needs, such as ip or until
	Title     string            // English fallback
	Message   string
	ReadAt    *time.Time
	CreatedAt time.Time
}
