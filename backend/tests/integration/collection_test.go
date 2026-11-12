package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	collectionrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/collection"
	recordrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/record"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
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
	if _, err := records.Update(ctx, input.UpdateRecordCommand{ID: clicked.ID, Data: json.RawMessage(`{"categoria":"gatilho"}`)}); !apperror.IsCode(err, apperror.CodeValidationFailed) {
		t.Fatalf("update must follow the schema: %v", err)
	}

	list, _, err := collections.List(ctx, domain.CollectionFilter{})
	if err != nil || len(list) != 1 || list[0].RecordCount != 2 || list[0].Role != domain.RoleOwner {
		t.Fatalf("list with counts: %+v, %v", list, err)
	}

	// A collection with records cannot be deleted; once empty it can.
	if err := collections.DeleteMany(ctx, []string{wired.ID}); !apperror.IsCode(err, apperror.CodeResourceInUse) {
		t.Fatalf("delete with records: %v", err)
	}
	ids := []string{}
	for _, r := range mustList(ctx, t, records, wired.ID) {
		ids = append(ids, r.ID)
	}
	if err := records.DeleteMany(ctx, ids); err != nil {
		t.Fatal(err)
	}
	if err := collections.DeleteMany(ctx, []string{wired.ID}); err != nil {
		t.Fatalf("delete empty: %v", err)
	}
}

func collectionServices(e *env) (*collection.Service, *record.Service) {
	repo := collectionrepo.NewRepository(testDB)
	return collection.New(repo, e.store, e.svc), record.New(recordrepo.NewRepository(testDB), repo, e.store)
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
	if list, _, _ := collections.List(outsider, domain.CollectionFilter{}); len(list) != 1 || list[0].ID == c.ID {
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
		WITH ro AS (INSERT INTO usr.usr_role (name) VALUES ('full-` + adminID[:8] + `') RETURNING id),
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
