package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// Lookups show a field of the referenced record, rollups sum up the records
// pointing here; both are computed on read and never stored.
func TestLookupsAndRollupsComputeOnRead(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	mk := func(name, schema string) string {
		c, err := collections.Create(owner, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(schema)})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	clients := mk("Clientes", `{"type":"object","properties":{"nome":{"type":"string"}}}`)
	orders := mk("Pedidos", `{"type":"object","properties":{
		"cliente":{"type":"string","x-ref":"`+clients+`"},
		"total":{"type":"number"},
		"cliente_nome":{"x-lookup":{"ref":"cliente","field":"nome"}}}}`)
	// A lookup must go through a reference field of the same schema.
	wantCode(t, ignore(collections.Create(owner, input.CreateCollectionCommand{Name: "Ruim", Schema: json.RawMessage(`{"type":"object","properties":{"x":{"x-lookup":{"ref":"nada","field":"nome"}}}}`)})), apperror.CodeInvalidInput)
	wantCode(t, ignore(collections.Create(owner, input.CreateCollectionCommand{Name: "Ruim", Schema: json.RawMessage(`{"type":"object","properties":{"x":{"x-rollup":{"collection":"` + orders + `","ref":"cliente","fn":"median","field":"total"}}}}`)})), apperror.CodeInvalidInput)
	if _, err := collections.Update(owner, input.UpdateCollectionCommand{ID: clients, Schema: json.RawMessage(`{"type":"object","properties":{
		"nome":{"type":"string"},
		"gasto":{"x-rollup":{"collection":"` + orders + `","ref":"cliente","fn":"sum","field":"total"}},
		"pedidos":{"x-rollup":{"collection":"` + orders + `","ref":"cliente","fn":"count"}}}}`)}); err != nil {
		t.Fatal(err)
	}
	ana, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: clients, Data: json.RawMessage(`{"nome":"Ana"}`)})
	bia, _ := records.Create(owner, input.CreateRecordCommand{CollectionID: clients, Data: json.RawMessage(`{"nome":"Bia"}`)})
	var first string
	for _, total := range []string{"10", "2.5"} {
		r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: orders, Data: json.RawMessage(`{"cliente":"` + ana.ID + `","total":` + total + `,"cliente_nome":"forjado"}`)})
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = r.ID
		}
	}
	var stored string
	testDB.Raw(`SELECT data::text FROM app.record WHERE id = ?`, first).Scan(&stored)
	if stored == "" || strings.Contains(stored, "cliente_nome") {
		t.Fatalf("a lookup was stored: %s", stored)
	}
	got, err := records.Get(owner, first)
	if err != nil {
		t.Fatal(err)
	}
	var order map[string]any
	_ = json.Unmarshal(got.Data, &order)
	if order["cliente_nome"] != "Ana" {
		t.Fatalf("lookup: %s", got.Data)
	}
	list, _, err := records.List(owner, domain.RecordFilter{CollectionID: clients, Page: domain.NewPage(1, 10)})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		var d map[string]any
		_ = json.Unmarshal(c.Data, &d)
		switch c.ID {
		case ana.ID:
			if d["gasto"] != 12.5 || d["pedidos"] != float64(2) {
				t.Fatalf("ana: %s", c.Data)
			}
		case bia.ID:
			if _, has := d["gasto"]; has || d["pedidos"] != float64(0) {
				t.Fatalf("bia: %s", c.Data)
			}
		}
	}
	// Someone who cannot read Pedidos sees Clientes without its rollups.
	viewerAddr := e.activeUser(t)
	if _, err := collections.AddMember(owner, input.AddMemberCommand{CollectionID: clients, Email: viewerAddr, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewer := reqctx.WithActor(context.Background(), e.userID(t, viewerAddr))
	seen, err := records.Get(viewer, ana.ID)
	if err != nil || strings.Contains(string(seen.Data), "gasto") {
		t.Fatalf("viewer: %s, %v", seen.Data, err)
	}
}
