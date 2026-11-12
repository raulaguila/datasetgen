package integration_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/audit"
	"github.com/raulaguila/dataset/internal/core/usecase/role"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

func TestInviteActivatesAndShowsInTheAuditLog(t *testing.T) {
	e := newEnv(t)
	actor := e.userID(t, e.activeUser(t))
	ctx := reqctx.WithActor(context.Background(), actor)
	addr := email()

	res, err := e.svc.Invite(ctx, input.RegisterCommand{Name: "Bea", Email: addr})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Invite(ctx, input.RegisterCommand{Name: "Bea", Email: addr})
	wantCode(t, err, apperror.CodeAlreadyExists)
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: res.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}

	records, total, err := audit.New(e.store.Audit()).List(ctx, domain.AuditFilter{Page: domain.NewPage(1, 10), Action: "user.", UserID: res.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || records[0].Action != "user.invite" || records[0].ActorID != actor || records[0].TargetLabel != addr {
		t.Fatalf("audit %d %+v", total, records)
	}
}

func TestRevokeSessionsEndsAnotherUsersLogins(t *testing.T) {
	e := newEnv(t)
	me, other := e.activeUser(t), e.activeUser(t)
	meID, otherID := e.userID(t, me), e.userID(t, other)
	pair := e.loginAt(t, other, "10.0.0.1")

	wantCode(t, e.users().RevokeSessions(context.Background(), meID, meID), apperror.CodeForbidden)
	if err := e.users().RevokeSessions(context.Background(), meID, otherID); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Refresh(context.Background(), input.RefreshCommand{RefreshToken: pair.RefreshToken})
	if err == nil {
		t.Fatal("refresh still works after the sessions were revoked")
	}
}

func TestUserListFiltersByRoleAndSorts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	roles := role.New(e.store, e.store, rbac.NewRepository(testDB))
	r, err := roles.Create(ctx, "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec(`DELETE FROM usr.usr_role WHERE id = ?`, r.ID).Error })
	var emails []string
	for range 3 {
		addr := e.activeUser(t)
		if err := roles.AddMember(ctx, r.ID, e.userID(t, addr)); err != nil {
			t.Fatal(err)
		}
		emails = append(emails, addr)
	}
	e.activeUser(t) // outside the role

	users, total, err := e.users().List(ctx, domain.UserFilter{Page: domain.NewPage(1, 10), RoleID: r.ID, Sort: "email", Order: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range users {
		got = append(got, u.Email)
	}
	slices.Sort(emails)
	slices.Reverse(emails)
	if total != 3 || !slices.Equal(got, emails) {
		t.Fatalf("got %d %v, want %v", total, got, emails)
	}
	_, _, err = e.users().List(ctx, domain.UserFilter{Sort: "password_hash"})
	wantCode(t, err, apperror.CodeInvalidInput)
}

func TestRoleListSortsByCreation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	roles := role.New(e.store, e.store, rbac.NewRepository(testDB))
	prefix := "s" + uuid.NewString()[:6]
	var ids []string
	for _, suffix := range []string{"b", "a"} { // created b first, a second
		r, err := roles.Create(ctx, prefix+suffix)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
		t.Cleanup(func() { _ = testDB.Exec(`DELETE FROM usr.usr_role WHERE id = ?`, r.ID).Error })
	}
	list, _, err := roles.List(ctx, domain.RoleFilter{Page: domain.NewPage(1, 10), Search: prefix, Sort: "created_at", Order: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != ids[1] || list[1].ID != ids[0] {
		t.Fatalf("order %+v", list)
	}
}

func TestReinviteReplacesTheActivationToken(t *testing.T) {
	e := newEnv(t)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	first, err := e.svc.Invite(ctx, input.RegisterCommand{Name: "Bea", Email: email()})
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Cfg.EmailTokenCooldown = time.Minute
	_, err = e.svc.Reinvite(ctx, first.UserID)
	wantCode(t, err, apperror.CodeTooManyRequests)

	e.clock.now = e.clock.now.Add(2 * time.Minute)
	second, err := e.svc.Reinvite(ctx, first.UserID)
	if err != nil || second.ActivationToken == "" || second.ActivationToken == first.ActivationToken {
		t.Fatalf("reinvite: %+v %v", second, err)
	}
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword}); err == nil {
		t.Fatal("the replaced token still activates")
	}
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: second.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	e.clock.now = e.clock.now.Add(2 * time.Minute)
	_, err = e.svc.Reinvite(ctx, first.UserID)
	wantCode(t, err, apperror.CodeConflict)
}

func TestActivationLinkHandsOverATokenWithoutEmail(t *testing.T) {
	e := newEnv(t)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	e.svc.Cfg.AllowManualTokenDelivery = false // the link comes back even when tokens normally go by email only
	e.svc.Cfg.EmailTokenCooldown = time.Minute
	first, err := e.svc.Invite(ctx, input.RegisterCommand{Name: "Cid", Email: email()})
	if err != nil || first.ActivationToken != "" {
		t.Fatalf("invite: %+v %v", first, err)
	}
	outbox := func() (n int64) {
		if err := testDB.Raw("SELECT count(*) FROM sys.outbox").Row().Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := outbox()

	e.clock.now = e.clock.now.Add(2 * time.Minute) // past the email cooldown
	link, err := e.svc.ActivationLink(ctx, first.UserID)
	if err != nil || link.ActivationToken == "" || !link.ExpiresAt.Equal(e.clock.now.Add(e.svc.Cfg.RegistrationTokenTTL)) {
		t.Fatalf("link: %+v %v", link, err)
	}
	if outbox() != before {
		t.Fatal("a copied link must not send an email")
	}
	access, err := e.store.Users().Access(ctx, first.UserID, e.clock.now, e.clock.now)
	if err != nil || access.ActivationExpiresAt == nil || access.ActivationExpiresAt.Sub(link.ExpiresAt).Abs() > time.Millisecond {
		t.Fatalf("access: %+v %v", access, err)
	}
	var audited int64
	if err := testDB.Raw("SELECT count(*) FROM usr.usr_audit_log WHERE action = 'user.activation_link' AND target_id = ?", first.UserID).Row().Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("audited %d %v", audited, err)
	}
	_, err = e.svc.ActivationLink(ctx, first.UserID)
	wantCode(t, err, apperror.CodeTooManyRequests) // shares the email throttle

	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: link.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	if access, err = e.store.Users().Access(ctx, first.UserID, e.clock.now, e.clock.now); err != nil || access.ActivationExpiresAt != nil {
		t.Fatalf("after activation: %+v %v", access, err)
	}
	e.clock.now = e.clock.now.Add(2 * time.Minute)
	_, err = e.svc.ActivationLink(ctx, first.UserID)
	wantCode(t, err, apperror.CodeConflict)
}

func TestAuditCountStopsAtTheCap(t *testing.T) {
	e := newEnv(t)
	target := uuid.NewString()
	if err := testDB.Exec(`INSERT INTO usr.usr_audit_log (action, target_id) SELECT 'cap.test', ? FROM generate_series(1, 10050)`, target).Error; err != nil {
		t.Fatal(err)
	}
	records, total, err := audit.New(e.store.Audit()).List(context.Background(), domain.AuditFilter{Page: domain.NewPage(1, 10), UserID: target})
	if err != nil || total != 10001 || len(records) != 10 {
		t.Fatalf("total %d, %d records, %v", total, len(records), err)
	}
}
