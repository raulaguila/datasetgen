package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/adapter/outbound/emailpayload"
	"github.com/raulaguila/dataset/internal/adapter/outbound/files"
	"github.com/raulaguila/dataset/internal/adapter/outbound/webhook"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/integration"
	"github.com/raulaguila/dataset/internal/core/usecase/tools"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

func toolServices(e *env, t *testing.T) (*integration.Service, *tools.Service) {
	key := make([]byte, 32)
	_, records := collectionServices(e)
	return &integration.Service{Store: e.store, Tx: e.store, Send: webhook.New(true), SealKey: key, Records: records, Mail: emailpayload.New(key)},
		&tools.Service{Store: e.store, Tx: e.store, Files: &files.Local{Dir: t.TempDir()}}
}

// A key reaches the collections its maker picked, each with at most the access
// picked and the maker's role, and stops with its maker.
func TestAPIKeysReachTheirCollections(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, _ := toolServices(e, t)
	ownerAddr := e.activeUser(t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	mk := func(name string) string {
		c, err := collections.Create(owner, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(`{"type":"object"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	a, b, other := mk("Chaves"), mk("Outra"), mk("Fora")
	read, err := keys.CreateMyKey(owner, "Leitura", []domain.KeyScope{{CollectionID: a}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	write, err := keys.CreateMyKey(owner, "Escrita", []domain.KeyScope{{CollectionID: a, CanWrite: true}, {CollectionID: b}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.CreateMyKey(owner, "Vazia", nil, 0)), apperror.CodeInvalidInput)
	wantCode(t, ignore(keys.CreateMyKey(owner, "Dupla", []domain.KeyScope{{CollectionID: a}, {CollectionID: a}}, 0)), apperror.CodeInvalidInput)
	as := func(secret string) context.Context {
		g, err := keys.AuthenticateKey(context.Background(), secret)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		ctx := reqctx.WithActor(context.Background(), g.UserID)
		return reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: g.KeyID, Name: g.Name, Scopes: g.Scopes})
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), "dsk_nope")), apperror.CodeUnauthorized)

	// Read: lists, cannot write; write: creates in a, only reads b; neither sees the
	// collection left out nor changes fields; a key cannot make keys.
	if _, _, err := records.List(as(read.Secret), domain.RecordFilter{CollectionID: a}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(as(read.Secret), input.CreateRecordCommand{CollectionID: a, Data: json.RawMessage(`{}`)})), apperror.CodeForbidden)
	r, err := records.Create(as(write.Secret), input.CreateRecordCommand{CollectionID: a, Data: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := records.List(as(write.Secret), domain.RecordFilter{CollectionID: b}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(as(write.Secret), input.CreateRecordCommand{CollectionID: b, Data: json.RawMessage(`{}`)})), apperror.CodeForbidden)
	wantCode(t, ignore2(records.List(as(write.Secret), domain.RecordFilter{CollectionID: other})), apperror.CodeNotFound)
	wantCode(t, ignore(collections.Update(as(write.Secret), input.UpdateCollectionCommand{ID: a, Schema: json.RawMessage(`{}`)})), apperror.CodeForbidden)
	wantCode(t, ignore(keys.CreateMyKey(as(write.Secret), "Filha", []domain.KeyScope{{CollectionID: a}}, 0)), apperror.CodeForbidden)

	// The log names the key.
	log, _, err := collections.AuditLog(owner, a, domain.AuditFilter{Action: "record.create", Page: domain.NewPage(1, 5)})
	if err != nil || len(log) != 1 || log[0].Details["api_key"] != "Escrita" || log[0].TargetID != r.ID {
		t.Fatalf("audit: %+v, %v", log, err)
	}

	// A viewer may make a read key, not a write one, and only for their collections.
	viewerAddr := e.activeUser(t)
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: a, Email: viewerAddr, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewer := reqctx.WithActor(context.Background(), e.userID(t, viewerAddr))
	wantCode(t, ignore(keys.CreateMyKey(viewer, "V", []domain.KeyScope{{CollectionID: a, CanWrite: true}}, 0)), apperror.CodeForbidden)
	wantCode(t, ignore(keys.CreateMyKey(viewer, "V", []domain.KeyScope{{CollectionID: b}}, 0)), apperror.CodeNotFound)
	if _, err := keys.CreateMyKey(viewer, "V", []domain.KeyScope{{CollectionID: a}}, 0); err != nil {
		t.Fatal(err)
	}
	// The owner sees every key that reaches the collection, each member's own list only theirs.
	if list, err := keys.Keys(owner, a); err != nil || len(list) != 3 {
		t.Fatalf("collection keys: %+v, %v", list, err)
	}
	if mine, err := keys.MyKeys(owner); err != nil || len(mine) != 2 || len(mine[0].Scopes) != 2 {
		t.Fatalf("my keys: %+v, %v", mine, err)
	}
	wantCode(t, ignore(keys.UpdateMyKey(viewer, write.Key.ID, "Minha", []domain.KeyScope{{CollectionID: a}}, nil)), apperror.CodeNotFound)
	wantCode(t, keys.DeleteMyKey(viewer, read.Key.ID), apperror.CodeNotFound)

	// Removing a member takes the collection out of their keys: coming back does not revive them.
	viewerKey, err := keys.CreateMyKey(viewer, "Volta", []domain.KeyScope{{CollectionID: a}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.RemoveMember(owner, a, e.userID(t, viewerAddr)); err != nil {
		t.Fatal(err)
	}
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: a, Email: viewerAddr, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore2(records.List(as(viewerKey.Secret), domain.RecordFilter{CollectionID: a})), apperror.CodeNotFound)
	if list, err := keys.Keys(owner, a); err != nil || len(list) != 2 {
		t.Fatalf("keys after removal: %+v, %v", list, err)
	}

	// Changing a key's collections takes effect at once.
	k, err := keys.UpdateMyKey(owner, write.Key.ID, "Escrita B", []domain.KeyScope{{CollectionID: b, CanWrite: true}, {CollectionID: other}}, nil)
	if err != nil || len(k.Scopes) != 2 || k.Name != "Escrita B" {
		t.Fatalf("update: %+v, %v", k, err)
	}
	wantCode(t, ignore2(records.List(as(write.Secret), domain.RecordFilter{CollectionID: a})), apperror.CodeNotFound)
	if _, err := records.Create(as(write.Secret), input.CreateRecordCommand{CollectionID: b, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	log, _, err = collections.AuditLog(owner, a, domain.AuditFilter{Action: "collection.key_delete", Page: domain.NewPage(1, 5)})
	if err != nil || len(log) != 1 || log[0].Details["key"] != "Escrita" || log[0].Details["prefix"] != write.Key.Prefix {
		t.Fatalf("key_delete audit: %+v, %v", log, err)
	}

	// An owner takes their collection out of a key; the key keeps the others.
	if err := keys.DeleteKey(owner, b, write.Key.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore2(records.List(as(write.Secret), domain.RecordFilter{CollectionID: b})), apperror.CodeNotFound)
	if _, _, err := records.List(as(write.Secret), domain.RecordFilter{CollectionID: other}); err != nil {
		t.Fatal(err)
	}

	// Deleting the key ends it.
	if err := keys.DeleteMyKey(owner, write.Key.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), write.Secret)), apperror.CodeUnauthorized)

	// A key with an end date works until then and not after.
	wantCode(t, ignore(keys.CreateMyKey(owner, "Longa", []domain.KeyScope{{CollectionID: a}}, 400)), apperror.CodeInvalidInput)
	brief, err := keys.CreateMyKey(owner, "Breve", []domain.KeyScope{{CollectionID: a}}, 30)
	if err != nil || brief.Key.ExpiresAt == nil || time.Until(*brief.Key.ExpiresAt) < 29*24*time.Hour {
		t.Fatalf("expiring key: %v %v", brief, err)
	}
	if _, err := keys.AuthenticateKey(context.Background(), brief.Secret); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec("UPDATE app.api_key SET expires_at = now() - INTERVAL '1 second' WHERE id = ?", brief.Key.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), brief.Secret)), apperror.CodeUnauthorized)

	// Renewing brings an expired key back; no renewal keeps the end; 0 removes it.
	scope := []domain.KeyScope{{CollectionID: a}}
	wantCode(t, ignore(keys.UpdateMyKey(owner, brief.Key.ID, "Breve", scope, pointer.Of(400))), apperror.CodeInvalidInput)
	renewed, err := keys.UpdateMyKey(owner, brief.Key.ID, "Breve", scope, pointer.Of(90))
	if err != nil || renewed.ExpiresAt == nil || time.Until(*renewed.ExpiresAt) < 89*24*time.Hour {
		t.Fatalf("renew: %+v %v", renewed, err)
	}
	if _, err := keys.AuthenticateKey(context.Background(), brief.Secret); err != nil {
		t.Fatalf("a renewed key does not work: %v", err)
	}
	if kept, err := keys.UpdateMyKey(owner, brief.Key.ID, "Breve", scope, nil); err != nil || kept.ExpiresAt == nil || !kept.ExpiresAt.Equal(*renewed.ExpiresAt) {
		t.Fatalf("update without renewal changed the end: %+v %v", kept, err)
	}
	if open, err := keys.UpdateMyKey(owner, brief.Key.ID, "Breve", scope, pointer.Of(0)); err != nil || open.ExpiresAt != nil {
		t.Fatalf("renew with no end: %+v %v", open, err)
	}
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
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID}, nil)), apperror.CodeConflict)
	// New values: only unique fields, one record, fitting the field and still free.
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID}, map[string]json.RawMessage{"outro": json.RawMessage(`"x"`)})), apperror.CodeInvalidInput)
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID, taker.ID}, map[string]json.RawMessage{"cod": json.RawMessage(`"x"`)})), apperror.CodeInvalidInput)
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID}, map[string]json.RawMessage{"cod": json.RawMessage(`5`)})), apperror.CodeValidationFailed)
	wantCode(t, ignore(box.RestoreRecords(owner, c.ID, []string{r.ID}, map[string]json.RawMessage{"cod": json.RawMessage(`"b"`)})), apperror.CodeConflict)
	if err := records.DeleteMany(owner, []string{taker.ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := box.RestoreRecords(owner, c.ID, []string{r.ID}, nil); err != nil || n != 1 {
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
	// Taken again, it comes back with a new value instead.
	if err := records.DeleteMany(owner, []string{r.ID}); err != nil {
		t.Fatal(err)
	}
	taker, err = records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"cod":"b"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := box.RestoreRecords(owner, c.ID, []string{r.ID}, map[string]json.RawMessage{"cod": json.RawMessage(`"c"`)}); err != nil || n != 1 {
		t.Fatalf("restore with a new value: %d, %v", n, err)
	}
	if got, err := records.Get(owner, r.ID); err != nil || string(got.Data) != `{"cod": "c"}` || got.Version != 3 {
		t.Fatalf("renewed: %+v, %v", got, err)
	}
	if err := records.DeleteMany(owner, []string{taker.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := box.PurgeRecords(owner, c.ID, nil); err != nil {
		t.Fatal(err)
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

	// The dashboard starts empty, keeps what owners save and refuses unknown widgets.
	if w, err := box.Dashboard(owner, tasks.ID); err != nil || string(w) != "[]" {
		t.Fatalf("empty dashboard: %s %v", w, err)
	}
	widgets := json.RawMessage(`[{"kind":"count","title":"Tarefas"},{"kind":"bars","title":"Por dono","field":"dono"}]`)
	if err := box.SaveDashboard(owner, tasks.ID, widgets); err != nil {
		t.Fatal(err)
	}
	if w, err := box.Dashboard(owner, tasks.ID); err != nil || string(w) != string(widgets) {
		t.Fatalf("dashboard: %s %v", w, err)
	}
	wantCode(t, box.SaveDashboard(owner, tasks.ID, json.RawMessage(`[{"kind":"pizza"}]`)), apperror.CodeInvalidInput)
	wantCode(t, box.SaveDashboard(owner, tasks.ID, json.RawMessage(`{}`)), apperror.CodeInvalidInput)

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

// Rules select records by comparison; groups count them and sum numbers.
func TestWhereRulesAndGroups(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Vendas", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		`{"cat":"a","v":10,"nome":"Caneta azul","tags":["x"]}`,
		`{"cat":"a","v":5,"nome":"Lápis","tags":["y"]}`,
		`{"cat":"b","v":7,"nome":"Caneta preta"}`,
		`{"cat":"b","nome":""}`,
	} {
		if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(d)}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(w domain.Where) int {
		t.Helper()
		list, _, err := records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &w})
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}
	rule := func(f, op, v string) domain.Condition {
		return domain.Condition{Field: f, Op: op, Value: json.RawMessage(v)}
	}
	cases := []struct {
		name string
		w    domain.Where
		want int
	}{
		{"gt", domain.Where{Rules: []domain.Condition{rule("v", "gt", "6")}}, 2},
		{"contains and lte", domain.Where{Rules: []domain.Condition{rule("nome", "contains", `"caneta"`), rule("v", "lte", "7")}}, 1},
		{"or", domain.Where{Any: true, Rules: []domain.Condition{rule("v", "eq", "5"), rule("cat", "eq", `"b"`)}}, 3},
		{"empty", domain.Where{Rules: []domain.Condition{{Field: "v", Op: "empty"}}}, 1},
		{"in", domain.Where{Rules: []domain.Condition{rule("v", "in", "[5,7]")}}, 2},
		{"has", domain.Where{Rules: []domain.Condition{rule("tags", "has", `"x"`)}}, 1},
		{"neq counts missing", domain.Where{Rules: []domain.Condition{rule("v", "neq", "10")}}, 3},
		// (cat a AND v > 6) OR (cat b AND v < 8)
		{"groups", domain.Where{Any: true, Groups: []domain.Where{
			{Rules: []domain.Condition{rule("cat", "eq", `"a"`), rule("v", "gt", "6")}},
			{Rules: []domain.Condition{rule("cat", "eq", `"b"`), rule("v", "lt", "8")}},
		}}, 2},
	}
	for _, tc := range cases {
		if got := count(tc.w); got != tc.want {
			t.Errorf("%s: %d records, want %d", tc.name, got, tc.want)
		}
	}
	wantCode(t, ignore2(records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Rules: []domain.Condition{rule("v", "drop table", "1")}}})), apperror.CodeInvalidInput)
	wantCode(t, ignore2(records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Rules: []domain.Condition{rule("v", "in", "5")}}})), apperror.CodeInvalidInput)
	wantCode(t, ignore2(records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Groups: []domain.Where{{Groups: []domain.Where{{}}}}}})), apperror.CodeInvalidInput)

	groups, err := records.Aggregate(owner, domain.RecordFilter{CollectionID: c.ID}, "cat", []string{"v"})
	if err != nil || len(groups) != 2 {
		t.Fatalf("groups: %+v, %v", groups, err)
	}
	for _, g := range groups {
		want := map[string]float64{`"a"`: 15, `"b"`: 7}[string(g.Key)]
		if g.Count != 2 || g.Sums["v"] != want {
			t.Errorf("group %s: %+v, want count 2 sum %v", g.Key, g, want)
		}
	}
	total, err := records.Aggregate(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Rules: []domain.Condition{rule("cat", "eq", `"a"`)}}}, "", []string{"v"})
	if err != nil || len(total) != 1 || total[0].Count != 2 || total[0].Avgs["v"] != 7.5 || total[0].Mins["v"] != 5 || total[0].Maxs["v"] != 10 {
		t.Fatalf("totals: %+v, %v", total, err)
	}

	for _, d := range []string{`{"cat":"c","quando":"2026-01-05"}`, `{"cat":"c","quando":"2026-01-20T10:00:00Z"}`, `{"cat":"c","quando":"2026-03-01"}`, `{"cat":"c","quando":"2026-02-30"}`} {
		if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(d)}); err != nil {
			t.Fatal(err)
		}
	}
	st, err := records.Stats(owner, domain.RecordFilter{CollectionID: c.ID}, domain.StatsQuery{Filled: []string{"v", "nome"}, Numbers: []string{"v"}, Dates: []string{"quando"}, Values: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}
	v, nome, cat, quando := st.Fields["v"], st.Fields["nome"], st.Fields["cat"], st.Fields["quando"]
	if st.Total != 8 || v.Filled != 3 || nome.Filled != 3 || *v.Min != 5 || *v.Max != 10 || *v.Sum != 22 {
		t.Errorf("stats: total %d, v %+v, nome %+v", st.Total, v, nome)
	}
	if cat.Distinct != 3 || len(cat.Top) != 3 || string(cat.Top[0].Key) != `"c"` || cat.Top[0].Count != 4 {
		t.Errorf("cat values: %+v", cat)
	}
	// The invalid 2026-02-30 is skipped; the datetime counts by its date.
	if *quando.First != "2026-01-05" || *quando.Last != "2026-03-01" || len(quando.Series) != 2 || quando.Series[0] != (domain.DateCount{At: "2026-01-01", Count: 2}) {
		t.Errorf("dates: %+v first %v last %v", quando.Series, *quando.First, *quando.Last)
	}
	wantCode(t, ignore(records.Stats(owner, domain.RecordFilter{CollectionID: c.ID}, domain.StatsQuery{Bucket: "hour"})), apperror.CodeInvalidInput)
}

// A share link opens one collection to anyone holding it: a form adds records,
// a list reads them; neither is an API key, and deleting the link ends it.
func TestShareLinks(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Links", Schema: json.RawMessage(`{"type":"object","required":["n","doc"],"properties":{"n":{"type":"string"},"doc":{"type":"object","x-file":true}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	form, err := keys.CreateShare(owner, c.ID, domain.ShareForm, 0)
	if err != nil {
		t.Fatal(err)
	}
	view, err := keys.CreateShare(owner, c.ID, domain.ShareView, 7)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.CreateShare(owner, c.ID, "edit", 0)), apperror.CodeInvalidInput)
	wantCode(t, ignore(keys.CreateShare(owner, c.ID, domain.ShareView, 400)), apperror.CodeInvalidInput)
	if form.ExpiresAt != nil || view.ExpiresAt == nil || time.Until(*view.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("expiry: form %v, view %v", form.ExpiresAt, view.ExpiresAt)
	}
	// The owner can copy a link again; the API key list leaves links out.
	list, err := keys.Shares(owner, c.ID)
	if err != nil || len(list) != 2 || (list[0].Token != view.Token && list[0].Token != form.Token) {
		t.Fatalf("shares: %+v, %v", list, err)
	}
	if k, err := keys.Keys(owner, c.ID); err != nil || len(k) != 0 {
		t.Fatalf("keys: %+v, %v", k, err)
	}
	wantCode(t, ignore(keys.AuthenticateKey(context.Background(), form.Token)), apperror.CodeUnauthorized)
	as := func(token string) context.Context {
		g, err := keys.AuthenticateShare(context.Background(), token)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		ctx := reqctx.WithActor(context.Background(), g.UserID)
		return reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: g.KeyID, Name: g.Name, Scopes: g.Scopes, Form: g.Share == domain.ShareForm})
	}
	// The form's record follows the fields, but a file it cannot upload is not
	// required there; the owner still must fill it. The list cannot write.
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"n":"x"}`)})), apperror.CodeValidationFailed)
	wantCode(t, ignore(records.Create(as(form.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)})), apperror.CodeValidationFailed)
	if _, err := records.Create(as(form.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"n":"x"}`)}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(as(view.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"n":"y"}`)})), apperror.CodeForbidden)
	if items, total, err := records.List(as(view.Token), domain.RecordFilter{CollectionID: c.ID, Page: domain.NewPage(1, 10)}); err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list: %d, %v", total, err)
	}
	// An expired link stops working.
	if err := testDB.Exec(`UPDATE app.api_key SET expires_at = now() - interval '1 minute' WHERE id = ?`, form.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.AuthenticateShare(context.Background(), form.Token)), apperror.CodeNotFound)
	if err := keys.DeleteShare(owner, c.ID, view.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.AuthenticateShare(context.Background(), view.Token)), apperror.CodeNotFound)
}

// A reference field may hold several ids; the target record lists who points at it.
func TestManyReferencesAndLinks(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	tags, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Tags", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: tags.ID, Data: json.RawMessage(`{"n":"a"}`)})
	b, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: tags.ID, Data: json.RawMessage(`{"n":"b"}`)})
	posts, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Posts", Schema: json.RawMessage(`{"type":"object","properties":{
		"tags":{"type":"array","items":{"type":"string"},"x-ref":"` + tags.ID + `"},
		"main":{"type":"string","x-ref":"` + tags.ID + `"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: posts.ID, Data: json.RawMessage(`{"tags":["` + a.ID + `","` + b.ID + `"],"main":"` + a.ID + `"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: posts.ID, Data: json.RawMessage(`{"tags":["` + b.ID + `"]}`)}); err != nil {
		t.Fatal(err)
	}
	// A list naming a record that does not exist is refused.
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: posts.ID, Data: json.RawMessage(`{"tags":["` + a.ID + `","00000000-0000-4000-8000-000000000000"]}`)})), apperror.CodeValidationFailed)

	links, err := records.Links(owner, a.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("links of a: %+v, %v", links, err)
	}
	for _, l := range links {
		if l.CollectionID != posts.ID || l.Total != 1 {
			t.Fatalf("link: %+v", l)
		}
	}
	links, err = records.Links(owner, b.ID)
	if err != nil || len(links) != 1 || links[0].Field != "tags" || links[0].Total != 2 {
		t.Fatalf("links of b: %+v, %v", links, err)
	}
	stranger := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	wantCode(t, ignore(records.Links(stranger, a.ID)), apperror.CodeNotFound)
}

// An automation acts on the records that pass its conditions, as its maker;
// its own change starts no automation; a daily one runs on every match.
func TestAutomationsActOnMatchingRecords(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	autos, _ := toolServices(e, t)
	ownerID := e.userID(t, e.activeUser(t))
	owner := reqctx.WithActor(context.Background(), ownerID)
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Auto", Schema: json.RawMessage(`{"type":"object","properties":{"p":{"type":"integer"},"s":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	cond := domain.Where{Rules: []domain.Condition{{Field: "p", Op: domain.OpGt, Value: json.RawMessage(`5`)}}}
	wantCode(t, ignore(autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "x", Trigger: "nope", Action: domain.ActionNotify, Config: json.RawMessage(`{"message":"m"}`)})), apperror.CodeInvalidInput)
	wantCode(t, ignore(autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "x", Trigger: domain.TriggerDaily, Action: domain.ActionNotify, Config: json.RawMessage(`{"message":"m"}`)})), apperror.CodeInvalidInput)
	wantCode(t, ignore(autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "x", Trigger: domain.TriggerCreated, Conditions: domain.Where{Rules: []domain.Condition{{Field: "p", Op: "bad"}}}, Action: domain.ActionNotify, Config: json.RawMessage(`{"message":"m"}`)})), apperror.CodeInvalidInput)
	set, err := autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "Urgente", Enabled: true, Trigger: domain.TriggerCreated, Conditions: cond, Action: domain.ActionSetField, Config: json.RawMessage(`{"field":"s","value":"urgente"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// Watching updates: the set_field change above must not start it.
	if _, err := autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "Mudou", Enabled: true, Trigger: domain.TriggerUpdated, Action: domain.ActionNotify, Config: json.RawMessage(`{"message":"mudou"}`)}); err != nil {
		t.Fatal(err)
	}
	jobs := func(kind string) (n int64) {
		testDB.Raw(`SELECT count(*) FROM sys.job WHERE kind = ? AND payload->>'automation_id' IN (SELECT id::text FROM app.automation WHERE collection_id = ?)`, kind, c.ID).Find(&n)
		return n
	}
	hi, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"p":9}`)})
	lo, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"p":1}`)})
	if n := jobs("automation.run"); n != 2 {
		t.Fatalf("queued %d runs, want 2", n)
	}
	run := autos.Jobs()["automation.run"]
	for _, id := range []string{hi.ID, lo.ID} {
		payload, _ := json.Marshal(map[string]string{"automation_id": set.ID, "record_id": id})
		if err := run(context.Background(), payload); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := records.Get(owner, hi.ID)
	if string(got.Data) != `{"p": 9, "s": "urgente"}` && string(got.Data) != `{"p":9,"s":"urgente"}` {
		t.Fatalf("hi: %s", got.Data)
	}
	if got, _ := records.Get(owner, lo.ID); string(got.Data) == `{"p": 1, "s": "urgente"}` {
		t.Fatal("lo changed")
	}
	if n := jobs("automation.run"); n != 2 { // the automation's own update queued nothing
		t.Fatalf("queued %d runs after the change, want 2", n)
	}

	// Daily: needs an hour; run now notifies the maker once for every match.
	hour := 0
	daily, err := autos.CreateAutomation(owner, c.ID, input.AutomationInput{Name: "Resumo", Enabled: true, Trigger: domain.TriggerDaily, Hour: &hour, Conditions: cond, Action: domain.ActionNotify, Config: json.RawMessage(`{"message":"altos"}`)})
	if err != nil {
		t.Fatal(err)
	}
	ran, err := autos.RunAutomation(owner, c.ID, daily.ID)
	if err != nil || ran.Runs != 1 || ran.LastError != "" {
		t.Fatalf("run: %+v, %v", ran, err)
	}
	var notes int64
	testDB.Raw(`SELECT count(*) FROM usr.usr_notification WHERE user_id = ? AND title = 'Resumo' AND params->>'count' = '1'`, ownerID).Find(&notes)
	if notes != 1 {
		t.Fatalf("notifications: %d", notes)
	}
	// Schedule claims it once a day.
	testDB.Exec(`UPDATE app.automation SET last_run_at = NULL WHERE id = ?`, daily.ID)
	if err := autos.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := autos.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	var queued int64
	testDB.Raw(`SELECT count(*) FROM sys.job WHERE kind = 'automation.run' AND payload->>'automation_id' = ?`, daily.ID).Find(&queued)
	if queued != 1 {
		t.Fatalf("daily queued %d, want 1", queued)
	}
}

// Email, webhook and create_record actions: each run queues one email per
// recipient, one webhook call, and one record per match in the target.
func TestAutomationActionsReachOutside(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	autos, _ := toolServices(e, t)
	ownerAddr := e.activeUser(t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	mk := func(name, schema string) string {
		c, err := collections.Create(owner, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(schema)})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	src := mk("Pedidos", `{"type":"object","properties":{"cliente":{"type":"string"},"total":{"type":"number"}}}`)
	dst := mk("Tarefas", `{"type":"object","properties":{"titulo":{"type":"string"},"quem":{"type":"string"},"pedido":{"type":"string"}},"required":["titulo"]}`)
	other := mk("Outra", `{"type":"object"}`)
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: src, Email: e.activeUser(t), Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	hook, err := autos.CreateWebhook(owner, src, "https://example.com/h", []string{domain.EventRecordCreated})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := autos.CreateWebhook(owner, other, "https://example.com/o", []string{domain.EventRecordCreated})
	if err != nil {
		t.Fatal(err)
	}
	newAuto := func(action, config string) (*domain.Automation, error) {
		hour := 3
		return autos.CreateAutomation(owner, src, input.AutomationInput{Name: action, Enabled: true, Trigger: domain.TriggerDaily, Hour: &hour, Action: action, Config: json.RawMessage(config)})
	}
	for _, bad := range [][2]string{
		{domain.ActionEmail, `{"to":"everyone","subject":"s","message":"m"}`},
		{domain.ActionEmail, `{"to":"me","subject":"a\nBcc: x@y","message":"m"}`},
		{domain.ActionWebhook, `{"webhook_id":"` + foreign.Webhook.ID + `"}`},
		{domain.ActionCreateRecord, `{"collection_id":"` + dst + `"}`},
	} {
		wantCode(t, ignore(newAuto(bad[0], bad[1])), apperror.CodeInvalidInput)
	}
	email, err := newAuto(domain.ActionEmail, `{"to":"members","subject":"Novos pedidos","message":"Veja os pedidos."}`)
	if err != nil {
		t.Fatal(err)
	}
	call, err := newAuto(domain.ActionWebhook, `{"webhook_id":"`+hook.Webhook.ID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	create, err := newAuto(domain.ActionCreateRecord, `{"collection_id":"`+dst+`","data":{"titulo":"Separar","quem":"{{cliente}}","pedido":"{{id}}"}}`)
	if err != nil {
		t.Fatal(err)
	}
	r1, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: src, Data: json.RawMessage(`{"cliente":"Ana","total":10}`)})
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: src, Data: json.RawMessage(`{"total":5}`)}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []*domain.Automation{email, call, create} {
		if ran, err := autos.RunAutomation(owner, src, a.ID); err != nil || ran.LastError != "" {
			t.Fatalf("%s: %+v, %v", a.Action, ran, err)
		}
	}
	var mails, calls int64
	testDB.Raw(`SELECT count(*) FROM sys.outbox WHERE purpose = 'automation'`).Find(&mails)
	testDB.Raw(`SELECT count(*) FROM sys.job WHERE kind = 'webhook.deliver' AND payload->>'hook_id' = ? AND payload->'body'->>'event' = 'automation.run' AND jsonb_array_length(payload->'body'->'records') = 2`, hook.Webhook.ID).Find(&calls)
	if mails != 2 || calls != 1 {
		t.Fatalf("emails %d (want 2, both members), webhook calls %d (want 1)", mails, calls)
	}
	made, total, err := records.List(owner, domain.RecordFilter{CollectionID: dst, Page: domain.NewPage(1, 10), Search: "Ana"})
	if err != nil || total != 1 || !strings.Contains(string(made[0].Data), r1.ID) {
		t.Fatalf("created: %+v, %d, %v", made, total, err)
	}
	if _, total, _ := records.List(owner, domain.RecordFilter{CollectionID: dst, Page: domain.NewPage(1, 10)}); total != 2 {
		t.Fatalf("created %d records, want one per match", total)
	}

	// A run that starts failing tells its maker once; more failures stay quiet.
	if err := testDB.Exec("UPDATE app.collection SET deleted_at = now() WHERE id = ?", dst).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if ran, _ := autos.RunAutomation(owner, src, create.ID); ran == nil || ran.LastError == "" {
			t.Fatalf("a create into a deleted collection did not fail: %+v", ran)
		}
	}
	var notes int64
	if err := testDB.Raw("SELECT count(*) FROM usr.usr_notification WHERE kind = ? AND params->>'automation' = ?", domain.NotifyAutomationError, create.Name).Scan(&notes).Error; err != nil || notes != 1 {
		t.Fatalf("failure notices: %d %v", notes, err)
	}
}

