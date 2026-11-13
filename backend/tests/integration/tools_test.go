package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/files"
	"github.com/raulaguila/dataset/internal/adapter/outbound/webhook"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/integration"
	"github.com/raulaguila/dataset/internal/core/usecase/tools"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

func toolServices(e *env, t *testing.T) (*integration.Service, *tools.Service) {
	key := make([]byte, 32)
	return &integration.Service{Store: e.store, Tx: e.store, Send: webhook.New(true), SealKey: key},
		&tools.Service{Store: e.store, Tx: e.store, Files: &files.Local{Dir: t.TempDir()}}
}

// A key reaches one collection's records with at most its role, and stops with its maker.
func TestAPIKeysReachOneCollection(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, _ := toolServices(e, t)
	ownerAddr := e.activeUser(t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Chaves", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	other, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Outra", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	read, err := keys.CreateKey(owner, c.ID, "Leitura", false)
	if err != nil {
		t.Fatal(err)
	}
	write, err := keys.CreateKey(owner, c.ID, "Escrita", true)
	if err != nil {
		t.Fatal(err)
	}
	as := func(secret string) context.Context {
		g, err := keys.AuthenticateKey(context.Background(), secret)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		ctx := reqctx.WithActor(context.Background(), g.UserID)
		return reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: g.KeyID, Name: g.Name, CollectionID: g.CollectionID, CanWrite: g.CanWrite})
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), "dsk_nope")), apperror.CodeUnauthorized)

	// Read: lists, cannot write; write: creates; neither sees the other collection nor changes fields.
	if _, _, err := records.List(as(read.Secret), domain.RecordFilter{CollectionID: c.ID}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(as(read.Secret), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)})), apperror.CodeForbidden)
	r, err := records.Create(as(write.Secret), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore2(records.List(as(write.Secret), domain.RecordFilter{CollectionID: other.ID})), apperror.CodeNotFound)
	wantCode(t, ignore(collections.Update(as(write.Secret), input.UpdateCollectionCommand{ID: c.ID, Schema: json.RawMessage(`{}`)})), apperror.CodeForbidden)

	// The log names the key.
	log, _, err := collections.AuditLog(owner, c.ID, domain.AuditFilter{Action: "record.create", Page: domain.NewPage(1, 5)})
	if err != nil || len(log) != 1 || log[0].Details["api_key"] != "Escrita" || log[0].TargetID != r.ID {
		t.Fatalf("audit: %+v, %v", log, err)
	}

	// Deleting the key ends it.
	if err := keys.DeleteKey(owner, c.ID, write.Key.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), write.Secret)), apperror.CodeUnauthorized)
}

