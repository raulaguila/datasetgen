package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// Pause immediately before the session transaction: a completed reset must
// make the verified hash stale. A second test checks the lock inside the tx.
type beforeTx struct {
	output.TxManager
	hook func()
}

func (tx *beforeTx) WithinTx(ctx context.Context, f func(context.Context, output.UnitOfWork) error) error {
	if hook := tx.hook; hook != nil {
		tx.hook = nil
		hook()
	}
	return tx.TxManager.WithinTx(ctx, f)
}

func TestAuditResetMustInvalidateInFlightLogin(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	reset, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	// reserveAttempt is the first transaction; the session is the second.
	tx := &beforeTx{TxManager: e.store}
	tx.hook = func() {
		tx.hook = func() {
			if err := e.svc.ConfirmPasswordReset(ctx, input.ConfirmPasswordResetCommand{Token: reset.ResetToken, Password: "the replacement password"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	e.svc.Tx = tx
	pair, err := e.login(addr, strongPassword)
	if err == nil {
		if _, err = e.svc.Authenticate(ctx, pair.AccessToken); err == nil {
			t.Fatal("old password created a live session after reset")
		}
	}
}

func TestLoginLosingToALockStaysAPasswordFailure(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	id := e.userID(t, addr)
	// reserveAttempt is the first transaction; the session is the second.
	tx := &beforeTx{TxManager: e.store}
	tx.hook = func() {
		tx.hook = func() {
			if _, err := e.store.Credentials().RecordFailure(context.Background(), id, 1, e.clock.now.Add(time.Hour), e.clock.now); err != nil {
				t.Fatal(err)
			}
		}
	}
	e.svc.Tx = tx
	_, err := e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)
	if got := e.lastFailure(t, addr); got != domain.FailureBadPassword {
		t.Fatalf("failure kind %q", got)
	}
}

func TestAuditInitialSessionMustRespectMaxLifetime(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	e.svc.Cfg.SessionMaxLifetime = time.Second
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.now = e.clock.now.Add(2 * time.Second)
	if _, err = e.svc.Authenticate(context.Background(), pair.AccessToken); err == nil {
		t.Fatal("initial session accepted after AUTH_SESSION_MAX_LIFETIME")
	}
}

func TestAuditConcurrentEmailRequestsMustRespectCap(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	e.svc.Cfg.EmailTokenCooldown = time.Minute
	e.svc.Cfg.EmailTokenHourlyMax = 1
	start := make(chan struct{})
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			<-start
			_, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
			errs <- err
		}()
	}
	close(start)
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	n, err := e.store.Tokens().CountIssuedSince(ctx, e.userID(t, addr), domain.TokenPurposePasswordReset, e.clock.now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("hourly cap=1, tokens=%d", n)
	}
}

func TestAuditExpiredOutboxClaimMustNotOverwriteNewClaim(t *testing.T) {
	e := newEnv(t)
	e.register(t, email())
	ctx := context.Background()
	out := e.store.Outbox()
	now := time.Now().Add(time.Second)
	first, err := out.ClaimPending(ctx, 1, now, time.Second)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim %v %v", first, err)
	}
	second, err := out.ClaimPending(ctx, 1, now.Add(2*time.Second), time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("second claim %v %v", second, err)
	}
	if err := out.MarkSent(ctx, first[0].ID, first[0].ClaimID, now.Add(3*time.Second)); err == nil {
		t.Fatal("stale worker marked a newer claim sent")
	}
	if err := out.MarkSent(ctx, second[0].ID, second[0].ClaimID, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := out.MarkFailed(ctx, first[0].ID, first[0].ClaimID, 1, now.Add(4*time.Second), nil, "stale worker", false); err == nil {
		t.Fatal("stale worker changed a sent message")
	}
	var status string
	if err := testDB.Raw("SELECT status FROM sys.outbox WHERE id = ?", first[0].ID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "sent" {
		t.Fatalf("stale claim overwrote sent status: %s", status)
	}
}

// The credential lock must cover insertion, not just the hash comparison.
type insertionTx struct {
	output.TxManager
	check func()
}

func (tx insertionTx) WithinTx(ctx context.Context, f func(context.Context, output.UnitOfWork) error) error {
	return tx.TxManager.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error { return f(ctx, insertionUOW{uow, tx.check}) })
}

type insertionUOW struct {
	output.UnitOfWork
	check func()
}

func (u insertionUOW) Sessions() output.SessionRepository {
	return insertionSessions{u.UnitOfWork.Sessions(), u.check}
}

type insertionSessions struct {
	output.SessionRepository
	check func()
}

func (s insertionSessions) Create(ctx context.Context, row *domain.Session) error {
	s.check()
	return s.SessionRepository.Create(ctx, row)
}

func TestLoginKeepsCredentialLockedUntilSessionInsert(t *testing.T) {
	e := newEnv(t)
	addr := e.activeUser(t)
	ctx := context.Background()
	id := e.userID(t, addr)
	reset, err := e.svc.RequestPasswordReset(ctx, input.RequestPasswordResetCommand{Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	e.svc.Tx = insertionTx{e.store, func() {
		checked = true
		err := testDB.Transaction(func(tx *gorm.DB) error {
			return tx.Exec("SELECT user_id FROM usr.usr_credential WHERE user_id = ? FOR UPDATE NOWAIT", id).Error
		})
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "55P03" {
			t.Fatalf("credential not locked during session insertion: %v", err)
		}
	}}
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("session bypassed transaction")
	}
	if err := e.svc.ConfirmPasswordReset(ctx, input.ConfirmPasswordResetCommand{Token: reset.ResetToken, Password: "the replacement password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, pair.AccessToken); err == nil {
		t.Fatal("reset did not revoke completed login")
	}
}
