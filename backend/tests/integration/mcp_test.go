package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

type mcpCall func(tool string, args any) (map[string]any, bool)
type mcpKeyCall func(secret, tool string, args any) (map[string]any, bool)

// The MCP endpoint takes only API keys and its tools see only the key's collections.
func TestMCPToolsReadTheKeysCollections(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, toolSvc := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	mk := func(name string) string {
		c, err := collections.Create(owner, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(
			`{"type":"object","properties":{"nome":{"type":"string","title":"Nome do item"},"preco":{"type":"number"},"cor":{"enum":["Azul","Laranja"]},"tags":{"type":"array","items":{"type":"string"}},"dia":{"type":"string","format":"date"}}}`)})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	a, hidden := mk("Produtos"), mk("Segredo")
	var cheap, dear, safe string
	for _, row := range []struct {
		col, data string
		id        *string
	}{{a, `{"nome":"lapis","preco":2,"tags":["x","y"],"dia":"2026-01-05"}`, &cheap}, {a, `{"nome":"caneta","preco":12,"tags":["y"],"dia":"2026-01-20"}`, &dear}, {hidden, `{"nome":"cofre","preco":99}`, &safe}} {
		r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: row.col, Data: json.RawMessage(row.data)})
		if err != nil {
			t.Fatal(err)
		}
		*row.id = r.ID
	}
	// The picker list (and workspace_overview) carries the caller's role.
	opts, err := collections.Options(owner)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(opts, func(c domain.Collection) bool { return c.ID == a }); i < 0 || opts[i].Role != domain.RoleOwner || !opts[i].Member {
		t.Fatalf("options: %+v", opts)
	}
	key, err := keys.CreateMyKey(owner, "Assistente", []domain.KeyScope{{CollectionID: a}}, 0)
	if err != nil {
		t.Fatal(err)
	}

	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	post := func(secret, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("POST /mcp: %d want %d: %s", res.StatusCode, want, data)
		}
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		return out
	}
	id := 0
	var callWith func(secret, tool string, args any) (map[string]any, bool)
	call := func(tool string, args any) (map[string]any, bool) {
		t.Helper()
		return callWith(key.Secret, tool, args)
	}
	callWith = func(secret, tool string, args any) (map[string]any, bool) {
		t.Helper()
		id++
		raw, _ := json.Marshal(args)
		out := post(secret, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, raw), 200)
		res, _ := out["result"].(map[string]any)
		if res == nil {
			t.Fatalf("%s: no result: %v", tool, out)
		}
		failed, _ := res["isError"].(bool)
		got, _ := res["structuredContent"].(map[string]any)
		if failed {
			got = map[string]any{"error": res["content"]} // the message, for a failure report
		}
		return got, failed
	}

	post("", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401)
	post("dsk_nope", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401)
	list := post(key.Secret, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 200)
	if tools, _ := list["result"].(map[string]any)["tools"].([]any); len(tools) != 19 {
		t.Fatalf("tools: %v", list)
	}

	// The overview: counts and fields of the key's collections only, and who references whom.
	orders, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Pedidos", Description: "Vendas", Schema: json.RawMessage(
		`{"type":"object","required":["produto"],"properties":{"produto":{"type":"string","format":"uuid","x-ref":"` + a + `","title":"Produto"},"status":{"enum":["novo","pago"]}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: orders.ID, Data: json.RawMessage(`{"produto":"` + cheap + `","status":"novo"}`)}); err != nil {
		t.Fatal(err)
	}
	both, err := keys.CreateMyKey(owner, "Visão geral", []domain.KeyScope{{CollectionID: a}, {CollectionID: orders.ID, CanWrite: true}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	ov := post(both.Secret, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_overview","arguments":{}}}`, 200)
	checkMCPOverview(t, ov, call, a)

	checkOrderTools(t, call, callWith, both.Secret, orders.ID, cheap, dear)

	checkMCPSearch(t, call, a, cheap, dear)
	checkMCPAggregates(t, call, a, hidden)
	checkMCPRecordFields(t, call, a, cheap, dear, safe)
	if _, err := toolSvc.CreateView(owner, a, "Caros", `?q=a&where={"any":false,"rules":[{"field":"preco","op":"gt","value":10}]}&sort=data.preco`, true); err != nil {
		t.Fatal(err)
	}
	got, _ := call("list_views", map[string]any{"collection_id": a})
	if v := got["views"].([]any); len(v) != 1 || v[0].(map[string]any)["search"] != "a" || v[0].(map[string]any)["where"].(map[string]any)["rules"] == nil {
		t.Fatalf("list_views: %v", got)
	}
	// A read key cannot write; a write key creates, changes some fields and deletes.
	if _, failed := call("create_record", map[string]any{"collection_id": a, "data": map[string]any{"nome": "x"}}); !failed {
		t.Fatal("a read key created a record")
	}
	writer, err := keys.UpdateMyKey(owner, key.Key.ID, "Assistente", []domain.KeyScope{{CollectionID: a, CanWrite: true}}, nil)
	if err != nil || !writer.Scopes[0].CanWrite {
		t.Fatalf("update key: %+v, %v", writer, err)
	}
	made := checkMCPRecordWrites(t, call, a, safe)
	checkMCPRestoredHistory(t, call, a, made)
	checkMCPBulkUpdate(t, call, records, owner, a, dear)

	checkMCPHTTP(t, app, key.Secret, a, hidden)

	// Outside the key's collections everything reads as missing.
	for tool, args := range map[string]any{
		"get_collection":   map[string]any{"collection_id": hidden},
		"search_records":   map[string]any{"collection_id": hidden},
		"collection_stats": map[string]any{"collection_id": hidden},
	} {
		if _, failed := call(tool, args); !failed {
			t.Fatalf("%s reached a collection outside the key", tool)
		}
	}
}