// Record changes queue one delivery per hook that asked for the event.
func TestWebhooksQueueOnRecordChanges(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	hooks, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Ganchos", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(hooks.CreateWebhook(owner, c.ID, "ftp://x", []string{domain.EventRecordCreated})), apperror.CodeInvalidInput)
	wantCode(t, ignore(hooks.CreateWebhook(owner, c.ID, "https://example.com/h", []string{"nope"})), apperror.CodeInvalidInput)
	h, err := hooks.CreateWebhook(owner, c.ID, "https://example.com/h", []string{domain.EventRecordCreated})
	if err != nil || h.Secret == "" {
		t.Fatalf("create: %+v, %v", h, err)
	}
	count := func() (n int64) {
		testDB.Raw(`SELECT count(*) FROM sys.job WHERE kind = 'webhook.deliver' AND payload->>'hook_id' = ?`, h.Webhook.ID).Find(&n)
		return n
	}
	r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Update(owner, input.UpdateRecordCommand{ID: r.ID, Data: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 { // updated was not asked for
		t.Fatalf("queued %d deliveries, want 1", n)
	}
}

// Deleted records and collections wait in the trash and come back whole.
func TestTrashKeepsRecordsAndCollections(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	_, box := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Lixo", Schema: json.RawMessage(`{"type":"object","properties":{"cod":{"type":"string","x-unique":true}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"cod":"a"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Update(owner, input.UpdateRecordCommand{ID: r.ID, Data: json.RawMessage(`{"cod":"b"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := records.DeleteMany(owner, []string{r.ID}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Get(owner, r.ID)), apperror.CodeNotFound)
	trashed, err := box.TrashedRecords(owner, c.ID)
	if err != nil || len(trashed) != 1 || trashed[0].ID != r.ID {
		t.Fatalf("trash: %+v, %v", trashed, err)
	}

	// Its unique value is free while it is away; taking it blocks the restore.
	taker, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"cod":"b"}`)})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID})), apperror.CodeConflict)
	if err := records.DeleteMany(owner, []string{taker.ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := box.RestoreRecords(owner, c.ID, []string{r.ID}); err != nil || n != 1 {
		t.Fatalf("restore: %d, %v", n, err)
	}
	if got, err := records.Get(owner, r.ID); err != nil || got.Version != 2 {
		t.Fatalf("restored record keeps its version: %+v, %v", got, err)
	}
	if v, err := records.Versions(owner, r.ID); err != nil || len(v) != 1 {
		t.Fatalf("restored record keeps its history: %+v, %v", v, err)
	}
	if n, err := box.PurgeRecords(owner, c.ID, nil); err != nil || n != 1 { // the taker
		t.Fatalf("purge: %d, %v", n, err)
	}

	// A collection comes back with its records; purged, it is gone.
	if err := collections.DeleteMany(owner, []string{c.ID}); err != nil {
		t.Fatal(err)
	}
	gone, err := box.TrashedCollections(owner)
	if err != nil || len(gone) != 1 || gone[0].RecordCount != 1 {
		t.Fatalf("trashed collections: %+v, %v", gone, err)
	}
	if err := box.RestoreCollection(owner, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := records.Get(owner, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := collections.DeleteMany(owner, []string{c.ID}); err != nil {
		t.Fatal(err)
	}
	if err := box.PurgeCollection(owner, c.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, box.RestoreCollection(owner, c.ID), apperror.CodeNotFound)
}

// Sorting by a field, editing in bulk, references, files, field history and views.
func TestRecordToolsFollowTheFields(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	_, box := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	people, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Pessoas", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	ana, err := records.Create(owner, input.CreateRecordCommand{CollectionID: people.ID, Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Tarefas", Schema: json.RawMessage(`{"type":"object","properties":{
		"n":{"type":"integer"},"s":{"enum":["a","b"]},
		"dono":{"type":"string","format":"uuid","x-ref":"` + people.ID + `"},
		"anexo":{"type":"object","x-file":true,"properties":{"id":{"type":"string"}}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range []int{3, 1, 2} {
		r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: tasks.ID, Data: json.RawMessage(`{"n":` + string(rune('0'+n)) + `}`)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	list, _, err := records.List(owner, domain.RecordFilter{CollectionID: tasks.ID, Sort: "data.n", Order: "asc"})
	if err != nil || len(list) != 3 || string(list[0].Data) != `{"n": 1}` || string(list[2].Data) != `{"n": 3}` {
		t.Fatalf("sort by n: %+v, %v", list, err)
	}
	if list, _, err := records.List(owner, domain.RecordFilter{CollectionID: tasks.ID, IDs: ids[:2]}); err != nil || len(list) != 2 {
		t.Fatalf("by ids: %+v, %v", list, err)
	}

	// Bulk: all or none against the fields.
	wantCode(t, ignore(records.BulkUpdate(owner, input.BulkUpdateCommand{IDs: ids, Set: map[string]json.RawMessage{"s": json.RawMessage(`"z"`)}})), apperror.CodeValidationFailed)
	if n, err := records.BulkUpdate(owner, input.BulkUpdateCommand{IDs: ids, Set: map[string]json.RawMessage{"s": json.RawMessage(`"b"`)}}); err != nil || n != 3 {
		t.Fatalf("bulk: %d, %v", n, err)
	}
	if list, _, err := records.List(owner, domain.RecordFilter{CollectionID: tasks.ID, Match: json.RawMessage(`{"s":"b"}`)}); err != nil || len(list) != 3 {
		t.Fatalf("after bulk: %+v, %v", list, err)
	}

	// A reference must name a record of the other collection; a file one of this collection.
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: tasks.ID, Data: json.RawMessage(`{"dono":"` + ids[0] + `"}`)})), apperror.CodeValidationFailed)
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: tasks.ID, Data: json.RawMessage(`{"dono":"` + ana.ID + `"}`)}); err != nil {
		t.Fatal(err)
	}
	f, err := box.UploadFile(owner, tasks.ID, "nota.txt", "text/plain", []byte("oi"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: tasks.ID, Data: json.RawMessage(`{"anexo":{"id":"` + f.ID + `"}}`)}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: tasks.ID, Data: json.RawMessage(`{"anexo":{"id":"` + ana.ID + `"}}`)})), apperror.CodeValidationFailed)
	if got, data, err := box.File(owner, f.ID); err != nil || string(data) != "oi" || got.Name != "nota.txt" {
		t.Fatalf("file: %+v %q %v", got, data, err)
	}

	// The field history keeps each schema a change replaced.
	if _, err := collections.Update(owner, input.UpdateCollectionCommand{ID: people.ID, Schema: json.RawMessage(`{"type":"object","properties":{"nome":{"type":"string"}}}`)}); err != nil {
		t.Fatal(err)
	}
	if v, err := box.SchemaVersions(owner, people.ID); err != nil || len(v) != 1 || string(v[0].Schema) != `{"type":"object"}` {
		t.Fatalf("history: %+v, %v", v, err)
	}

	// Views: own and shared.
	view, err := box.CreateView(owner, tasks.ID, "Por n", "?sort=data.n", true)
	if err != nil || view.Query != "sort=data.n" {
		t.Fatalf("view: %+v, %v", view, err)
	}
	if vs, err := box.Views(owner, tasks.ID); err != nil || len(vs) != 1 || !vs[0].Mine {
		t.Fatalf("views: %+v, %v", vs, err)
	}
	if err := box.DeleteView(owner, tasks.ID, view.ID); err != nil {
		t.Fatal(err)
	}
}
