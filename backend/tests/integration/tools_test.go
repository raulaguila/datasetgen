package integration_test

import (
	"context"
	"encoding/json"
	"strings"
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
	_, records := collectionServices(e)
	return &integration.Service{Store: e.store, Tx: e.store, Send: webhook.New(true), SealKey: key, Records: records},
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
	}
	for _, tc := range cases {
		if got := count(tc.w); got != tc.want {
			t.Errorf("%s: %d records, want %d", tc.name, got, tc.want)
		}
	}
	wantCode(t, ignore2(records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Rules: []domain.Condition{rule("v", "drop table", "1")}}})), apperror.CodeInvalidInput)
	wantCode(t, ignore2(records.List(owner, domain.RecordFilter{CollectionID: c.ID, Where: &domain.Where{Rules: []domain.Condition{rule("v", "in", "5")}}})), apperror.CodeInvalidInput)

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
	if err != nil || len(total) != 1 || total[0].Count != 2 || total[0].Avgs["v"] != 7.5 {
		t.Fatalf("totals: %+v, %v", total, err)
	}
}

// A share link opens one collection to anyone holding it: a form adds records,
// a list reads them; neither is an API key, and deleting the link ends it.
func TestShareLinks(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Links", Schema: json.RawMessage(`{"type":"object","required":["n"],"properties":{"n":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	form, err := keys.CreateShare(owner, c.ID, domain.ShareForm)
	if err != nil {
		t.Fatal(err)
	}
	view, err := keys.CreateShare(owner, c.ID, domain.ShareView)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(keys.CreateShare(owner, c.ID, "edit")), apperror.CodeInvalidInput)
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
		return reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: g.KeyID, Name: g.Name, CollectionID: g.CollectionID, CanWrite: g.CanWrite})
	}
	// The form's record follows the fields; the list cannot write.
	wantCode(t, ignore(records.Create(as(form.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)})), apperror.CodeValidationFailed)
	if _, err := records.Create(as(form.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"n":"x"}`)}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Create(as(view.Token), input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"n":"y"}`)})), apperror.CodeForbidden)
	if items, total, err := records.List(as(view.Token), domain.RecordFilter{CollectionID: c.ID, Page: domain.NewPage(1, 10)}); err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list: %d, %v", total, err)
	}
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
