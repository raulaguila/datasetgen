package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestProfileHasLivePermissionsAndDefaults(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	id := e.userID(t, addr)

	profile, err := e.account().Profile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if profile.IsSuperuser || profile.User.Email != addr {
		t.Fatalf("got %+v", profile)
	}
	if profile.Preferences.Theme != domain.ThemeSystem || profile.Preferences.Language != domain.DefaultLanguage {
		t.Fatalf("defaults: %+v", profile.Preferences)
	}

	if err := admin.New(e.store, e.clock).Promote(ctx, addr); err != nil {
		t.Fatal(err)
	}
	if profile, err = e.account().Profile(ctx, id); err != nil || !profile.IsSuperuser {
		t.Fatalf("promotion not seen: %+v %v", profile, err)
	}
}

func TestPreferencesSaveAndValidate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.userID(t, e.activeUser(t))
	svc := e.account()

	_, err := svc.UpdatePreferences(ctx, domain.Preferences{UserID: id, Theme: "neon", Language: "pt-BR"})
	wantCode(t, err, apperror.CodeInvalidInput)
	_, err = svc.UpdatePreferences(ctx, domain.Preferences{UserID: id, Theme: "dark", Language: "portuguese"})
	wantCode(t, err, apperror.CodeInvalidInput)

	for _, want := range []domain.Preferences{
		{UserID: id, Theme: domain.ThemeDark, Language: "en"},
		{UserID: id, Theme: domain.ThemeLight, Language: "en-US"},
	} {
		if _, err := svc.UpdatePreferences(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := svc.Preferences(ctx, id); err != nil || *got != want {
			t.Fatalf("got %+v, %v", got, err)
		}
	}
}

func TestNotificationsReportSecurityEvents(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	id := e.userID(t, addr)

	first := e.loginAt(t, addr, "203.0.113.1")
	e.loginAt(t, addr, "203.0.113.1")
	if got := e.titles(t, id); len(got) != 0 {
		t.Fatalf("first and repeated client: %v", got)
	}
	e.loginAt(t, addr, "198.51.100.7")
	if got := e.titles(t, id); len(got) != 1 || got[0] != "New login" {
		t.Fatalf("new client: %v", got)
	}

	if _, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: first.RefreshToken}); err != nil {
		t.Fatal(err)
	}
	e.clock.now = e.clock.now.Add(time.Minute)
	if _, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: first.RefreshToken}); err == nil {
		t.Fatal("replay accepted")
	}
	if got := e.titles(t, id); len(got) != 2 || got[1] != "Session ended" {
		t.Fatalf("reuse: %v", got)
	}

	for range e.svc.Cfg.MaxFailedLoginAttempts {
		_, _ = e.login(addr, "wrong password")
	}
	if got := e.titles(t, id); len(got) != 3 || got[0] != "Account locked" {
		t.Fatalf("lock: %v", got)
	}
	items, _, err := e.account().Notifications(ctx, id, domain.NewPage(1, 1))
	if err != nil || items[0].Kind != domain.NotifyAccountLocked || items[0].Params["until"] == "" {
		t.Fatalf("lock kind: %+v %v", items, err)
	}

	reset, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ConfirmPasswordReset(ctx, input.ConfirmPasswordResetCommand{Token: reset.ResetToken, Password: strongPassword + "2"}); err != nil {
		t.Fatal(err)
	}
	if got := e.titles(t, id); len(got) != 4 || got[2] != "Password changed" {
		t.Fatalf("reset: %v", got)
	}
}

func TestNotificationsStayWithTheirOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.account()
	id, strangerID := e.userID(t, e.activeUser(t)), e.userID(t, e.activeUser(t))
	for range 3 {
		n := &domain.Notification{UserID: id, Level: domain.NotificationInfo, Title: "t", Message: "m"}
		if err := e.store.Notifications().Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := svc.Notifications(ctx, id, domain.NewPage(2, 2))
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("page 2: %d of %d, %v", len(items), total, err)
	}

	wantCode(t, svc.MarkRead(ctx, strangerID, []string{items[0].ID}), apperror.CodeNotFound)
	wantCode(t, svc.MarkRead(ctx, id, []string{}), apperror.CodeInvalidInput)
	for range 2 { // idempotent
		if err := svc.MarkRead(ctx, id, []string{items[0].ID}); err != nil {
			t.Fatal(err)
		}
	}
	unread := func(user string, want int64) {
		t.Helper()
		if n, err := svc.CountUnread(ctx, user); err != nil || n != want {
			t.Fatalf("unread %d, want %d, %v", n, want, err)
		}
	}
	unread(id, 2)
	unread(strangerID, 0)

	// A list with one foreign id changes nothing.
	all, _, err := svc.Notifications(ctx, id, domain.NewPage(1, 10))
	if err != nil {
		t.Fatal(err)
	}
	stranger := &domain.Notification{UserID: strangerID, Level: domain.NotificationInfo, Title: "t", Message: "m"}
	if err := e.store.Notifications().Create(ctx, stranger); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.MarkRead(ctx, id, []string{all[0].ID, stranger.ID}), apperror.CodeNotFound)
	unread(id, 2)

	// nil marks every one of the user's, and only theirs.
	if err := svc.MarkRead(ctx, id, nil); err != nil {
		t.Fatal(err)
	}
	unread(id, 0)
	unread(strangerID, 1)
}

func TestNewClientNoticeSurvivesSessionCleanup(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	id := e.userID(t, addr)

	e.loginAt(t, addr, "203.0.113.1")
	if err := testDB.Exec("DELETE FROM usr.usr_session WHERE user_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	e.loginAt(t, addr, "198.51.100.7")
	if got := e.titles(t, id); len(got) != 1 || got[0] != "New login" {
		t.Fatalf("no sessions left on the account: %v", got)
	}
}

func TestLoginLimitSaysHowLongIsLeft(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.MaxFailedLoginAttempts = 100 // keep the account lock out of the way
	addr := e.activeUser(t)
	for range e.svc.Cfg.LoginAttemptPairMaxFailures {
		_, err := e.loginFrom(addr, "wrong password", "203.0.113.50")
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	e.clock.now = e.clock.now.Add(10 * time.Minute)
	_, err := e.loginFrom(addr, strongPassword, "203.0.113.50")
	wantCode(t, err, apperror.CodeTooManyRequests)
	var appErr *apperror.Error
	_ = errors.As(err, &appErr)
	// The clock keeps nanoseconds and the database microseconds, hence the range.
	if left, _ := appErr.Details[apperror.DetailRetryAfter].(int); left < 295 || left > 300 {
		t.Fatalf("about 5 of the 15 minutes are left: %+v", appErr)
	}
}

func TestAvatarIsStoredByContentAndRemoved(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.userID(t, e.activeUser(t))
	svc := e.account()

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if err := svc.SetAvatar(ctx, id, png); err != nil {
		t.Fatal(err)
	}
	profile, err := svc.Profile(ctx, id)
	if err != nil || profile.AvatarAt == nil {
		t.Fatalf("avatar_at %v, %v", profile, err)
	}
	if image, contentType, err := svc.Avatar(ctx, id); err != nil || contentType != "image/png" || len(image) != len(png) {
		t.Fatalf("avatar %s %d %v", contentType, len(image), err)
	}
	// An SVG can carry script, and a picture past the cap is refused, whatever the client claims.
	wantCode(t, svc.SetAvatar(ctx, id, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)), apperror.CodeInvalidInput)
	wantCode(t, svc.SetAvatar(ctx, id, append(png, make([]byte, domain.AvatarMaxBytes)...)), apperror.CodeInvalidInput)

	if err := svc.RemoveAvatar(ctx, id); err != nil {
		t.Fatal(err)
	}
	if profile, err = svc.Profile(ctx, id); err != nil || profile.AvatarAt != nil {
		t.Fatalf("after remove: %v %v", profile.AvatarAt, err)
	}
	_, _, err = svc.Avatar(ctx, id)
	wantCode(t, err, apperror.CodeNotFound)
}
