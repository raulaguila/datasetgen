package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestOldRefreshTokenRevokesSessionAtAnyAge(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	gen0, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	gen1, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen0.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	gen2, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen1.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen0.RefreshToken}) // two generations old
	wantCode(t, err, apperror.CodeUnauthorized)
	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen2.RefreshToken})
	wantCode(t, err, apperror.CodeUnauthorized)
}

func TestLockedAccountKeepsItsSession(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_, _ = e.login(addr, "wrong-password")
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusLocked {
		t.Fatalf("status %s", got)
	}
	if _, err := e.svc.Refresh(context.Background(), input.RefreshCommand{RefreshToken: pair.RefreshToken}); err != nil {
		t.Fatalf("attacker logged the owner out: %v", err)
	}
}

func TestRefreshRaceLoserKeepsSessionButReplayLaterRevokes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	gen0, err := e.login(e.activeUser(t), strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	gen1, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen0.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen0.RefreshToken}) // the other tab
	wantCode(t, err, apperror.CodeConflict)
	gen2, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen1.RefreshToken})
	if err != nil {
		t.Fatalf("race cost the session: %v", err)
	}

	e.clock.now = e.clock.now.Add(11 * time.Second)
	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen1.RefreshToken}) // replay outside the window
	wantCode(t, err, apperror.CodeUnauthorized)
	_, err = e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: gen2.RefreshToken})
	wantCode(t, err, apperror.CodeUnauthorized)
}

func TestConcurrentRefreshHasOneWinnerAtMost(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 8 {
		wg.Go(func() {
			if _, err := e.svc.Refresh(context.Background(), input.RefreshCommand{RefreshToken: pair.RefreshToken}); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if wins > 1 {
		t.Fatalf("%d refreshes won with one token", wins)
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	e := newEnv(t)
	pair, err := e.login(e.activeUser(t), strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := e.svc.Authenticate(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := e.svc.Logout(context.Background(), principal.SessionID); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.Refresh(context.Background(), input.RefreshCommand{RefreshToken: pair.RefreshToken})
	wantCode(t, err, apperror.CodeUnauthorized)
}

func TestSessionsStayWithTheirOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.account()
	addr, stranger := e.activeUser(t), e.activeUser(t)
	id, strangerID := e.userID(t, addr), e.userID(t, stranger)
	e.loginAt(t, addr, "203.0.113.1")
	e.loginAt(t, addr, "203.0.113.1")
	e.loginAt(t, addr, "203.0.113.1")

	sessions, err := svc.Sessions(ctx, id)
	if err != nil || len(sessions) != 3 {
		t.Fatalf("got %d, %v", len(sessions), err)
	}
	if ip := sessions[0].IPAddress; ip == nil || *ip != "203.0.113.1" {
		t.Fatalf("ip: %v", ip)
	}

	wantCode(t, svc.RevokeSession(ctx, strangerID, sessions[0].ID), apperror.CodeNotFound)
	wantCode(t, svc.RevokeSession(ctx, id, "not-a-uuid"), apperror.CodeInvalidInput)
	if err := svc.RevokeSession(ctx, id, sessions[0].ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.RevokeSession(ctx, id, sessions[0].ID), apperror.CodeNotFound)

	keep := sessions[1].ID
	if err := svc.RevokeOtherSessions(ctx, id, keep); err != nil {
		t.Fatal(err)
	}
	if sessions, err = svc.Sessions(ctx, id); err != nil || len(sessions) != 1 || sessions[0].ID != keep {
		t.Fatalf("after others: %+v %v", sessions, err)
	}
}

func TestRefreshCannotOutliveTheMaxLifetime(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pair, err := e.login(e.activeUser(t), strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	// 50 minutes between refreshes stays inside the one hour refresh TTL.
	refresh := func() error {
		e.clock.now = e.clock.now.Add(50 * time.Minute)
		next, err := e.svc.Refresh(ctx, input.RefreshCommand{RefreshToken: pair.RefreshToken})
		if err == nil {
			pair = next
		}
		return err
	}
	for range 2 {
		if err := refresh(); err != nil {
			t.Fatal(err)
		}
	}
	e.svc.Cfg.SessionMaxLifetime = 2 * time.Hour
	if err := refresh(); err != nil { // 2h30 would pass the limit: this one is cut to it
		t.Fatal(err)
	}
	wantCode(t, refresh(), apperror.CodeTokenExpired)
}
