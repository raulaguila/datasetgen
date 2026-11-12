package integration_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestLoginGivesSameAnswerForUnusableAccounts(t *testing.T) {
	e := newEnv(t)
	pending := email()
	e.register(t, pending)
	for _, addr := range []string{pending, email()} {
		_, err := e.login(addr, "whatever-password")
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
}

func TestConcurrentFailuresAreAllCounted(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	u, err := e.store.Users().FindByEmail(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := e.store.Credentials().RecordFailure(ctx, u.ID, 100, e.clock.now.Add(time.Hour), e.clock.now); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	cred, err := e.store.Credentials().FindByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cred.FailedLoginAttempts != 10 {
		t.Fatalf("counted %d of 10", cred.FailedLoginAttempts)
	}
}

func TestBlockedRetriesDoNotExtendTheBlock(t *testing.T) {
	e := newEnv(t)
	addr := email()
	for range 5 {
		_, err := e.login(addr, "wrong-password")
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	_, err := e.login(addr, "wrong-password") // blocked; used to count as a new failure
	wantCode(t, err, apperror.CodeTooManyRequests)

	if err := testDB.Exec(`UPDATE usr.usr_login_attempt SET created_at = now() - INTERVAL '16 minutes'
		WHERE email = ? AND failure_kind = 'user_not_found'`, addr).Error; err != nil {
		t.Fatal(err)
	}
	_, err = e.login(addr, "wrong-password")
	wantCode(t, err, apperror.CodeInvalidCredentials)
}

func TestLockLapsesAndCounterRestarts(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)

	for range 2 {
		_, err := e.login(addr, "wrong-password")
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	_, err := e.login(addr, "wrong-password") // the lock itself must look like any other failure
	wantCode(t, err, apperror.CodeInvalidCredentials)
	if got := e.userStatus(t, addr); got != domain.UserStatusLocked {
		t.Fatalf("status %s", got)
	}
	_, err = e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)

	e.clock.now = e.clock.now.Add(16 * time.Minute)
	_, err = e.login(addr, "wrong-password") // a typo after the lock must not relock at once
	wantCode(t, err, apperror.CodeInvalidCredentials)
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
	if _, err := e.login(addr, strongPassword); err != nil {
		t.Fatal(err)
	}
}

func TestLoginTimeoutIsNoPasswordFailure(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.svc.Login(ctx, input.LoginCommand{Email: addr, Password: "wrong-password"})
	if err == nil || apperror.IsCode(err, apperror.CodeInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
	u, err := e.store.Users().FindByEmail(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := e.store.Credentials().FindByUserID(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cred.FailedLoginAttempts != 0 {
		t.Fatalf("timeout counted as %d failures", cred.FailedLoginAttempts)
	}
}

func TestOneClientCannotLockTheOwnerOut(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.MaxFailedLoginAttempts = 50
	e.svc.Cfg.LoginAttemptEmailMaxFailures = 50
	addr := e.activeUser(t)
	attacker := fmt.Sprintf("203.0.113.%d", time.Now().UnixNano()%250+1)

	for range 5 {
		_, err := e.loginFrom(addr, "wrong-password", attacker)
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	_, err := e.loginFrom(addr, "wrong-password", attacker)
	wantCode(t, err, apperror.CodeTooManyRequests)

	if _, err := e.loginFrom(addr, strongPassword, "198.51.100.7"); err != nil {
		t.Fatalf("owner refused: %v", err)
	}
}

func TestEmailLimitStillStopsManyClients(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	e.svc.Cfg.MaxFailedLoginAttempts = 100
	for i := range 5 {
		_, err := e.loginFrom(addr, "wrong-password", fmt.Sprintf("198.18.0.%d", i+1))
		wantCode(t, err, apperror.CodeInvalidCredentials)
	}
	_, err := e.loginFrom(addr, strongPassword, "198.18.0.200")
	wantCode(t, err, apperror.CodeTooManyRequests)
}

func TestIPv6ClientCountsAsItsPrefix(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Minute)
	prefix := fmt.Sprintf("2001:db8:%x:%x", time.Now().UnixNano()&0xffff, time.Now().UnixNano()>>16&0xffff)
	for _, suffix := range []string{"::1", "::2", "::ffff"} {
		ip := prefix + suffix
		kind := domain.FailureBadPassword
		if err := e.store.Attempts().Create(ctx, &domain.LoginAttempt{Email: email(), IPAddress: &ip, FailureKind: &kind}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := countByIP(ctx, prefix+"::abcd", since)
	if err != nil || n != 3 {
		t.Fatalf("counted %d %v", n, err)
	}
}

// Parallel requests must not pass a limit because the others are still hashing.
func TestParallelLoginsCannotOutrunTheLimit(t *testing.T) {
	e := newEnv(t)
	e.svc.Cfg.MaxFailedLoginAttempts = 1000
	e.svc.Cfg.LoginAttemptEmailMaxFailures = 1000
	addr := e.activeUser(t)

	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[string]int{}
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.loginFrom(addr, "wrong-password", "192.0.2.200")
			var appErr *apperror.Error
			if !errors.As(err, &appErr) {
				t.Errorf("got %v", err)
				return
			}
			mu.Lock()
			codes[string(appErr.Code)]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[string(apperror.CodeInvalidCredentials)] != 5 || codes[string(apperror.CodeTooManyRequests)] != 25 {
		t.Fatalf("codes %v", codes)
	}
}

func TestSuccessfulLoginsDoNotUseUpTheLimit(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	for range 8 {
		if _, err := e.loginFrom(addr, strongPassword, "192.0.2.201"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTimedOutLoginDoesNotCount(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ip := "192.0.2.202"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancelled past the reservation, before the password check: no timer to race.
	store := e.svc.UOW
	e.svc.UOW = &slowLogin{store, cancel}
	_, err := e.svc.Login(ctx, input.LoginCommand{Email: addr, Password: "wrong-password", IPAddress: &ip})
	e.svc.UOW = store
	if err == nil {
		t.Fatal("login succeeded")
	}
	n, err := countByIP(context.Background(), ip, time.Now().Add(-time.Minute))
	if err != nil || n != 0 {
		t.Fatalf("counted %d %v", n, err)
	}
}
