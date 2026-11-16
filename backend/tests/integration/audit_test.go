package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/ratelimit"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

func TestChangesLeaveAnAuditTrail(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	actor := e.userID(t, e.activeUser(t))
	ctx := reqctx.WithRequestID(reqctx.WithActor(context.Background(), actor), "req-1")

	name := "audit-" + uuid.NewString()[:8]
	created, err := collections.Create(ctx, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// A refused change leaves nothing behind.
	if _, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "", Schema: json.RawMessage(`{"type":"object"}`)}); err == nil {
		t.Fatal("a collection without a name was made")
	}

	var rows []struct {
		ActorID   string
		RequestID string
		Action    string
	}
	if err := testDB.Raw(`SELECT actor_id::text AS actor_id, request_id, action FROM usr.usr_audit_log WHERE actor_id = ? AND request_id = 'req-1'`, actor).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "collection.create" || rows[0].ActorID != actor || rows[0].RequestID != "req-1" {
		t.Fatalf("audit %+v for %s", rows, created.ID)
	}
}

func TestAppCannotRewriteTheAuditLog(t *testing.T) {
	for _, sql := range []string{
		"DELETE FROM usr.usr_audit_log WHERE false",
		"UPDATE usr.usr_audit_log SET action = action WHERE false",
	} {
		err := testDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL ROLE dataset_app").Error; err != nil {
				return err
			}
			return tx.Exec(sql).Error
		})
		if err == nil {
			t.Fatalf("app role ran %s", sql)
		}
	}
}

func TestRateLimitCountsAcrossCalls(t *testing.T) {
	hits := ratelimit.NewRepository(testDB)
	key := "t:" + uuid.NewString()
	for want := int64(1); want <= 3; want++ {
		got, err := hits.Hit(context.Background(), key)
		if err != nil || got != want {
			t.Fatalf("hit %d: %d %v", want, got, err)
		}
	}
	if got, _ := hits.Hit(context.Background(), key+"x"); got != 1 {
		t.Fatalf("other key shares the count: %d", got)
	}
}