func toolErrorText(got map[string]any) string { b, _ := json.Marshal(got["error"]); return string(b) }

func checkMCPOverview(t *testing.T, ov map[string]any, call mcpCall, a string) {
	t.Helper()
	cols, _ := ov["result"].(map[string]any)["structuredContent"].(map[string]any)["collections"].([]any)
	if len(cols) != 2 {
		t.Fatalf("overview: %v", ov)
	}
	ped, prod := cols[0].(map[string]any), cols[1].(map[string]any) // by name: Pedidos, Produtos
	field := ped["fields"].([]any)[0].(map[string]any)
	if ped["name"] != "Pedidos" || ped["description"] != "Vendas" || ped["access"] != "write" || ped["records"].(float64) != 1 || ped["role"] != "owner" ||
		field["type"] != "reference" || field["target"] != a || field["target_name"] != "Produtos" || field["required"] != true ||
		ped["fields"].([]any)[1].(map[string]any)["options"] == nil {
		t.Fatalf("overview Pedidos: %v", ped)
	}
	ref, _ := prod["referenced_by"].([]any)
	if prod["records"].(float64) != 2 || prod["access"] != "read" || len(prod["fields"].([]any)) != 5 || len(ref) != 1 ||
		ref[0].(map[string]any)["collection"] != "Pedidos" || ref[0].(map[string]any)["field"] != "produto" {
		t.Fatalf("overview Produtos: %v", prod)
	}
	// The first key reaches one collection: the other two stay out, Pedidos' reference with them.
	if got, _ := call("workspace_overview", map[string]any{}); len(got["collections"].([]any)) != 1 || got["collections"].([]any)[0].(map[string]any)["referenced_by"] != nil {
		t.Fatalf("overview, one collection: %v", got)
	}
}

