package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/usecase/role"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestSeveralDefaultRolesGoToNewAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created := newRole(t, svc)
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_user_role WHERE role_id = ?", created.ID).Error })
	yes, no := true, false

	if _, err := svc.Update(ctx, created.ID, domain.RolePatch{Enabled: &no}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Update(ctx, created.ID, domain.RolePatch{IsDefault: &yes})
	wantCode(t, err, apperror.CodeConflict) // a disabled role cannot be default
	if _, err := svc.Update(ctx, created.ID, domain.RolePatch{IsDefault: &yes, Enabled: &yes}); err != nil {
		t.Fatal(err)
	}
	if !findRole(t, svc, created.Name).IsDefault || !findRole(t, svc, "user").IsDefault {
		t.Fatal("both roles must be default")
	}
	_, err = svc.Update(ctx, created.ID, domain.RolePatch{Enabled: &no})
	wantCode(t, err, apperror.CodeConflict)
	wantCode(t, svc.DeleteMany(ctx, []string{created.ID}), apperror.CodeConflict)

	names := func() []string {
		got, err := e.store.Roles().ForUser(ctx, e.userID(t, e.activeUser(t)))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range got {
			out = append(out, r.Name)
		}
		slices.Sort(out)
		return out
	}
	want := []string{created.Name, "user"}
	slices.Sort(want)
	if got := names(); !slices.Equal(got, want) {
		t.Fatalf("new user roles %v", got)
	}

	if _, err := svc.Update(ctx, created.ID, domain.RolePatch{IsDefault: &no}); err != nil {
		t.Fatal(err)
	}
	if got := names(); !slices.Equal(got, []string{"user"}) {
		t.Fatalf("after unmark %v", got)
	}
}

func TestRoleListPagesAndSearches(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	tag := "q" + uuid.NewString()[:6]
	for range 3 {
		if _, err := svc.Create(ctx, tag+uuid.NewString()[:6]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_role WHERE name LIKE ?", tag+"%").Error })
	page, total, err := svc.List(ctx, domain.RoleFilter{Page: domain.NewPage(2, 2), Search: tag})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 1 {
		t.Fatalf("total %d, page %d", total, len(page))
	}
	_, err = svc.Get(ctx, findRole(t, svc, "admin").ID)
	wantCode(t, err, apperror.CodeForbidden)

	holder := newRole(t, svc)
	first, second := e.activeUser(t), e.activeUser(t)
	if err := svc.SetUserRoles(ctx, e.userID(t, first), []string{holder.ID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserRoles(ctx, e.userID(t, second), []string{holder.ID}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_user_role WHERE role_id = ?", holder.ID).Error })
	members, total, err := svc.Members(ctx, holder.ID, domain.UserFilter{Page: domain.NewPage(1, 10), Search: first})
	if err != nil || total != 1 || len(members) != 1 || members[0].Email != first {
		t.Fatalf("members %v %d %v", members, total, err)
	}
}

func TestRoleBatchDeleteIsAllOrNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	a, b := newRole(t, svc), newRole(t, svc)

	wantCode(t, svc.DeleteMany(ctx, []string{a.ID, findRole(t, svc, "user").ID}), apperror.CodeConflict)
	if _, err := svc.Get(ctx, a.ID); err != nil {
		t.Fatalf("a refused batch deleted a role: %v", err)
	}
	if err := svc.DeleteMany(ctx, []string{a.ID, b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Get(ctx, b.ID)
	wantCode(t, err, apperror.CodeNotFound)
}

func TestUserBatchDisableSkipsNobody(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	me, other := e.activeUser(t), e.activeUser(t)
	meID, otherID := e.userID(t, me), e.userID(t, other)

	wantCode(t, e.users().DisableMany(ctx, meID, []string{otherID, meID}), apperror.CodeForbidden)
	if e.userStatus(t, other) != domain.UserStatusActive {
		t.Fatal("a refused batch disabled a user")
	}
	if err := e.users().DisableMany(ctx, meID, []string{otherID}); err != nil {
		t.Fatal(err)
	}
	if e.userStatus(t, other) != domain.UserStatusDisabled {
		t.Fatal("not disabled")
	}
	if err := e.users().EnableMany(ctx, []string{otherID}); err != nil {
		t.Fatal(err)
	}
	if e.userStatus(t, other) != domain.UserStatusActive {
		t.Fatal("not enabled")
	}
}
