package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/internal/core/usecase/role"
	"github.com/raulaguila/dataset/internal/core/usecase/user"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func (e *env) users() *user.Service {
	return user.New(e.store, e.store, e.clock, e.svc.Cfg.LoginAttemptEmailWindow)
}

func listed(t *testing.T, svc *user.Service, filter domain.UserFilter) map[string]bool {
	t.Helper()
	filter.Page = domain.NewPage(1, 100)
	users, total, err := svc.List(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if int(total) != len(users) {
		t.Fatalf("total %d, got %d", total, len(users))
	}
	found := map[string]bool{}
	for _, u := range users {
		found[u.Email] = true
	}
	return found
}

func TestListFiltersAndHidesProtectedUsers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.users()
	mark := "m" + uuid.NewString()[:8]
	addr, pending := mark+"-a@acme.io", mark+"-b@acme.io"
	res := e.register(t, addr)
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: res.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	e.register(t, pending)
	boss := mark + "-c@acme.io"
	res = e.register(t, boss)
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: res.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	if err := admin.New(e.store, e.clock).Promote(ctx, boss); err != nil {
		t.Fatal(err)
	}

	found := listed(t, svc, domain.UserFilter{Search: strings.ToUpper(mark)})
	if !found[addr] || !found[pending] || found[boss] || len(found) != 2 {
		t.Fatalf("listed %v", found)
	}
	found = listed(t, svc, domain.UserFilter{Search: mark, Status: domain.UserStatusPending})
	if !found[pending] || len(found) != 1 {
		t.Fatalf("listed %v", found)
	}
	if found := listed(t, svc, domain.UserFilter{Search: "%"}); len(found) != 0 {
		t.Fatalf("a wildcard matched: %v", found)
	}

	// Counts sizes the same list: the protected boss stays out.
	counts, err := svc.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for status, got := range map[string]int64{"": counts.Total, domain.UserStatusPending: counts.Pending, domain.UserStatusLocked: counts.Locked} {
		if _, want, err := svc.List(ctx, domain.UserFilter{Page: domain.NewPage(1, 1), Status: status}); err != nil || got != want {
			t.Fatalf("count %q = %d, list total %d (%v)", status, got, want, err)
		}
	}

	_, _, err = svc.List(ctx, domain.UserFilter{Page: domain.NewPage(1, 10), Status: "expired"})
	wantCode(t, err, apperror.CodeInvalidInput)
	_, _, err = svc.List(ctx, domain.UserFilter{Page: domain.NewPage(1, 10), Search: "a\x00b"})
	wantCode(t, err, apperror.CodeInvalidInput)
}

func TestGetShowsRolesAndRefusesProtectedUsers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.users()
	addr := e.activeUser(t)

	got, err := svc.Get(ctx, e.userID(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != addr || len(got.Roles) != 1 || got.Roles[0].Name != "user" {
		t.Fatalf("%+v", got)
	}

	_, err = svc.Get(ctx, "nope")
	wantCode(t, err, apperror.CodeInvalidInput)
	_, err = svc.Get(ctx, uuid.NewString())
	wantCode(t, err, apperror.CodeNotFound)

	if err := admin.New(e.store, e.clock).Promote(ctx, addr); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Get(ctx, e.userID(t, addr))
	wantCode(t, err, apperror.CodeForbidden)
}

func TestDisableAndEnableThroughTheAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.users()
	addr := e.activeUser(t)
	id := e.userID(t, addr)
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		t.Fatal(err)
	}

	actor := e.userID(t, e.activeUser(t))
	if err := svc.DisableMany(ctx, actor, []string{id}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DisableMany(ctx, actor, []string{id}); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusDisabled {
		t.Fatalf("status %s", got)
	}
	_, err = e.svc.Authenticate(ctx, pair.AccessToken)
	wantCode(t, err, apperror.CodeUnauthorized)

	if err := svc.EnableMany(ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
	_, err = e.svc.Authenticate(ctx, pair.AccessToken)
	wantCode(t, err, apperror.CodeUnauthorized) // a new login is required
	if _, err := e.login(addr, strongPassword); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnableMany(ctx, []string{id}); err != nil { // nothing to lift: still active
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
}

func TestNobodyDisablesThemselves(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	id := e.userID(t, addr)
	wantCode(t, e.users().DisableMany(ctx, id, []string{id}), apperror.CodeForbidden)
	wantCode(t, e.users().DisableMany(ctx, id, []string{strings.ToUpper(id)}), apperror.CodeForbidden)
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
}

func TestEnablingAnUnactivatedAccountKeepsItUnactivated(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := email()
	first := e.register(t, addr)
	id := e.userID(t, addr)
	if err := e.users().DisableMany(ctx, e.userID(t, e.activeUser(t)), []string{id}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword}), apperror.CodeConflict)

	if err := e.users().EnableMany(ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusPending {
		t.Fatalf("status %s", got)
	}
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: first.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedUsersChangeOnlyThroughTheOperator(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.users()
	operator := admin.New(e.store, e.clock)
	addr := e.activeUser(t)
	id := e.userID(t, addr)
	if err := operator.Promote(ctx, addr); err != nil {
		t.Fatal(err)
	}

	actor := e.userID(t, e.activeUser(t))
	wantCode(t, svc.DisableMany(ctx, actor, []string{id}), apperror.CodeForbidden)
	wantCode(t, svc.EnableMany(ctx, []string{id}), apperror.CodeForbidden)
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}

	if err := operator.Disable(ctx, addr); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.EnableMany(ctx, []string{id}), apperror.CodeForbidden)
	if err := operator.Enable(ctx, addr); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusActive {
		t.Fatalf("status %s", got)
	}
}