func checkOrderTools(t *testing.T, call mcpCall, callWith mcpKeyCall, secret, orderID, cheap, dear string) {
	t.Helper()
	// expand brings the product each order points at; search_everywhere finds text across the key's collections.
	got, _ := callWith(secret, "search_records", map[string]any{"collection_id": orderID, "expand": []string{"produto.nome"}})
	if ex := got["records"].([]any)[0].(map[string]any)["expanded"].(map[string]any)["produto"].([]any); len(ex) != 1 ||
		ex[0].(map[string]any)["id"] != cheap || ex[0].(map[string]any)["data"].(map[string]any)["nome"] != "lapis" || len(ex[0].(map[string]any)["data"].(map[string]any)) != 1 {
		t.Fatalf("expand: %v", got)
	}
	if got, failed := callWith(secret, "search_records", map[string]any{"collection_id": orderID, "expand": []string{"status"}}); !failed {
		t.Fatalf("expand took a field that is no reference: %v", got)
	}
	got, _ = callWith(secret, "search_everywhere", map[string]any{"search": "novo"})
	if c := got["collections"].([]any); len(c) != 1 || c[0].(map[string]any)["collection"] != "Pedidos" || c[0].(map[string]any)["total"].(float64) != 1 {
		t.Fatalf("search_everywhere: %v", got)
	}
	if got, _ = call("search_everywhere", map[string]any{"search": "novo"}); len(got["collections"].([]any)) != 0 {
		t.Fatalf("search_everywhere beyond the key: %v", got)
	}
	// create_records makes all or none, naming the bad record.
	got, failed := callWith(secret, "create_records", map[string]any{"collection_id": orderID, "records": []any{map[string]any{"produto": dear}, map[string]any{"status": "pago"}}})
	if !failed || !strings.Contains(toolErrorText(got), "record 1") {
		t.Fatalf("create_records with a bad record: %v", got)
	}
	if got, failed = callWith(secret, "create_records", map[string]any{"collection_id": orderID, "records": []any{map[string]any{"produto": dear}, map[string]any{"produto": cheap, "status": "pago"}}}); failed || got["created"].(float64) != 2 {
		t.Fatalf("create_records: %v", got)
	}
}

func checkMCPSearch(t *testing.T, call mcpCall, a, cheap, dear string) {
	t.Helper()
	got, _ := call("workspace_overview", map[string]any{})
	if cols := got["collections"].([]any); len(cols) != 1 || cols[0].(map[string]any)["id"] != a || cols[0].(map[string]any)["access"] != "read" {
		t.Fatalf("collections: %v", got)
	}
	// A field or option the collection lacks fails with what to send, instead of matching nothing.
	got, failed := call("search_records", map[string]any{"collection_id": a, "rules": []any{map[string]any{"field": "Nome do Item", "op": "eq", "value": "lapis"}}})
	if !failed || !strings.Contains(toolErrorText(got), `use its key \"nome\"`) || !strings.Contains(toolErrorText(got), "preco") {
		t.Fatalf("unknown rule field: %v", got)
	}
	got, failed = call("search_records", map[string]any{"collection_id": a, "rules": []any{map[string]any{"field": "cor", "op": "in", "value": []string{"Azul", "laranja"}}}})
	if !failed || !strings.Contains(toolErrorText(got), `use \"Laranja\"`) {
		t.Fatalf("wrong option: %v", got)
	}
	for tool, args := range map[string]map[string]any{
		"aggregate_records": {"collection_id": a, "group_by": "Cor"},
		"field_values":      {"collection_id": a, "field": "price"},
		"get_records":       {"collection_id": a, "record_ids": []string{cheap}, "fields": []string{"x"}},
		"search_records":    {"collection_id": a, "sort": "data.x"},
	} {
		if got, failed := call(tool, args); !failed {
			t.Fatalf("%s took an unknown field: %v", tool, got)
		}
	}
	if got, failed := call("search_records", map[string]any{"collection_id": a, "rules": []any{map[string]any{"field": "cor", "op": "eq", "value": "Azul"}, map[string]any{"field": "$created_at", "op": "not_empty"}}}); failed || got["total"].(float64) != 0 {
		t.Fatalf("valid rules refused: %v", got)
	}
	if got, failed := call("get_collection", map[string]any{"collection_id": a}); failed || got["schema"] == nil {
		t.Fatalf("get_collection: %v", got)
	}
	got, _ = call("search_records", map[string]any{"collection_id": a, "rules": []any{map[string]any{"field": "preco", "op": "gt", "value": 10}}})
	if got["total"].(float64) != 1 || got["records"].([]any)[0].(map[string]any)["id"] != dear {
		t.Fatalf("search: %v", got)
	}
	if got, _ = call("search_records", map[string]any{"collection_id": a, "search": "lapis"}); got["total"].(float64) != 1 {
		t.Fatalf("search text: %v", got)
	}
	if got, failed := call("get_record", map[string]any{"record_id": cheap}); failed || got["id"] != cheap {
		t.Fatalf("get_record: %v", got)
	}
	if got, failed := call("related_records", map[string]any{"record_id": cheap}); failed || got["links"] == nil {
		t.Fatalf("related_records: %v", got)
	}
	if got, failed := call("collection_stats", map[string]any{"collection_id": a}); failed || got["records"].(float64) != 2 {
		t.Fatalf("collection_stats: %v", got)
	}
}