// Every webhook call is kept with its answer, and can be sent again.
func TestWebhookDeliveriesAndRedelivery(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	hooks, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, _ := collections.Create(owner, input.CreateCollectionCommand{Name: "Log", Schema: json.RawMessage(`{"type":"object"}`)})
	h, err := hooks.CreateWebhook(owner, c.ID, "http://127.0.0.1:1/nada", []string{domain.EventRecordCreated})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hooks.TestWebhook(owner, c.ID, h.Webhook.ID); err != nil {
		t.Fatal(err)
	}
	list, err := hooks.Deliveries(owner, c.ID, h.Webhook.ID)
	if err != nil || len(list) != 1 || list[0].Event != "ping" || list[0].Error == "" {
		t.Fatalf("deliveries: %+v, %v", list, err)
	}
	again, err := hooks.Redeliver(owner, c.ID, h.Webhook.ID, list[0].ID)
	if err != nil || again.ID == list[0].ID {
		t.Fatalf("redeliver: %+v, %v", again, err)
	}
	if list, _ := hooks.Deliveries(owner, c.ID, h.Webhook.ID); len(list) != 2 {
		t.Fatalf("after redeliver: %d", len(list))
	}

	// Ten failures in a row pause the hook once, tell its maker, and queued calls stop.
	for range domain.HookPauseAfter - 2 {
		if _, err := hooks.TestWebhook(owner, c.ID, h.Webhook.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := hooks.Webhooks(owner, c.ID)
	if len(got) != 1 || got[0].PausedAt == nil || got[0].Failures != domain.HookPauseAfter {
		t.Fatalf("pause: %+v", got)
	}
	var notes int64
	if err := testDB.Raw("SELECT count(*) FROM usr.usr_notification WHERE kind = ?", domain.NotifyWebhookPaused).Scan(&notes).Error; err != nil || notes != 1 {
		t.Fatalf("pause notices: %d %v", notes, err)
	}
	if _, err := hooks.TestWebhook(owner, c.ID, h.Webhook.ID); err != nil { // an 11th failure does not tell again
		t.Fatal(err)
	}
	if err := testDB.Raw("SELECT count(*) FROM usr.usr_notification WHERE kind = ?", domain.NotifyWebhookPaused).Scan(&notes).Error; err != nil || notes != 1 {
		t.Fatalf("pause notices after more failures: %d %v", notes, err)
	}
	body, _ := json.Marshal(policy.WebhookJob{HookID: h.Webhook.ID, Body: map[string]any{"event": "record.created"}})
	if err := hooks.Jobs()[policy.JobWebhook](owner, body); err != nil {
		t.Fatalf("queued call to a paused hook: %v", err)
	}
	if list, _ := hooks.Deliveries(owner, c.ID, h.Webhook.ID); len(list) != domain.HookPauseAfter+1 {
		t.Fatalf("a paused hook still sent: %d deliveries", len(list))
	}
	// Editing keeps the secret's hook and resumes it; a bad address is refused.
	wantCode(t, ignore(hooks.UpdateWebhook(owner, c.ID, h.Webhook.ID, "ftp://x", []string{domain.EventRecordCreated}, true)), apperror.CodeInvalidInput)
	w, err := hooks.UpdateWebhook(owner, c.ID, h.Webhook.ID, "https://example.com/novo", []string{domain.EventRecordUpdated}, true)
	if err != nil || w.PausedAt != nil || w.Failures != 0 || w.URL != "https://example.com/novo" || w.Events[0] != domain.EventRecordUpdated {
		t.Fatalf("resume: %+v %v", w, err)
	}
	if w, err = hooks.UpdateWebhook(owner, c.ID, h.Webhook.ID, w.URL, w.Events, false); err != nil || w.PausedAt == nil {
		t.Fatalf("pause by hand: %+v %v", w, err)
	}
}

// Fields the server fills: the next number, who made and changed a record, formulas;
// an import can update the records a unique field names.
func TestAutoFieldsAndUpsert(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Auto campos", Schema: json.RawMessage(`{"type":"object","required":["n"],"properties":{
		"n":{"type":"integer","x-auto":"number"},
		"sku":{"type":"string","x-unique":true},
		"preco":{"type":"number"},"qtd":{"type":"integer"},
		"total":{"type":"number","x-formula":"preco * qtd"},
		"por":{"type":"string","x-auto":"created_by"},
		"mudou":{"type":"string","x-auto":"updated_by"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(collections.Create(owner, input.CreateCollectionCommand{Name: "Ruim", Schema: json.RawMessage(`{"type":"object","properties":{"t":{"type":"number","x-formula":"a +"}}}`)})), apperror.CodeInvalidInput)
	a, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"sku":"A","preco":2.5,"qtd":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(a.Data, &doc)
	if doc["n"] != float64(1) || doc["total"] != float64(10) || doc["por"] == "" || doc["por"] != doc["mudou"] {
		t.Fatalf("filled: %v", doc)
	}
	b, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"sku":"B","preco":1}`)})
	doc = nil
	_ = json.Unmarshal(b.Data, &doc)
	if doc["n"] != float64(2) || doc["total"] != nil {
		t.Fatalf("second: %v", doc)
	}
	// The highest number in the trash stays taken: the next record does not reuse it.
	if err := records.DeleteMany(owner, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	b2, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"sku":"B2"}`)})
	doc = nil
	_ = json.Unmarshal(b2.Data, &doc)
	if doc["n"] != float64(3) {
		t.Fatalf("after trash: %v", doc)
	}
	// Upsert by sku: A gets a new qtd (its other fields stay), C is new.
	res, err := records.Import(owner, input.ImportRecordsCommand{CollectionID: c.ID, UpsertKey: "sku", Records: []json.RawMessage{json.RawMessage(`{"sku":"A","qtd":10}`), json.RawMessage(`{"sku":"C","preco":3,"qtd":1}`)}})
	if err != nil || res.Created != 1 || res.Updated != 1 || res.Invalid != 0 {
		t.Fatalf("import: %+v, %v", res, err)
	}
	got, _ := records.Get(owner, a.ID)
	doc = nil
	_ = json.Unmarshal(got.Data, &doc)
	if doc["n"] != float64(1) || doc["total"] != float64(25) || doc["preco"] != 2.5 {
		t.Fatalf("upserted: %v", doc)
	}
	wantCode(t, ignore(records.Import(owner, input.ImportRecordsCommand{CollectionID: c.ID, UpsertKey: "preco", Records: []json.RawMessage{json.RawMessage(`{}`)}})), apperror.CodeInvalidInput)

	// Comments: members write, the author or an owner deletes.
	note, err := records.AddComment(owner, a.ID, "  primeiro  ")
	if err != nil || note.Body != "primeiro" {
		t.Fatalf("comment: %+v, %v", note, err)
	}
	wantCode(t, ignore(records.AddComment(owner, a.ID, "")), apperror.CodeInvalidInput)
	list, err := records.Comments(owner, a.ID)
	if err != nil || len(list) != 1 || list[0].AuthorName == "" {
		t.Fatalf("comments: %+v, %v", list, err)
	}
	stranger := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	wantCode(t, ignore(records.Comments(stranger, a.ID)), apperror.CodeNotFound)
	if err := records.DeleteComment(owner, a.ID, note.ID); err != nil {
		t.Fatal(err)
	}
}

