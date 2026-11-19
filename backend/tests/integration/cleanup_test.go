package integration_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/job"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/reqctx"
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

// The worker's hourly Cleanup empties the trash past its 30 days, versions included.
func TestCleanupPurgesOldTrash(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Purga", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, data := range []string{`{"a":1}`, `{"a":2}`} {
		r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(data)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := records.Update(owner, input.UpdateRecordCommand{ID: r.ID, Data: json.RawMessage(`{"a":3}`)}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	if err := records.DeleteMany(owner, ids); err != nil {
		t.Fatal(err)
	}
	old, recent := ids[0], ids[1]
	if err := testDB.Exec(`UPDATE app.record_trash SET deleted_at = now() - INTERVAL '31 days' WHERE id = ?`, old).Error; err != nil {
		t.Fatal(err)
	}
	if err := job.NewRepository(testDB).Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	count := func(table, id string) (n int64) {
		col := "id"
		if table == "app.record_version" {
			col = "record_id"
		}
		if err := testDB.Table(table).Where(col+" = ?", id).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("app.record_trash", old) != 0 || count("app.record_version", old) != 0 {
		t.Fatal("an old trashed record or its versions survived the cleanup")
	}
	if count("app.record_trash", recent) != 1 || count("app.record_version", recent) != 1 {
		t.Fatal("the cleanup removed a recently trashed record")
	}
}