func checkMCPAggregates(t *testing.T, call mcpCall, a, hidden string) {
	t.Helper()
	// Totals, values, several records by id and saved views.
	got, _ := call("aggregate_records", map[string]any{"collection_id": a, "numbers": []string{"preco"}})
	if g := got["groups"].([]any); len(g) != 1 || g[0].(map[string]any)["count"].(float64) != 2 || g[0].(map[string]any)["preco"].(map[string]any)["sum"].(float64) != 14 {
		t.Fatalf("aggregate_records: %v", got)
	}
	got, _ = call("aggregate_records", map[string]any{"collection_id": a, "group_by": "nome", "rules": []any{map[string]any{"field": "preco", "op": "lt", "value": 10}}})
	if g := got["groups"].([]any); len(g) != 1 || g[0].(map[string]any)["value"] != "lapis" {
		t.Fatalf("aggregate_records by nome: %v", got)
	}
	// A list field counts each element; a date field groups by month.
	got, _ = call("field_values", map[string]any{"collection_id": a, "field": "tags"})
	if v := got["values"].([]any); got["distinct"].(float64) != 2 || v[0].(map[string]any)["value"] != "y" || v[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("field_values of a list: %v", got)
	}
	got, _ = call("aggregate_records", map[string]any{"collection_id": a, "group_by": "tags", "numbers": []string{"preco"}})
	if g := got["groups"].([]any)[0].(map[string]any); g["value"] != "y" || g["count"].(float64) != 2 || g["preco"].(map[string]any)["sum"].(float64) != 14 {
		t.Fatalf("aggregate_records by list: %v", got)
	}
	got, _ = call("aggregate_records", map[string]any{"collection_id": a, "group_by": "dia", "bucket": "month"})
	if g := got["groups"].([]any); len(g) != 1 || g[0].(map[string]any)["value"] != "2026-01-01" || g[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("aggregate_records by month: %v", got)
	}
	if got, failed := call("aggregate_records", map[string]any{"collection_id": a, "bucket": "month"}); !failed {
		t.Fatalf("bucket without group_by: %v", got)
	}
	if got, failed := call("aggregate_records", map[string]any{"collection_id": hidden}); !failed {
		t.Fatalf("aggregate_records outside the key: %v", got)
	}
	if got, _ = call("field_values", map[string]any{"collection_id": a, "field": "nome"}); got["distinct"].(float64) != 2 || got["filled"].(float64) != 2 || len(got["values"].([]any)) != 2 || got["more"] != false {
		t.Fatalf("field_values: %v", got)
	}
}

