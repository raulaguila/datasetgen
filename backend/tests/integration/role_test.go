package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/usecase/admin"
	"github.com/raulaguila/dataset/internal/core/usecase/authorize"
	"github.com/raulaguila/dataset/internal/core/usecase/role"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func findRole(t *testing.T, svc *role.Service, name string) domain.Role {
	t.Helper()
	ctx := context.Background()
	id, err := svc.UOW.Roles().FindIDByName(ctx, name) // the repository also sees protected roles
	if err != nil {
		t.Fatalf("role %s: %v", name, err)
	}
	r, err := svc.UOW.Roles().FindByID(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	return *r
}

func TestAdminRoleCannotBeChangedThroughTheAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	before := findRole(t, svc, "admin")

	addr := e.activeUser(t)
	if err := admin.New(e.store, e.clock).Promote(ctx, addr); err != nil {
		t.Fatal(err)
	}
	userID := e.userID(t, addr)

	_, err := svc.SetPermissions(ctx, before.ID, map[string]domain.Action{domain.ResourceRecords: domain.ActionRead})
	wantCode(t, err, apperror.CodeForbidden)
	_, err = svc.SetPermissions(ctx, before.ID, map[string]domain.Action{})
	wantCode(t, err, apperror.CodeForbidden)
	name, enabled := "renamed", false
	_, err = svc.Update(ctx, before.ID, domain.RolePatch{Name: &name})
	wantCode(t, err, apperror.CodeForbidden)
	_, err = svc.Update(ctx, before.ID, domain.RolePatch{Enabled: &enabled})
	wantCode(t, err, apperror.CodeForbidden)
	wantCode(t, svc.DeleteMany(ctx, []string{before.ID}), apperror.CodeForbidden)
	wantCode(t, svc.AddMember(ctx, before.ID, e.userID(t, e.activeUser(t))), apperror.CodeForbidden)
	wantCode(t, svc.RemoveMember(ctx, before.ID, userID), apperror.CodeForbidden)

	after := findRole(t, svc, "admin")
	if !after.IsSuperuser || !after.Enabled || len(after.Permissions) != 1 || after.Permissions[domain.ResourceAll] != before.Permissions[domain.ResourceAll] {
		t.Fatalf("admin changed: %+v", after)
	}
	eff, err := rbac.NewRepository(testDB).LoadEffective(ctx, userID)
	if err != nil || !eff.IsSuperuser {
		t.Fatalf("the member must stay admin: %+v %v", eff, err)
	}
}

func TestListHidesProtectedRoles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	sentinel := newRole(t, svc)
	if err := testDB.Exec(`
		INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, '*', 1)
	`, sentinel.ID).Error; err != nil {
		t.Fatal(err)
	}

	roles, _, err := svc.List(ctx, domain.RoleFilter{Page: domain.NewPage(1, 100)})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, r := range roles {
		found[r.Name] = true
	}
	if !found["user"] || found["admin"] || found[sentinel.Name] {
		t.Fatalf("listed %v", found)
	}
}

func TestSentinelGrantMakesARoleProtected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created, err := svc.Create(ctx, "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_role WHERE id = ?", created.ID).Error })

	_, err = svc.SetPermissions(ctx, created.ID, map[string]domain.Action{domain.ResourceAll: domain.ActionRead})
	wantCode(t, err, apperror.CodeInvalidInput) // the API never grants "*"

	if err := testDB.Exec(`
		INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, '*', 1)
	`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.AddMember(ctx, created.ID, e.userID(t, e.activeUser(t))), apperror.CodeForbidden)
}

func TestRolePermissionsApplyAtOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	perms := rbac.NewRepository(testDB)
	svc := role.New(e.store, e.store, perms)
	authz := authorize.New(perms, e.clock)
	addr := e.activeUser(t)
	userID := e.userID(t, addr)
	p, err := e.svc.Authenticate(ctx, e.loginAt(t, addr, "203.0.113.9").AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	created, err := svc.Create(ctx, "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_role WHERE id = ?", created.ID).Error })
	_, err = svc.Create(ctx, created.Name)
	wantCode(t, err, apperror.CodeAlreadyExists)
	_, err = svc.Create(ctx, "Not Valid")
	wantCode(t, err, apperror.CodeInvalidInput)

	wantCode(t, authz.Authorize(ctx, *p, domain.ResourceUsers, domain.ActionRead), apperror.CodeForbidden)

	updated, err := svc.SetPermissions(ctx, created.ID, map[string]domain.Action{
		domain.ResourceUsers: domain.ActionRead | domain.ActionUpdate,
		domain.ResourceRoles: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Permissions) != 1 || updated.Permissions[domain.ResourceUsers] != domain.ActionRead|domain.ActionUpdate {
		t.Fatalf("permissions %v", updated.Permissions)
	}
	if err := svc.AddMember(ctx, created.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err := authz.Authorize(ctx, *p, domain.ResourceUsers, domain.ActionUpdate); err != nil {
		t.Fatal(err)
	}
	wantCode(t, authz.Authorize(ctx, *p, domain.ResourceUsers, domain.ActionDelete), apperror.CodeForbidden)

	members, total, err := svc.Members(ctx, created.ID, domain.UserFilter{Page: domain.NewPage(1, 20)})
	if err != nil || total != 1 || len(members) != 1 || members[0].ID != userID {
		t.Fatalf("members %+v %d %v", members, total, err)
	}

	updated, err = svc.SetPermissions(ctx, created.ID, map[string]domain.Action{domain.ResourceRecords: domain.ActionUpdate})
	if err != nil || updated.Permissions[domain.ResourceCollections] != domain.ActionRead {
		t.Fatalf("records must bring collections read: %+v %v", updated, err)
	}

	wantCode(t, svc.DeleteMany(ctx, []string{created.ID}), apperror.CodeResourceInUse)
	updated, err = svc.SetPermissions(ctx, created.ID, map[string]domain.Action{})
	if err != nil || len(updated.Permissions) != 0 {
		t.Fatalf("permissions %+v %v", updated, err)
	}
	wantCode(t, authz.Authorize(ctx, *p, domain.ResourceUsers, domain.ActionRead), apperror.CodeForbidden)

	_, err = svc.SetPermissions(ctx, created.ID, map[string]domain.Action{"unknown": domain.ActionRead})
	wantCode(t, err, apperror.CodeInvalidInput)
	wantCode(t, svc.AddMember(ctx, created.ID, uuid.NewString()), apperror.CodeNotFound)

	if err := svc.RemoveMember(ctx, created.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMany(ctx, []string{created.ID}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.DeleteMany(ctx, []string{created.ID}), apperror.CodeNotFound)
}

func TestDefaultRoleCanChangeButNotLeave(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	def := findRole(t, svc, "user")

	wantCode(t, svc.DeleteMany(ctx, []string{def.ID}), apperror.CodeConflict)
	off := false
	_, err := svc.Update(ctx, def.ID, domain.RolePatch{Enabled: &off})
	wantCode(t, err, apperror.CodeConflict)
	if _, err := svc.SetPermissions(ctx, def.ID, map[string]domain.Action{domain.ResourceRecords: domain.ActionRead}); err != nil {
		t.Fatal(err)
	}
	// Back to the seeded grant: every account may create collections.
	if _, err := svc.SetPermissions(ctx, def.ID, map[string]domain.Action{domain.ResourceCollections: domain.ActionCreate}); err != nil {
		t.Fatal(err)
	}
}

func newRole(t *testing.T, svc *role.Service) *domain.Role {
	t.Helper()
	created, err := svc.Create(context.Background(), "t"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_role WHERE id = ?", created.ID).Error })
	return created
}

func TestFailedSaveKeepsThePreviousPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created := newRole(t, svc)
	if _, err := svc.SetPermissions(ctx, created.ID, map[string]domain.Action{"users": domain.ActionRead}); err != nil {
		t.Fatal(err)
	}

	// A constraint for this role only makes the second grant fail at the database, after the old rows are gone.
	if err := testDB.Exec(`ALTER TABLE usr.usr_role_permission ADD CONSTRAINT chk_test_fail
		CHECK (role_id <> '` + created.ID + `' OR resource <> 'roles') NOT VALID`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Exec("ALTER TABLE usr.usr_role_permission DROP CONSTRAINT chk_test_fail").Error })
	_, err := svc.SetPermissions(ctx, created.ID, map[string]domain.Action{
		domain.ResourceRecords: domain.ActionRead,
		domain.ResourceRoles:   domain.ActionRead,
	})
	if err == nil {
		t.Fatal("the failing grant must fail the save")
	}
	got := findRole(t, svc, created.Name)
	if len(got.Permissions) != 1 || got.Permissions["users"] != domain.ActionRead {
		t.Fatalf("permissions changed: %v", got.Permissions)
	}
}

func TestGrantsStayWithinTheResourceActions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created := newRole(t, svc)
	for _, bad := range []map[string]domain.Action{
		{domain.ResourceAudit: domain.ActionCreate},
		{domain.ResourceUsers: domain.ActionRead | domain.ActionDelete},
		{domain.ResourceAudit: domain.Action(-1)},
	} {
		_, err := svc.SetPermissions(ctx, created.ID, bad)
		wantCode(t, err, apperror.CodeInvalidInput)
	}
	if _, err := svc.SetPermissions(ctx, created.ID, map[string]domain.Action{domain.ResourceAudit: domain.ActionRead}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentSavesEndWithOneOfThem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created := newRole(t, svc)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = svc.SetPermissions(ctx, created.ID, map[string]domain.Action{
				domain.ResourceAudit: domain.ActionRead,
				domain.ResourceRoles: domain.ActionRead,
			})
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := findRole(t, svc, created.Name); len(got.Permissions) != 2 {
		t.Fatalf("permissions %v", got.Permissions)
	}
}

func TestUpdateChangesOnlyWhatItCarries(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := role.New(e.store, e.store, rbac.NewRepository(testDB))
	created := newRole(t, svc)

	off := false
	updated, err := svc.Update(ctx, created.ID, domain.RolePatch{Enabled: &off})
	if err != nil || updated.Enabled || updated.Name != created.Name {
		t.Fatalf("%+v %v", updated, err)
	}
	name := created.Name + "-x"
	updated, err = svc.Update(ctx, created.ID, domain.RolePatch{Name: &name})
	if err != nil || updated.Name != name || updated.Enabled {
		t.Fatalf("%+v %v", updated, err)
	}

	bad, taken := "Not Valid", "user"
	_, err = svc.Update(ctx, created.ID, domain.RolePatch{Name: &bad})
	wantCode(t, err, apperror.CodeInvalidInput)
	_, err = svc.Update(ctx, created.ID, domain.RolePatch{Name: &taken})
	wantCode(t, err, apperror.CodeAlreadyExists)
	_, err = svc.Update(ctx, uuid.NewString(), domain.RolePatch{Name: &name})
	wantCode(t, err, apperror.CodeNotFound)
}

func TestAccessFollowsRolesGrantsAndSuperuser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	repo := rbac.NewRepository(testDB)
	svc := role.New(e.store, e.store, repo)
	addr := e.activeUser(t)
	userID := e.userID(t, addr)
	p, err := e.svc.Authenticate(ctx, e.loginAt(t, addr, "203.0.113.9").AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	want := func(resource string, mask domain.Action) {
		t.Helper()
		sess, got, err := repo.Access(ctx, p.SessionID, resource)
		if err != nil || got != mask || sess.UserID != userID || sess.RevokedAt != nil {
			t.Fatalf("%s: got %d, want %d (%+v %v)", resource, got, mask, sess, err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := testDB.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	want("records", 0)

	a, b := newRole(t, svc), newRole(t, svc)
	t.Cleanup(func() { _ = testDB.Exec("DELETE FROM usr.usr_user_role WHERE user_id = ?", userID).Error })
	exec(`INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, 'records', 6), (?, 'records', 1), (?, 'users', 2)`, a.ID, b.ID, b.ID)
	exec(`INSERT INTO usr.usr_user_role (user_id, role_id) VALUES (?, ?), (?, ?)`, userID, a.ID, userID, b.ID)
	want("records", 7)
	want("users", 2)
	want("roles", 0)

	exec(`UPDATE usr.usr_role SET enabled = false WHERE id = ?`, b.ID)
	want("records", 6)

	exec(`INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, '*', 1)`, a.ID)
	want("roles", 2147483647)

	exec(`DELETE FROM usr.usr_role_permission WHERE role_id = ? AND resource = '*'`, a.ID)
	exec(`INSERT INTO usr.usr_user_role (user_id, role_id) SELECT ?, id FROM usr.usr_role WHERE name = 'admin'`, userID)
	want("records", 2147483647)

	if err := e.svc.Logout(ctx, p.SessionID); err != nil {
		t.Fatal(err)
	}
	if sess, _, err := repo.Access(ctx, p.SessionID, "audit"); err != nil || sess.RevokedAt == nil {
		t.Fatalf("logout not seen: %+v %v", sess, err)
	}
	_, _, err = repo.Access(ctx, "00000000-0000-0000-0000-000000000000", "audit")
	wantCode(t, err, apperror.CodeNotFound)
}
