package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// Activation gives a new account every enabled default role.
func TestSeveralDefaultRolesGoToNewAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id, name := newRole(t, true)

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
	want := []string{name, "user"}
	slices.Sort(want)
	if got := names(); !slices.Equal(got, want) {
		t.Fatalf("new user roles %v", got)
	}

	if err := testDB.Exec(`UPDATE usr.usr_role SET is_default = false WHERE id = ?`, id).Error; err != nil {
		t.Fatal(err)
	}
	if got := names(); !slices.Equal(got, []string{"user"}) {
		t.Fatalf("after unmark %v", got)
	}
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