// A file field may hold several files; the sweep drops files no record names
// and then their stored bytes, those of a purged collection included.
func TestManyFilesAndSweep(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	_, box := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Fotos", Schema: json.RawMessage(`{"type":"object","properties":{
		"fotos":{"type":"array","x-file":true,"items":{"type":"object","properties":{"id":{"type":"string"}}}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range []string{"a.png", "b.png", "solto.png"} {
		f, err := box.UploadFile(owner, c.ID, name, "image/png", []byte(name))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"fotos":[{"id":"` + ids[0] + `","name":"a.png"},{"id":"` + ids[1] + `","name":"b.png"}]}`)}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"fotos":[{"id":"` + ids[0] + `"},{"id":"` + c.ID + `"}]}`)})), apperror.CodeValidationFailed)
	// The list preview keeps each file's id and name.
	if items, _, err := records.List(owner, domain.RecordFilter{CollectionID: c.ID}); err != nil || len(items) != 1 || !strings.Contains(string(items[0].Data), `"name": "b.png"`) {
		t.Fatalf("preview: %+v, %v", items, err)
	}

	// Fresh files are spared: one may be waiting for its record to be saved.
	if err := box.SweepFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := box.File(owner, ids[2]); err != nil {
		t.Fatalf("fresh file swept: %v", err)
	}
	if err := testDB.Exec(`UPDATE app.file SET created_at = now() - interval '2 days' WHERE collection_id = ?`, c.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := box.SweepFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore2(box.File(owner, ids[2])), apperror.CodeNotFound)
	for _, id := range ids[:2] {
		if _, data, err := box.File(owner, id); err != nil || len(data) == 0 {
			t.Fatalf("named file %s: %v", id, err)
		}
	}

	// A purge cascades the rows away; the next sweep removes the bytes.
	if err := testDB.Exec(`DELETE FROM app.record WHERE collection_id = ?`, c.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`DELETE FROM app.collection WHERE id = ?`, c.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := box.SweepFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	var left int64
	if err := testDB.Raw(`SELECT count(*) FROM app.file_gone WHERE id IN (?, ?, ?)`, ids[0], ids[1], ids[2]).Scan(&left).Error; err != nil || left != 0 {
		t.Fatalf("gone list not emptied: %d, %v", left, err)
	}
	if _, err := box.Files.Get(context.Background(), "files/"+ids[0]); !apperror.IsCode(err, apperror.CodeNotFound) {
		t.Fatalf("bytes kept after purge: %v", err)
	}
}

