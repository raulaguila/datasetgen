package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	auditrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/audit"
	collectionrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/collection"
	recordrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/record"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	auditsvc "github.com/raulaguila/dataset/internal/core/usecase/audit"
	"github.com/raulaguila/dataset/internal/core/usecase/collection"
	"github.com/raulaguila/dataset/internal/core/usecase/record"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

const wiredSchema = `{
  "type": "object",
  "required": ["nome", "categoria"],
  "properties": {
    "nome": {"type": "string", "minLength": 1},
    "categoria": {"enum": ["gatilho", "efeito", "condicao"]}
  }
}`

func TestCollectionRecordsFollowTheSchema(t *testing.T) {
	if err := testDB.Exec("DELETE FROM app.record; DELETE FROM app.collection").Error; err != nil {
		t.Fatal(err)
	}
	e := newEnv(t)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	collections, records := collectionServices(e)

	wired, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "Wired", Schema: json.RawMessage(wiredSchema)})
	if err != nil {
		t.Fatal(err)
	}
	// The schema comes back as written: field order drives the generated form.
	if got, err := collections.Get(ctx, wired.ID); err != nil {
		t.Fatal(err)
	} else if string(got.Schema) != wiredSchema {
		t.Fatalf("schema changed on the way back: %s", got.Schema)
	}

	// Invalid data is refused with one issue per broken rule.
	_, err = records.Create(ctx, input.CreateRecordCommand{CollectionID: wired.ID, Data: json.RawMessage(`{"nome":"","categoria":"outro"}`)})
	if !apperror.IsCode(err, apperror.CodeValidationFailed) {
		t.Fatalf("invalid record: %v", err)
	}

	clicked, err := records.Create(ctx, input.CreateRecordCommand{CollectionID: wired.ID, Data: json.RawMessage(`{"nome":"Ao clicar","categoria":"gatilho"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(ctx, input.CreateRecordCommand{CollectionID: wired.ID, Data: json.RawMessage(`{"nome":"Teleportar","categoria":"efeito","descricao":"` + strings.Repeat("a", 200) + `","exemplos":[{"cenario":"x"}]}`)}); err != nil {
		t.Fatal(err)
	}

	items, total, err := records.List(ctx, domain.RecordFilter{CollectionID: wired.ID, Search: "teleport"})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("search: %v of %d, %v", items, total, err)
	}
	// The list carries a preview: plain values only, long text cut.
	if want := `{"nome": "Teleportar", "categoria": "efeito", "descricao": "` + strings.Repeat("a", 120) + `"}`; string(items[0].Data) != want {
		t.Fatalf("preview %s", items[0].Data)
	}

	// An edit based on an old version is a conflict; the current one lands.
	updated, err := records.Update(ctx, input.UpdateRecordCommand{ID: clicked.ID, Data: json.RawMessage(`{"nome":"Ao clicar no mobi","categoria":"gatilho"}`), Version: pointer.Of(1)})
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if _, err := records.Update(ctx, input.UpdateRecordCommand{ID: clicked.ID, Data: json.RawMessage(`{"nome":"x","categoria":"gatilho"}`), Version: pointer.Of(1)}); !apperror.IsCode(err, apperror.CodeConflict) {
		t.Fatalf("stale update: %v", err)
	}
	// By last change, the record just edited comes first though it was created first.
	if items, _, err := records.List(ctx, domain.RecordFilter{CollectionID: wired.ID, Sort: "updated_at"}); err != nil || len(items) != 2 || items[0].ID != clicked.ID {
		t.Fatalf("by updated_at: %v, %v", items, err)
	}
	wantCode(t, ignore2(records.List(ctx, domain.RecordFilter{CollectionID: wired.ID, Sort: "data"})), apperror.CodeInvalidInput)
	if _, err := records.Update(ctx, input.UpdateRecordCommand{ID: clicked.ID, Data: json.RawMessage(`{"categoria":"gatilho"}`)}); !apperror.IsCode(err, apperror.CodeValidationFailed) {
		t.Fatalf("update must follow the schema: %v", err)
	}

	list, _, err := collections.List(ctx, domain.CollectionFilter{Search: "Wired"})
	if err != nil || len(list) != 1 || list[0].RecordCount != 2 || list[0].Role != domain.RoleOwner {
		t.Fatalf("list with counts: %+v, %v", list, err)
	}

	// Deleting moves the collection, records and all, to the trash: it is gone from every read.
	if err := collections.DeleteMany(ctx, []string{wired.ID}); err != nil {
		t.Fatalf("delete with records: %v", err)
	}
	wantCode(t, ignore(collections.Get(ctx, wired.ID)), apperror.CodeNotFound)
	if list, _, err := collections.List(ctx, domain.CollectionFilter{Search: "Wired"}); err != nil || len(list) != 0 {
		t.Fatalf("a trashed collection must not list: %+v, %v", list, err)
	}
}

func collectionServices(e *env) (*collection.Service, *record.Service) {
	repo := collectionrepo.NewRepository(testDB)
	return collection.New(repo, recordrepo.NewRepository(testDB), e.store, e.svc, auditsvc.New(auditrepo.NewRepository(testDB))), record.New(recordrepo.NewRepository(testDB), repo, e.store)
}

// Each collection is private to its members: the role decides what they may do,
// an invited email joins at once, and the last owner cannot leave.
func TestCollectionMembersDecideAccess(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	as := func(addr string) context.Context { return reqctx.WithActor(context.Background(), e.userID(t, addr)) }
	ownerAddr, helperAddr, outsiderAddr := e.activeUser(t), e.activeUser(t), e.activeUser(t)
	owner, helper, outsider := as(ownerAddr), as(helperAddr), as(outsiderAddr)

	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Privada", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// Someone else may use the same name: names are no longer global.
	if _, err := collections.Create(outsider, input.CreateCollectionCommand{Name: "Privada", Schema: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("same name for another person: %v", err)
	}

	// A non-member neither sees nor reaches it.
	wantCode(t, ignore(collections.Get(outsider, c.ID)), apperror.CodeNotFound)
	if list, _, _ := collections.List(outsider, domain.CollectionFilter{Search: "Privada"}); len(list) != 1 || list[0].ID == c.ID {
		t.Fatalf("outsider sees %+v", list)
	}
	_, _, err = records.List(outsider, domain.RecordFilter{CollectionID: c.ID})
	wantCode(t, err, apperror.CodeNotFound)

	// A viewer reads but does not write; an editor writes records but not the collection.
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: helperAddr, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	if got, err := collections.Get(helper, c.ID); err != nil || got.Role != domain.RoleViewer {
		t.Fatalf("viewer get: %+v, %v", got, err)
	}
	wantCode(t, ignore(records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)})), apperror.CodeForbidden)
	helperID := e.userID(t, helperAddr)
	wantCode(t, collections.SetMemberRole(helper, c.ID, helperID, domain.RoleOwner), apperror.CodeForbidden)
	if err := collections.SetMemberRole(owner, c.ID, helperID, domain.RoleEditor); err != nil {
		t.Fatal(err)
	}
	r, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatalf("editor create: %v", err)
	}
	wantCode(t, ignore(records.Get(outsider, r.ID)), apperror.CodeNotFound)
	wantCode(t, records.DeleteMany(outsider, []string{r.ID}), apperror.CodeNotFound)
	wantCode(t, ignore(collections.Update(helper, input.UpdateCollectionCommand{ID: c.ID, Name: pointer.Of("x")})), apperror.CodeForbidden)
	wantCode(t, ignore(collections.Members(helper, c.ID)), apperror.CodeForbidden)

	// An email without an account is invited and is a member right away.
	newAddr := email()
	wantCode(t, ignore(collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: newAddr, Role: domain.RoleViewer})), apperror.CodeInvalidInput)
	added, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: newAddr, Name: "Nova", Role: domain.RoleViewer})
	if err != nil || !added.Invited || added.ActivationToken == "" || added.Member.Status != domain.UserStatusPending {
		t.Fatalf("invite: %+v, %v", added, err)
	}
	wantCode(t, ignore(collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: newAddr, Role: domain.RoleViewer})), apperror.CodeAlreadyExists)
	if err := e.svc.Activate(context.Background(), input.ActivateCommand{Token: added.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	if _, err := collections.Get(as(newAddr), c.ID); err != nil {
		t.Fatalf("invited member after activation: %v", err)
	}

	// The last owner can neither leave nor step down.
	ownerID := e.userID(t, ownerAddr)
	wantCode(t, collections.RemoveMember(owner, c.ID, ownerID), apperror.CodeConflict)
	wantCode(t, collections.SetMemberRole(owner, c.ID, ownerID, domain.RoleEditor), apperror.CodeConflict)
	members, err := collections.Members(owner, c.ID)
	if err != nil || len(members) != 3 || members[0].UserID != ownerID {
		t.Fatalf("members: %+v, %v", members, err)
	}
	if err := collections.RemoveMember(owner, c.ID, helperID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(collections.Get(helper, c.ID)), apperror.CodeNotFound)

	// A platform admin (here: a role granting '*') sees and owns every collection.
	adminID := e.userID(t, outsiderAddr)
	if err := testDB.Exec(`
		WITH ro AS (INSERT INTO usr.usr_role (name) VALUES ('full-`+adminID[:8]+`') RETURNING id),
		     p AS (INSERT INTO usr.usr_role_permission (role_id, resource, actions) SELECT id, '*', 15 FROM ro)
		INSERT INTO usr.usr_user_role (user_id, role_id) SELECT ?, id FROM ro`, adminID).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := collections.Get(outsider, c.ID); err != nil || got.Role != domain.RoleOwner {
		t.Fatalf("admin get: %+v, %v", got, err)
	}
	if list, _, err := collections.List(outsider, domain.CollectionFilter{Search: "Privada"}); err != nil || len(list) != 2 {
		t.Fatalf("admin list: %+v, %v", list, err)
	}
}

func ignore[T any](_ T, err error) error { return err }

func mustList(ctx context.Context, t *testing.T, svc *record.Service, collectionID string) []domain.Record {
	t.Helper()
	items, _, err := svc.List(ctx, domain.RecordFilter{CollectionID: collectionID, Page: domain.NewPage(1, 100)})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// Monitoring: the collection's own audit log, and the records a schema change left out of format.
func TestCollectionAuditAndQuality(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	ownerAddr, helperAddr := e.activeUser(t), e.activeUser(t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	helper := reqctx.WithActor(context.Background(), e.userID(t, helperAddr))

	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Monitorada", Schema: json.RawMessage(`{"type":"object","properties":{"nome":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	other, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Outra", Schema: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: helperAddr, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	a, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"nome":"a"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: other.ID, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if err := records.DeleteMany(helper, []string{a.ID}); err != nil {
		t.Fatal(err)
	}

	// The log holds this collection's create, member, record creates and delete; not the other one's.
	log, total, err := collections.AuditLog(owner, c.ID, domain.AuditFilter{Page: domain.NewPage(1, 50)})
	if err != nil || total != 5 {
		t.Fatalf("audit: %d entries, %v: %+v", total, err, log)
	}
	// The deleted record still reads by its title and says it is gone.
	if log[0].Action != "record.delete" || log[0].TargetLabel != "a" || !log[0].TargetGone {
		t.Fatalf("newest first, labelled and gone: %+v", log[0])
	}
	if _, _, err := collections.AuditLog(owner, c.ID, domain.AuditFilter{Action: "record.", Page: domain.NewPage(1, 50)}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore2(collections.AuditLog(helper, c.ID, domain.AuditFilter{})), apperror.CodeForbidden)

	// Making "nome" required leaves the {} record out of format.
	if _, err := collections.Update(owner, input.UpdateCollectionCommand{ID: c.ID, Schema: json.RawMessage(`{"type":"object","required":["nome"],"properties":{"nome":{"type":"string"}}}`)}); err != nil {
		t.Fatal(err)
	}
	report := runQuality(helper, t, collections, c.ID)
	var invalid []input.InvalidRecord
	if err := json.Unmarshal(report.Invalid, &invalid); err != nil || report.Checked != 1 || report.InvalidCount != 1 ||
		len(invalid) != 1 || invalid[0].Issues[0].Keyword != "required" {
		t.Fatalf("quality: %+v, %v", report, err)
	}
}

// runQuality starts a check and runs its job the way the worker would.
func runQuality(ctx context.Context, t *testing.T, collections *collection.Service, id string) *domain.QualityRun {
	t.Helper()
	st, err := collections.StartQuality(ctx, id)
	if err != nil || st.Run == nil || st.Run.Status != domain.QualityRunning {
		t.Fatalf("start quality: %+v, %v", st, err)
	}
	payload, _ := json.Marshal(map[string]string{"id": id})
	if err := collection.Jobs(collections.Repo, collections.Records)[collection.JobQuality](context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	st, err = collections.Quality(ctx, id)
	if err != nil || st.Run.Status != domain.QualityDone {
		t.Fatalf("quality after job: %+v, %v", st, err)
	}
	return st.Run
}

// Overview stats, a member leaving and the curated icon.
func TestCollectionStatsLeaveAndIcon(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	ownerAddr, helperAddr := e.activeUser(t), e.activeUser(t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	helper := reqctx.WithActor(context.Background(), e.userID(t, helperAddr))

	wantCode(t, ignore(collections.Create(owner, input.CreateCollectionCommand{Name: "X", Icon: "nope", Schema: json.RawMessage(`{}`)})), apperror.CodeInvalidInput)
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Painel", Icon: "sparkles", Schema: json.RawMessage(`{}`)})
	if err != nil || c.Icon != "sparkles" {
		t.Fatalf("create with icon: %+v, %v", c, err)
	}
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: helperAddr, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	r, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Update(helper, input.UpdateRecordCommand{ID: r.ID, Data: json.RawMessage(`{"a":2}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}

	wantCode(t, ignore(collections.Stats(helper, c.ID, "Mars/Base")), apperror.CodeInvalidInput)
	st, err := collections.Stats(helper, c.ID, "America/Manaus")
	if err != nil || st.Records != 2 || st.Members != 2 || st.LastUpdate == nil || len(st.PerDay) != 1 || st.PerDay[0].Count != 2 {
		t.Fatalf("stats: %+v, %v", st, err)
	}
	// The list adds the aggregates only when asked.
	plain, _, err := collections.List(helper, domain.CollectionFilter{Page: domain.NewPage(1, 10), Search: "Painel"})
	if err != nil || len(plain) != 1 || plain[0].MemberCount != 0 || !plain[0].LastActivity.IsZero() {
		t.Fatalf("plain list: %+v, %v", plain, err)
	}
	full, _, err := collections.List(helper, domain.CollectionFilter{Page: domain.NewPage(1, 10), Search: "Painel", Details: true})
	if err != nil || len(full) != 1 || full[0].MemberCount != 2 || full[0].FieldCount != 0 || full[0].LastActivity.Before(*st.LastUpdate) {
		t.Fatalf("detailed list: %+v, %v", full, err)
	}
	if len(st.Contributors) != 2 || st.Contributors[0].Created != 1 || st.Contributors[0].Edited != 1 {
		t.Fatalf("contributors: %+v", st.Contributors)
	}

	// The helper leaves and loses access; the last owner cannot leave.
	if list, _, _ := collections.List(helper, domain.CollectionFilter{Search: "Painel"}); len(list) != 1 || !list[0].Member {
		t.Fatalf("member flag: %+v", list)
	}
	if err := collections.Leave(helper, c.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(collections.Get(helper, c.ID)), apperror.CodeNotFound)
	wantCode(t, collections.Leave(owner, c.ID), apperror.CodeConflict)
}

func ignore2[A, B any](_ A, _ B, err error) error { return err }

// toolsEnv is a collection with a required "nome" and an enum "cat", owned by
// owner, with helper as editor.
func toolsEnv(t *testing.T) (collections *collection.Service, records *record.Service, c *domain.Collection, owner, helper context.Context) {
	t.Helper()
	e := newEnv(t)
	collections, records = collectionServices(e)
	ownerAddr, helperAddr := e.activeUser(t), e.activeUser(t)
	owner = reqctx.WithActor(context.Background(), e.userID(t, ownerAddr))
	helperID := e.userID(t, helperAddr)
	helper = reqctx.WithActor(context.Background(), helperID)
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Tools", Schema: json.RawMessage(
		`{"type":"object","required":["nome"],"properties":{"nome":{"type":"string"},"cat":{"enum":["a","b"]}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: c.ID, Email: helperAddr, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	var notices int64
	testDB.Raw(`SELECT count(*) FROM usr.usr_notification WHERE user_id = ? AND kind = 'collection_added'`, helperID).Scan(&notices)
	if notices != 1 {
		t.Fatalf("member notice: %d", notices)
	}
	return collections, records, c, owner, helper
}

// Import checks before writing; filters match exact values; an edit keeps a
// version and restoring it adds one more.
func TestRecordImportFiltersAndVersions(t *testing.T) {
	_, records, c, _, helper := toolsEnv(t)
	batch := []json.RawMessage{json.RawMessage(`{"nome":"x","cat":"a"}`), json.RawMessage(`{"nome":"y","cat":"b"}`), json.RawMessage(`{"cat":"a"}`)}
	res, err := records.Import(helper, input.ImportRecordsCommand{CollectionID: c.ID, Records: batch, DryRun: true})
	if err != nil || res.Created != 0 || res.Invalid != 1 || res.Errors[0].Index != 2 {
		t.Fatalf("dry run: %+v, %v", res, err)
	}
	if res, err = records.Import(helper, input.ImportRecordsCommand{CollectionID: c.ID, Records: batch}); err != nil || res.Created != 0 {
		t.Fatalf("all or nothing: %+v, %v", res, err)
	}
	if res, err = records.Import(helper, input.ImportRecordsCommand{CollectionID: c.ID, Records: batch, SkipInvalid: true}); err != nil || res.Created != 2 {
		t.Fatalf("skip invalid: %+v, %v", res, err)
	}

	page := domain.NewPage(1, 10)
	list, total, err := records.List(helper, domain.RecordFilter{CollectionID: c.ID, Page: page, Match: json.RawMessage(`{"cat":"a"}`)})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("filter: %d %v", total, err)
	}
	wantCode(t, ignore2(records.List(helper, domain.RecordFilter{CollectionID: c.ID, Page: page, Match: json.RawMessage(`{"cat":["a"]}`)})), apperror.CodeInvalidInput)

	// An edit keeps the old data as a version, and restoring it is one more version.
	id := list[0].ID
	if _, err := records.Update(helper, input.UpdateRecordCommand{ID: id, Data: json.RawMessage(`{"nome":"changed","cat":"a"}`)}); err != nil {
		t.Fatal(err)
	}
	versions, err := records.Versions(helper, id)
	if err != nil || len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions: %+v, %v", versions, err)
	}
	two := 2
	restored, err := records.Restore(helper, input.RestoreRecordCommand{ID: id, Version: 1, CurrentVersion: &two})
	if err != nil || restored.Version != 3 || !strings.Contains(string(restored.Data), `"x"`) {
		t.Fatalf("restore: %+v, %v", restored, err)
	}
	wantCode(t, ignore(records.Restore(helper, input.RestoreRecordCommand{ID: id, Version: 1, CurrentVersion: &two})), apperror.CodeConflict)

	if n, err := exported(records, helper, c.ID, false); err != nil || n != 2 {
		t.Fatalf("export: %d, %v", n, err)
	}

}

// Bulk fixes, valid-only export, deleting what the check found and duplicating.
func TestRecordFixesAndDuplicate(t *testing.T) {
	collections, records, c, owner, helper := toolsEnv(t)
	batch := []json.RawMessage{json.RawMessage(`{"nome":"x","cat":"a"}`), json.RawMessage(`{"nome":"y","cat":"b"}`)}
	if _, err := records.Import(helper, input.ImportRecordsCommand{CollectionID: c.ID, Records: batch}); err != nil {
		t.Fatal(err)
	}
	// Renaming the required field leaves both records invalid until renamed back.
	// A dry run counts and changes nothing: the rename below still finds both.
	if n, err := collections.Fix(helper, c.ID, domain.RecordFix{Op: domain.FixRename, Field: "nome", To: "titulo", DryRun: true}); err != nil || n != 2 {
		t.Fatalf("dry run = %d, %v; want 2", n, err)
	}
	if n, err := collections.Fix(helper, c.ID, domain.RecordFix{Op: domain.FixRename, Field: "nome", To: "titulo"}); err != nil || n != 2 {
		t.Fatalf("rename: %d, %v", n, err)
	}
	if n, err := collections.Fix(helper, c.ID, domain.RecordFix{Op: domain.FixSetDefault, Field: "nome", Value: json.RawMessage(`"padrão"`)}); err != nil || n != 2 {
		t.Fatalf("set default: %d, %v", n, err)
	}
	if n, err := collections.Fix(helper, c.ID, domain.RecordFix{Op: domain.FixRemove, Field: "nome"}); err != nil || n != 2 {
		t.Fatalf("remove: %d, %v", n, err)
	}
	wantCode(t, ignore(collections.Fix(helper, c.ID, domain.RecordFix{Op: "drop", Field: "x"})), apperror.CodeInvalidInput)
	if n, err := exported(records, helper, c.ID, true); err != nil || n != 0 {
		t.Fatalf("valid export: %d, %v", n, err)
	}

	wantCode(t, ignore(collections.DeleteInvalid(helper, c.ID)), apperror.CodeConflict) // no check yet
	if run := runQuality(helper, t, collections, c.ID); run.InvalidCount != 2 {
		t.Fatalf("quality: %+v", run)
	}
	if n, err := collections.DeleteInvalid(helper, c.ID); err != nil || n != 2 {
		t.Fatalf("delete invalid: %d, %v", n, err)
	}

	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"nome":"z"}`)}); err != nil {
		t.Fatal(err)
	}
	dup, err := collections.Duplicate(helper, input.DuplicateCollectionCommand{ID: c.ID, Records: true})
	if err != nil || dup.Name != "Tools (copy)" || dup.RecordCount != 1 || dup.Role != domain.RoleOwner {
		t.Fatalf("duplicate: %+v, %v", dup, err)
	}
}

// A field marked unique refuses a value another record holds, on every way a
// record is written, and cannot be turned on while records repeat a value.
func TestUniqueFieldsHold(t *testing.T) {
	e := newEnv(t)
	ctx := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	collections, records := collectionServices(e)
	c, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "Unique", Schema: json.RawMessage(
		`{"type":"object","properties":{"codigo":{"type":"string","x-unique":true},"nome":{"type":"string"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	create := func(data string) (*domain.Record, error) {
		return records.Create(ctx, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(data)})
	}
	a, err := create(`{"codigo":"a1","nome":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = create(`{"codigo":"a1"}`)
	var ae *apperror.Error
	if !apperror.IsCode(err, apperror.CodeValidationFailed) || !errors.As(err, &ae) || !strings.Contains(fmt.Sprint(ae.Details["issues"]), "unique") {
		t.Fatalf("repeated value: %v", err)
	}
	// Missing is not a value: two records without one are fine.
	if _, err := create(`{"nome":"x"}`); err != nil {
		t.Fatal(err)
	}
	b, err := create(`{"codigo":"a2","nome":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, ignore(records.Update(ctx, input.UpdateRecordCommand{ID: b.ID, Data: json.RawMessage(`{"codigo":"a1"}`)})), apperror.CodeValidationFailed)
	// A record keeps its own value on edit.
	if _, err := records.Update(ctx, input.UpdateRecordCommand{ID: a.ID, Data: json.RawMessage(`{"codigo":"a1","nome":"z"}`)}); err != nil {
		t.Fatal(err)
	}

	// An import refuses repeats of stored values and within the batch.
	batch := []json.RawMessage{json.RawMessage(`{"codigo":"a3"}`), json.RawMessage(`{"codigo":"a3"}`), json.RawMessage(`{"codigo":"a1"}`)}
	res, err := records.Import(ctx, input.ImportRecordsCommand{CollectionID: c.ID, Records: batch, SkipInvalid: true})
	if err != nil || res.Created != 1 || res.Invalid != 2 {
		t.Fatalf("import: %+v, %v", res, err)
	}

	// Filling one value into many records would repeat it.
	wantCode(t, ignore(collections.Fix(ctx, c.ID, domain.RecordFix{Op: domain.FixSetDefault, Field: "codigo", Value: json.RawMessage(`"z"`)})), apperror.CodeInvalidInput)

	// "nome" repeats "x" twice: it cannot become unique until that is fixed.
	got, err := collections.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	unique := json.RawMessage(`{"type":"object","properties":{"codigo":{"type":"string","x-unique":true},"nome":{"type":"string","x-unique":true}}}`)
	wantCode(t, ignore(collections.Update(ctx, input.UpdateCollectionCommand{ID: c.ID, Schema: unique, Version: pointer.Of(got.Version)})), apperror.CodeInvalidInput)
}

// exported counts the records an export streams.
func exported(records input.RecordService, ctx context.Context, collectionID string, valid bool) (int, error) {
	exp, err := records.Export(ctx, collectionID, valid)
	if err != nil {
		return 0, err
	}
	n := 0
	err = exp.Each(func(json.RawMessage) error { n++; return nil })
	return n, err
}

// A record keeps its latest 100 earlier versions; older ones are dropped.
func TestRecordVersionsArePruned(t *testing.T) {
	_, records, c, _, helper := toolsEnv(t)
	rec, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"nome":"v1","cat":"a"}`)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 103; i++ {
		if _, err := records.Update(helper, input.UpdateRecordCommand{ID: rec.ID, Data: json.RawMessage(fmt.Sprintf(`{"nome":"v%d","cat":"a"}`, i))}); err != nil {
			t.Fatal(err)
		}
	}
	// Version 103 is current; 3 to 102 stay.
	wantCode(t, ignore(records.Restore(helper, input.RestoreRecordCommand{ID: rec.ID, Version: 2})), apperror.CodeNotFound)
	if _, err := records.Restore(helper, input.RestoreRecordCommand{ID: rec.ID, Version: 3}); err != nil {
		t.Fatalf("version 3: %v", err)
	}
}

// A fix made while a check runs starts a new one; the older run's result,
// read before the fix, is dropped instead of overwriting the newer.
func TestFixRestartsTheQualityCheck(t *testing.T) {
	collections, records, c, _, helper := toolsEnv(t)
	if _, err := records.Create(helper, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(`{"nome":"x","cat":"a"}`)}); err != nil {
		t.Fatal(err)
	}
	token := func() string {
		t.Helper()
		var s string
		if err := testDB.Raw(`SELECT started_at::text FROM app.collection_quality WHERE collection_id = ?`, c.ID).Scan(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	run := func(tok string) {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{"id": c.ID, "token": tok})
		if err := collection.Jobs(collections.Repo, collections.Records)[collection.JobQuality](context.Background(), payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := collections.StartQuality(helper, c.ID); err != nil {
		t.Fatal(err)
	}
	old := token()
	// The required field goes: the record breaks the schema.
	if n, err := collections.Fix(helper, c.ID, domain.RecordFix{Op: domain.FixRemove, Field: "nome"}); err != nil || n != 1 {
		t.Fatalf("fix: %d, %v", n, err)
	}
	fresh := token()
	if fresh == old {
		t.Fatal("the fix did not start a new check")
	}
	run(old)
	if st, err := collections.Quality(helper, c.ID); err != nil || st.Run.Status != domain.QualityRunning {
		t.Fatalf("the older run was stored: %+v, %v", st.Run, err)
	}
	run(fresh)
	if st, err := collections.Quality(helper, c.ID); err != nil || st.Run.Status != domain.QualityDone || st.Run.InvalidCount != 1 {
		t.Fatalf("after the new run: %+v, %v", st.Run, err)
	}
}