// A user whose role holds "*" is as protected as a member of is_superuser.
func TestSentinelGrantProtectsItsMembers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.users()
	roleID := uuid.NewString()
	if err := testDB.Exec(`INSERT INTO usr.usr_role (id, name) VALUES (?, ?)`, roleID, "t"+roleID[:8]).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = testDB.Exec(`DELETE FROM usr.usr_user_role WHERE role_id = ?`, roleID).Error
		_ = testDB.Exec(`DELETE FROM usr.usr_role WHERE id = ?`, roleID).Error
	})
	if err := testDB.Exec(`INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, '*', 1)`, roleID).Error; err != nil {
		t.Fatal(err)
	}
	addr := e.activeUser(t)
	if err := e.store.Roles().Assign(ctx, e.userID(t, addr), roleID); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Get(ctx, e.userID(t, addr))
	wantCode(t, err, apperror.CodeForbidden)
	if found := listed(t, svc, domain.UserFilter{Search: addr}); len(found) != 0 {
		t.Fatalf("listed %v", found)
	}

	// Role membership cannot reach them either.
	roles := role.New(e.store, e.store, rbac.NewRepository(testDB))
	plain, err := roles.Create(ctx, "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = testDB.Exec(`DELETE FROM usr.usr_user_role WHERE role_id = ?`, plain.ID).Error
		_ = testDB.Exec(`DELETE FROM usr.usr_role WHERE id = ?`, plain.ID).Error
	})
	wantCode(t, roles.AddMember(ctx, plain.ID, e.userID(t, addr)), apperror.CodeForbidden)
	if err := e.store.Roles().Assign(ctx, e.userID(t, addr), plain.ID); err != nil {
		t.Fatal(err)
	}
	members, total, err := roles.Members(ctx, plain.ID, domain.UserFilter{Page: domain.NewPage(1, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(members) != 0 {
		t.Fatalf("members %d %v", total, members)
	}
	wantCode(t, roles.RemoveMember(ctx, plain.ID, e.userID(t, addr)), apperror.CodeForbidden)
}

func TestUnlockLetsALockedUserBackIn(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	addr := e.activeUser(t)
	id := e.userID(t, addr)
	for range 3 { // the account lock
		_, _ = e.login(addr, "wrong password")
	}
	_, err := e.login(addr, strongPassword)
	wantCode(t, err, apperror.CodeInvalidCredentials)

	details, err := e.users().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if a := details.Access; a == nil || a.LockedUntil == nil || a.RecentFailures < 3 || details.Status != domain.UserStatusLocked {
		t.Fatalf("access before unlock: %+v %s", details.Access, details.Status)
	}

	if err := e.users().UnlockMany(ctx, []string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.login(addr, strongPassword); err != nil {
		t.Fatalf("login after unlock: %v", err)
	}
	details, err = e.users().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if a := details.Access; a.LockedUntil != nil || a.RecentFailures != 0 || a.ActiveSessions != 1 || a.LastLoginAt == nil {
		t.Fatalf("access after unlock: %+v", a)
	}
}

func TestUnlockKeepsAPendingUserPending(t *testing.T) {
	e := newEnv(t)
	addr := "pending-" + uuid.NewString()[:8] + "@example.test"
	if _, err := e.svc.Register(context.Background(), input.RegisterCommand{Name: "Pending", Email: addr}); err != nil {
		t.Fatal(err)
	}
	if err := e.users().UnlockMany(context.Background(), []string{e.userID(t, addr)}); err != nil {
		t.Fatal(err)
	}
	if got := e.userStatus(t, addr); got != domain.UserStatusPending {
		t.Fatalf("status %s", got)
	}
}