// A list of objects may hold a reference in each item: saves check every id,
// and the referenced record lists the records whose items point at it.
func TestReferencesInListItems(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	wireds, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Wireds", Schema: json.RawMessage(`{"type":"object","properties":{"nome":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	w, err := records.Create(owner, input.CreateRecordCommand{CollectionID: wireds.ID, Data: json.RawMessage(`{"nome":"Ao andar"}`)})
	if err != nil {
		t.Fatal(err)
	}
	examples, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Exemplos", Schema: json.RawMessage(`{"type":"object","properties":{
		"titulo":{"type":"string"},
		"passos":{"type":"array","items":{"type":"object","properties":{
			"wired":{"type":"string","format":"uuid","x-ref":"` + wireds.ID + `"},"config":{"type":"string"}}}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(owner, input.CreateRecordCommand{CollectionID: examples.ID, Data: json.RawMessage(`{"passos":[{"wired":"` + w.ID + `"},{"wired":"` + examples.ID + `"}]}`)})), apperror.CodeValidationFailed)
	ex, err := records.Create(owner, input.CreateRecordCommand{CollectionID: examples.ID, Data: json.RawMessage(`{"titulo":"Porta","passos":[{"wired":"` + w.ID + `","config":"1 passo"},{"config":"sem wired"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	links, err := records.Links(owner, w.ID)
	if err != nil || len(links) != 1 || links[0].Field != "passos" || links[0].Sub != "wired" || links[0].Total != 1 || links[0].Records[0].ID != ex.ID {
		t.Fatalf("links: %+v, %v", links, err)
	}
}
