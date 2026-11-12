package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"
	collectionrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/collection"
	recordrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/record"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/collection"
	"github.com/raulaguila/dataset/internal/core/usecase/record"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
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
	ctx := context.Background()
	store := postgres.NewStore(testDB)
	collections := collection.New(collectionrepo.NewRepository(testDB), store)
	records := record.New(recordrepo.NewRepository(testDB), store)

	wired, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "Wired", Schema: json.RawMessage(wiredSchema)})
	if err != nil {
		t.Fatal(err)
	}
	// The schema comes back as written: field order drives the generated form.
	if got, err := collections.Get(ctx, wired.ID); err != nil || string(got.Schema) != wiredSchema {
		t.Fatalf("schema changed on the way back: %s, %v", got.Schema, err)
	}
	if _, err := collections.Create(ctx, input.CreateCollectionCommand{Name: "Wired", Schema: json.RawMessage(`{}`)}); !apperror.IsCode(err, apperror.CodeAlreadyExists) {
		t.Fatalf("duplicate name: %v", err)
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
	if err != nil || len(list) != 1 || list[0].RecordCount != 2 {
		t.Fatalf("list with counts: %+v, %v", list, err)
	}

	// A collection with records cannot be deleted; once empty it can.
	if err := collections.DeleteMany(ctx, []string{wired.ID}); !apperror.IsCode(err, apperror.CodeResourceInUse) {
		t.Fatalf("delete with records: %v", err)
	}
	ids := []string{}
	for _, r := range mustList(t, records, wired.ID) {
		ids = append(ids, r.ID)
	}
	if err := records.DeleteMany(ctx, ids); err != nil {
		t.Fatal(err)
	}
	if err := collections.DeleteMany(ctx, []string{wired.ID}); err != nil {
		t.Fatalf("delete empty: %v", err)
	}
}

func mustList(t *testing.T, svc *record.Service, collectionID string) []domain.Record {
	t.Helper()
	items, _, err := svc.List(context.Background(), domain.RecordFilter{CollectionID: collectionID, Page: domain.NewPage(1, 100)})
	if err != nil {
		t.Fatal(err)
	}
	return items
}
