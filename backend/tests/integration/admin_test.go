package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestPromoteGivesAdminRoleOnlyToActiveUsers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := admin.New(e.store, e.clock)

	pending := email()
	e.register(t, pending)
	wantCode(t, svc.Promote(ctx, pending), apperror.CodeConflict)
	wantCode(t, svc.Promote(ctx, email()), apperror.CodeNotFound)

	addr := e.activeUser(t)
	if err := svc.Promote(ctx, "  "+strings.ToUpper(addr)); err != nil {
		t.Fatal(err)
	}
	var n int64
	err := testDB.Table("usr.usr_user_role ur").
		Joins("JOIN usr.usr_role r ON r.id = ur.role_id JOIN usr.usr_user u ON u.id = ur.user_id").
		Where("u.email = ? AND r.name = 'admin'", addr).Count(&n).Error
	if err != nil || n != 1 {
		t.Fatalf("admin roles: %d %v", n, err)
	}
}

func TestDisableEndsSessionsAtOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := e.svc.Tokens.ParseAccess(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	if err := admin.New(e.store, e.clock).Disable(ctx, addr); err != nil {
		t.Fatal(err)
	}
	sess, err := e.store.Sessions().FindByID(ctx, claims.SessionID)
	if err != nil || sess.RevokedAt == nil {
		t.Fatalf("session alive: %+v %v", sess, err)
	}
	_, err = e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)
}

func TestBootstrapCreatesOneWorkingAdmin(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := testDB.Exec(`DELETE FROM usr.usr_user_role WHERE role_id = (SELECT id FROM usr.usr_role WHERE name = 'admin')`).Error; err != nil {
		t.Fatal(err)
	}
	svc := admin.New(e.store, e.clock)

	taken := e.activeUser(t)
	_, err := svc.Bootstrap(ctx, input.BootstrapCommand{Name: "Root", Email: taken, Password: strongPassword})
	wantCode(t, err, apperror.CodeConflict)
	_, err = svc.Bootstrap(ctx, input.BootstrapCommand{Name: "Root", Email: email(), Password: "short"})
	wantCode(t, err, apperror.CodeInvalidInput)

	const runs = 4
	addrs := make([]string, runs)
	created := make([]bool, runs)
	var wg sync.WaitGroup
	for i := range runs {
		addrs[i] = email()
		wg.Go(func() {
			var err error
			created[i], err = svc.Bootstrap(ctx, input.BootstrapCommand{Name: "Root", Email: addrs[i], Password: strongPassword})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	winner := ""
	for i, ok := range created {
		if ok && winner != "" {
			t.Fatal("two runs each created an admin")
		}
		if ok {
			winner = addrs[i]
		}
	}
	if winner == "" {
		t.Fatal("no run created the admin")
	}
	pair, err := e.login(winner, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := e.account().Profile(ctx, pair.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.IsSuperuser || profile.User.Status != domain.UserStatusActive {
		t.Fatalf("got %+v", profile)
	}
}

// slowLogin runs hook once, while a login sits between its status check and its
// password check: the same place as the slow Argon2 run.
type slowLogin struct {
	output.UnitOfWork
	hook func()
}

func (u *slowLogin) Credentials() output.CredentialRepository {
	return slowCredentials{u.UnitOfWork.Credentials(), u}
}

type slowCredentials struct {
	output.CredentialRepository
	login *slowLogin
}

func (c slowCredentials) FindByUserID(ctx context.Context, id string) (*domain.Credential, error) {
	cred, err := c.CredentialRepository.FindByUserID(ctx, id)
	if hook := c.login.hook; hook != nil {
		c.login.hook = nil
		hook()
	}
	return cred, err
}

// disableDuringLogin makes the next login on e meet Disable halfway.
func (e *env) disableDuringLogin(t *testing.T, addr string) (restore func()) {
	t.Helper()
	store := e.svc.UOW
	e.svc.UOW = &slowLogin{store, func() {
		if err := admin.New(e.store, e.clock).Disable(context.Background(), addr); err != nil {
			t.Fatal(err)
		}
	}}
	return func() { e.svc.UOW = store }
}

// lastFailure returns the failure kind stored for the latest login of addr.
func (e *env) lastFailure(t *testing.T, addr string) string {
	t.Helper()
	var kind string
	err := testDB.Table("usr.usr_login_attempt").Select("failure_kind").
		Where("email = ?", addr).Order("created_at DESC, id DESC").Limit(1).Scan(&kind).Error
	if err != nil {
		t.Fatal(err)
	}
	return kind
}

func TestLoginInFlightLosesToDisable(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	restore := e.disableDuringLogin(t, addr)
	_, err := e.login(addr, strongPassword)
	restore()
	wantCode(t, err, apperror.CodeInvalidCredentials)

	var live int64
	err = testDB.Table("usr.usr_session").Where("user_id = ? AND revoked_at IS NULL", e.userID(t, addr)).Count(&live).Error
	if err != nil || live != 0 {
		t.Fatalf("live sessions: %d %v", live, err)
	}
	if got := e.lastFailure(t, addr); got != domain.FailureAccountDisabled {
		t.Fatalf("failure kind %q", got)
	}
}

func TestLockDoesNotUndoDisable(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	for range 2 {
		_, err := e.login(addr, "wrong password")
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	restore := e.disableDuringLogin(t, addr)
	_, err := e.login(addr, "wrong password") // the third failure reaches the lock
	restore()
	wantCode(t, err, apperror.CodeInvalidCredentials)
	if got := e.userStatus(t, addr); got != domain.UserStatusDisabled {
		t.Fatalf("status %s", got)
	}

	e.clock.now = e.clock.now.Add(16 * time.Minute)
	_, err = e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)
}

func TestDisabledSignupStaysOut(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := email()
	first := e.register(t, addr)
	if err := admin.New(e.store, e.clock).Disable(ctx, addr); err != nil {
		t.Fatal(err)
	}

	e.clock.now = e.clock.now.Add(25 * time.Hour)
	err := e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword})
	wantCode(t, err, apperror.CodeTokenExpired)
	if again := e.register(t, addr); again.ActivationToken != "" {
		t.Fatal("a disabled account got a new token")
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusDisabled {
		t.Fatalf("status %s", got)
	}
}

func TestDisableTwiceStaysQuiet(t *testing.T) {
	e := newEnv(t)
	svc := admin.New(e.store, e.clock)
	addr := e.activeUser(t)
	for range 2 {
		if err := svc.Disable(context.Background(), addr); err != nil {
			t.Fatal(err)
		}
	}
}
