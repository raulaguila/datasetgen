package domain

import (
	"regexp"
	"slices"
)

// usernamePattern is GitHub's rule: lowercase letters, digits and single inner hyphens.
var usernamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedUsernames are the frontend's top-level routes and a few names kept for
// later. usr.fn_username_reserved (migration 000002) holds the same list; both change together.
var reservedUsernames = []string{
	"about", "account", "activate", "admin", "api", "api-access", "app", "assets", "c", "collections",
	"dashboard", "dataset", "docs", "explore", "f", "fonts", "forgot-password", "help", "home", "login",
	"logout", "me", "new", "notifications", "oauth", "profile", "public", "r", "records", "register",
	"reset-password", "root", "s", "search", "settings", "signin", "signup", "static", "system", "u",
	"user", "users", "www",
}

// ValidUsername reports whether name may be a username: 1-39 characters of the
// pattern above and not reserved.
func ValidUsername(name string) bool {
	return len(name) >= 1 && len(name) <= 39 && usernamePattern.MatchString(name) && !slices.Contains(reservedUsernames, name)
}
