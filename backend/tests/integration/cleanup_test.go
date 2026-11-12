package integration_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
)

func TestCleanupRemovesAbandonedSignups(t *testing.T) {
	e := newEnv(t)
	abandoned, resumed, activated, fresh := email(), email(), e.activeUser(t), email()
	for _, addr := range []string{abandoned, resumed, fresh} {
		e.register(t, addr)
	}
	err := testDB.Exec(`UPDATE usr.usr_user SET created_at = now() - INTERVAL '31 days' WHERE email IN (?)`,
		[]string{abandoned, resumed, activated}).Error
	if err != nil {
		t.Fatal(err)
	}
	err = testDB.Exec(`UPDATE usr.usr_token SET expires_at = now() - INTERVAL '1 hour'
		WHERE user_id = (SELECT id FROM usr.usr_user WHERE email = ?)`, abandoned).Error
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`SELECT sys.fn_cleanup()`).Error; err != nil {
		t.Fatal(err)
	}
	var left []string
	all := []string{abandoned, resumed, activated, fresh}
	if err := testDB.Table("usr.usr_user").Where("email IN ?", all).Pluck("email", &left).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(left)
	want := []string{resumed, activated, fresh}
	sort.Strings(want)
	if strings.Join(left, ",") != strings.Join(want, ",") {
		t.Fatalf("left %v, want %v", left, want)
	}
}

func TestCleanupRemovesOnlyOldHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old, recent := email(), email()
	for _, addr := range []string{old, recent} {
		if err := e.store.Attempts().Create(ctx, &domain.LoginAttempt{Email: addr, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.Exec(`UPDATE usr.usr_login_attempt SET created_at = now() - INTERVAL '31 days' WHERE email = ?`, old).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`SELECT sys.fn_cleanup()`).Error; err != nil {
		t.Fatal(err)
	}
	var left []string
	if err := testDB.Table("usr.usr_login_attempt").Where("email IN ?", []string{old, recent}).Pluck("email", &left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0] != recent {
		t.Fatalf("left %v", left)
	}
}
