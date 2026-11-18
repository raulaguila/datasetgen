package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

func TestImportOrderedUpsertsAndUniqueValues(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "Import regressions", Schema: json.RawMessage(`{"type":"object","properties":{"sku":{"type":"integer","x-unique":true},"tag":{"type":"string","x-unique":true},"qty":{"type":"integer"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	send := func(cmd input.ImportRecordsCommand, lines ...string) *input.ImportResult {
		t.Helper()
		cmd.CollectionID = c.ID
		for _, line := range lines {
			cmd.Records = append(cmd.Records, json.RawMessage(line))
		}
		r, err := records.Import(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	bySKU := input.ImportRecordsCommand{UpsertKey: "sku"}
	if r := send(bySKU, `{"sku":1,"qty":1}`, `{"sku":2}`); r.Created != 2 {
		t.Fatal(r)
	}
	changes := []string{`{"sku":1,"qty":2}`, `{"sku":1.0,"tag":"red"}`, `{"sku":2,"tag":"red"}`}
	verify := func(qty, version int) {
		t.Helper()
		var row struct{ Qty, Version int }
		if err := testDB.Raw("SELECT (data->>'qty')::int AS qty,version FROM app.record WHERE collection_id=? AND data @> '{\"sku\":1}'", c.ID).Take(&row).Error; err != nil || row.Qty != qty || row.Version != version {
			t.Fatalf("row=%+v err=%v", row, err)
		}
	}
	for _, cmd := range []input.ImportRecordsCommand{{UpsertKey: "sku", DryRun: true}, bySKU} {
		r := send(cmd, changes...)
		if r.Invalid != 1 || r.Errors[0].Index != 2 || r.Created+r.Updated != 0 {
			t.Fatal(r)
		}
		verify(1, 1)
	}
	skip := input.ImportRecordsCommand{UpsertKey: "sku", SkipInvalid: true}
	if r := send(skip, changes...); r.Updated != 2 || r.Invalid != 1 || r.Errors[0].Index != 2 {
		t.Fatal(r)
	}
	verify(2, 3)
	var previous int
	if err := testDB.Raw(`SELECT count(*) FROM app.record_version v JOIN app.record r ON r.id=v.record_id WHERE r.collection_id=? AND r.data @> '{"sku":1}' AND (v.version=1 AND v.data @> '{"qty":1}' OR v.version=2 AND v.data @> '{"qty":2}')`, c.ID).Scan(&previous).Error; err != nil || previous != 2 {
		t.Fatal(previous, err)
	}
	// Two different existing targets, then a fresh target, claim the same value.
	if r := send(skip, `{"sku":1,"tag":"blue"}`, `{"sku":2,"tag":"blue"}`, `{"sku":3,"tag":"blue"}`, `{"sku":3,"tag":"green"}`); r.Invalid != 2 || r.Updated != 1 || r.Created != 1 || r.Errors[0].Index != 1 || r.Errors[1].Index != 2 {
		t.Fatal(r)
	}
	// New keys repeat inside this import as well. Invalid patches cannot contaminate later ones.
	if r := send(skip, `{"sku":4,"qty":1}`, `{"sku":4,"qty":"wrong"}`, `{"sku":4,"tag":"four"}`); r.Invalid != 1 || r.Created != 1 || r.Updated != 1 {
		t.Fatal(r)
	}
	if r := send(input.ImportRecordsCommand{SkipInvalid: true}, `{"sku":9007199254740992}`, `{"sku":9007199254740993}`, `{"sku":9007199254740993.0}`, `{"sku":9.007199254740992e15}`); r.Created != 2 || r.Invalid != 2 || r.Errors[0].Index != 2 || r.Errors[1].Index != 3 {
		t.Fatal(r)
	}
}

func TestConcurrentUpsertsMergeDisjointFields(t *testing.T) {
	e := newEnv(t)
	cols, records := collectionServices(e)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := cols.Create(ctx, input.CreateCollectionCommand{Name: "Concurrent imports", Schema: json.RawMessage(`{"type":"object","properties":{"sku":{"type":"string","x-unique":true}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, raw := range []string{`{"sku":"A","qty":2}`, `{"sku":"A","tag":"done"}`} {
		go func() {
			<-start
			_, err := records.Import(ctx, input.ImportRecordsCommand{CollectionID: c.ID, UpsertKey: "sku", Records: []json.RawMessage{json.RawMessage(raw)}})
			done <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := testDB.Raw(`SELECT count(*) FROM app.record WHERE collection_id=? AND data='{"sku":"A","qty":2,"tag":"done"}'::jsonb AND version=2`, c.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("merged records=%d err=%v", count, err)
	}
}

func TestImportVersionsAndWebhookRollback(t *testing.T) {
	e := newEnv(t)
	cols, records := collectionServices(e)
	hooks, _ := toolServices(e, t)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := cols.Create(ctx, input.CreateCollectionCommand{Name: "Notifications", Schema: json.RawMessage(`{"type":"object","properties":{"sku":{"type":"integer","x-unique":true}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	hook, err := hooks.CreateWebhook(ctx, c.ID, "https://example.com/updates", []string{domain.EventRecordUpdated})
	if err != nil {
		t.Fatal(err)
	}
	cmd := input.ImportRecordsCommand{CollectionID: c.ID, UpsertKey: "sku", Records: []json.RawMessage{json.RawMessage(`{"sku":1}`), json.RawMessage(`{"sku":2}`)}}
	if _, err := records.Import(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	cmd.BatchKey = "two-updates"
	for range 2 {
		if _, err := records.Import(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	// Stage one valid update, then fail on a different field in the dependent segment.
	cmd.BatchKey = ""
	cmd.Records = append(cmd.Records, json.RawMessage(`{"sku":1}`), json.RawMessage(`{"sku":"invalid"}`))
	r, err := records.Import(ctx, cmd)
	if err != nil || r.Invalid != 1 || r.Updated != 0 {
		t.Fatal(r, err)
	}
	var jobs int
	if err := testDB.Raw(`SELECT count(DISTINCT payload #>> '{body,data,id}') FROM sys.job WHERE kind='webhook.deliver' AND payload->>'hook_id'=? AND payload #>> '{body,data,version}'='2'`, hook.Webhook.ID).Scan(&jobs).Error; err != nil || jobs != 2 {
		t.Fatal(jobs, err)
	}
	if err := testDB.Raw(`SELECT count(*) FROM sys.job WHERE payload->>'hook_id'=?`, hook.Webhook.ID).Scan(&jobs).Error; err != nil || jobs != 2 {
		t.Fatal(jobs, err)
	}
	var ids []string
	if err := testDB.Raw("SELECT id FROM app.record WHERE collection_id=? ORDER BY id", c.ID).Scan(&ids).Error; err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	err = e.store.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		_, err := uow.Records().UpdateMany(ctx, []output.RecordChange{{ID: ids[0], Version: 2, Data: json.RawMessage(`{}`)}, {ID: ids[1], Version: 1, Data: json.RawMessage(`{}`)}})
		return err
	})
	wantCode(t, err, apperror.CodeConflict)
	var versions int
	if err := testDB.Raw(`SELECT count(*) FROM app.record_version v JOIN app.record r ON v.record_id=r.id WHERE r.collection_id=? AND r.version=2 AND v.version=1`, c.ID).Scan(&versions).Error; err != nil || versions != 2 {
		t.Fatal(versions, err)
	}
}