func checkMCPRecordFields(t *testing.T, call mcpCall, a, cheap, dear, safe string) {
	t.Helper()
	// One value a page: the second page holds the other one; past the end, none, still with the distinct count.
	first, _ := call("field_values", map[string]any{"collection_id": a, "field": "nome", "limit": 1})
	second, _ := call("field_values", map[string]any{"collection_id": a, "field": "nome", "limit": 1, "page": 2})
	if first["more"] != true || second["more"] != false || len(second["values"].([]any)) != 1 ||
		first["values"].([]any)[0].(map[string]any)["value"] == second["values"].([]any)[0].(map[string]any)["value"] {
		t.Fatalf("field_values pages: %v / %v", first, second)
	}
	if got, _ := call("field_values", map[string]any{"collection_id": a, "field": "nome", "limit": 1, "page": 5}); len(got["values"].([]any)) != 0 || got["distinct"].(float64) != 2 {
		t.Fatalf("field_values past the end: %v", got)
	}
	if _, failed := call("field_values", map[string]any{"collection_id": a, "field": "nome", "limit": 500}); !failed {
		t.Fatal("field_values took 500 a page")
	}
	// fields cuts each record's data to what the model asks for; the listing drops the collection it names.
	got, _ := call("search_records", map[string]any{"collection_id": a, "search": "lapis", "fields": []string{"nome"}})
	if r := got["records"].([]any)[0].(map[string]any); len(r["data"].(map[string]any)) != 1 || r["data"].(map[string]any)["nome"] != "lapis" || r["collection_id"] != nil || r["id"] != cheap {
		t.Fatalf("search_records fields: %v", got)
	}
	if got, _ = call("get_record", map[string]any{"record_id": dear, "fields": []string{"preco"}}); got["data"].(map[string]any)["nome"] != nil || got["data"].(map[string]any)["preco"].(float64) != 12 {
		t.Fatalf("get_record fields: %v", got)
	}
	if _, failed := call("search_records", map[string]any{"collection_id": a, "fields": []string{""}}); !failed {
		t.Fatal("search_records took an empty field name")
	}
	got, _ = call("get_records", map[string]any{"collection_id": a, "record_ids": []string{cheap, dear}, "fields": []string{"preco"}})
	if r := got["records"].([]any); len(r) != 2 || len(r[0].(map[string]any)["data"].(map[string]any)) != 1 {
		t.Fatalf("get_records fields: %v", got)
	}
	got, _ = call("get_records", map[string]any{"collection_id": a, "record_ids": []string{cheap, dear, safe}})
	if len(got["records"].([]any)) != 2 || len(got["missing"].([]any)) != 1 || got["missing"].([]any)[0] != safe {
		t.Fatalf("get_records: %v", got)
	}
}

func checkMCPRecordWrites(t *testing.T, call mcpCall, a, safe string) string {
	t.Helper()
	got, failed := call("create_record", map[string]any{"collection_id": a, "data": map[string]any{"nome": "borracha", "preco": 1}})
	if failed || got["id"] == nil {
		t.Fatalf("create_record: %v", got)
	}
	made := got["id"].(string)
	if _, failed := call("create_record", map[string]any{"collection_id": a, "data": map[string]any{"preco": "caro"}}); !failed {
		t.Fatal("create_record took data against the schema")
	}
	got, failed = call("update_record", map[string]any{"record_id": made, "set": map[string]any{"preco": 3, "nome": nil}})
	if data, _ := got["data"].(map[string]any); failed || data["preco"].(float64) != 3 || data["nome"] != nil {
		t.Fatalf("update_record: %v", got)
	}
	if _, failed := call("update_record", map[string]any{"record_id": made, "set": map[string]any{"preco": 4}, "version": 1}); !failed {
		t.Fatal("update_record ignored a stale version")
	}
	// A delete names its collection: a record outside it refuses the whole call.
	if _, failed := call("delete_records", map[string]any{"record_ids": []string{made}}); !failed {
		t.Fatal("delete_records ran without its collection")
	}
	if _, failed := call("delete_records", map[string]any{"collection_id": a, "record_ids": []string{made, safe}}); !failed {
		t.Fatal("delete_records took a record of another collection")
	}
	if _, failed := call("get_record", map[string]any{"record_id": made}); failed {
		t.Fatal("a refused delete still removed a record")
	}
	if got, failed := call("delete_records", map[string]any{"collection_id": a, "record_ids": []string{made}}); failed || got["deleted"].(float64) != 1 {
		t.Fatalf("delete_records: %v", got)
	}
	if _, failed := call("get_record", map[string]any{"record_id": made}); !failed {
		t.Fatal("a deleted record still reads")
	}
	return made
}

func checkMCPRestoredHistory(t *testing.T, call mcpCall, a, made string) {
	t.Helper()
	// The trash lists it and gives it back; its history shows what update_record changed.
	got, _ := call("list_trash", map[string]any{"collection_id": a})
	if r := got["records"].([]any); len(r) != 1 || r[0].(map[string]any)["id"] != made {
		t.Fatalf("list_trash: %v", got)
	}
	if got, failed := call("restore_records", map[string]any{"collection_id": a, "record_ids": []string{made}}); failed || got["restored"].(float64) != 1 {
		t.Fatalf("restore_records: %v", got)
	}
	got, _ = call("record_history", map[string]any{"record_id": made})
	v := got["versions"].([]any)
	if len(v) != 2 || v[1].(map[string]any)["data"] == nil {
		t.Fatalf("record_history: %v", got)
	}
	if ch := v[0].(map[string]any)["changes"].(map[string]any); len(ch) != 2 || ch["preco"].(map[string]any)["to"].(float64) != 3 || ch["nome"].(map[string]any)["to"] != nil {
		t.Fatalf("record_history changes: %v", got)
	}
}

func checkMCPBulkUpdate(t *testing.T, call mcpCall, records input.RecordService, owner context.Context, a, dear string) {
	t.Helper()
	// update_records wants a filter, counts with dry_run, then sets every match.
	if _, failed := call("update_records", map[string]any{"collection_id": a, "set": map[string]any{"nome": "x"}}); !failed {
		t.Fatal("update_records ran without a filter")
	}
	caros := []any{map[string]any{"field": "preco", "op": "gt", "value": 2}}
	got, _ := call("update_records", map[string]any{"collection_id": a, "rules": caros, "set": map[string]any{"nome": "caro"}, "dry_run": true})
	if got["matched"].(float64) != 2 || len(got["sample"].([]any)) != 2 {
		t.Fatalf("update_records dry run: %v", got)
	}
	if r, _ := records.Get(owner, dear); !strings.Contains(string(r.Data), "caneta") {
		t.Fatalf("a dry run changed a record: %s", r.Data)
	}
	if got, failed := call("update_records", map[string]any{"collection_id": a, "rules": caros, "set": map[string]any{"nome": "caro"}}); failed || got["updated"].(float64) != 2 {
		t.Fatalf("update_records: %v", got)
	}
	if got, _ = call("field_values", map[string]any{"collection_id": a, "field": "nome", "search": "caro"}); got["values"].([]any)[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("after update_records: %v", got)
	}
}

func checkMCPHTTP(t *testing.T, app *fiber.App, secret, a, hidden string) {
	t.Helper()
	// Swagger asks for JSON alone and sends the key in X-API-Key; the shortcuts skip the envelope.
	plain := func(method, path, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-API-Key", secret)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.StatusCode, want, data)
		}
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		return out
	}
	if out := plain("POST", "/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 200); out["result"] == nil {
		t.Fatalf("JSON-only accept: %v", out)
	}
	if tools, _ := plain("GET", "/v1/mcp/tools", "", 200)["tools"].([]any); len(tools) != 19 {
		t.Fatal("GET /v1/mcp/tools: want 9 tools")
	}
	if got, _ := plain("POST", "/v1/mcp/tools/get_collection", `{"collection_id":"`+a+`"}`, 200)["structuredContent"].(map[string]any); got["schema"] == nil {
		t.Fatal("POST /v1/mcp/tools/get_collection: no schema")
	}
	if out := plain("POST", "/v1/mcp/tools/get_collection", `{"collection_id":"`+hidden+`"}`, 200); out["isError"] != true {
		t.Fatalf("shortcut reached a collection outside the key: %v", out)
	}
	if out := plain("POST", "/v1/mcp/tools/workspace_overview", `[`, 400); out["code"] != "INVALID_INPUT" {
		t.Fatalf("bad body: %v", out)
	}
	plain("POST", "/v1/mcp/tools/no_such_tool", `{}`, 400)
	req := httptest.NewRequest("GET", "/v1/mcp/tools", nil)
	if res, err := app.Test(req); err != nil || res.StatusCode != 401 {
		t.Fatalf("GET /v1/mcp/tools without a key: %v %v", res, err)
	}
}
