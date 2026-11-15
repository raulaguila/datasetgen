package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// The MCP endpoint takes only API keys and its tools see only the key's collections.
func TestMCPToolsReadTheKeysCollections(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	keys, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	mk := func(name string) string {
		c, err := collections.Create(owner, input.CreateCollectionCommand{Name: name, Schema: json.RawMessage(
			`{"type":"object","properties":{"nome":{"type":"string"},"preco":{"type":"number"}}}`)})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	a, hidden := mk("Produtos"), mk("Segredo")
	var cheap, dear string
	for _, row := range []struct {
		col, data string
		id        *string
	}{{a, `{"nome":"lapis","preco":2}`, &cheap}, {a, `{"nome":"caneta","preco":12}`, &dear}, {hidden, `{"nome":"cofre","preco":99}`, new(string)}} {
		r, err := records.Create(owner, input.CreateRecordCommand{CollectionID: row.col, Data: json.RawMessage(row.data)})
		if err != nil {
			t.Fatal(err)
		}
		*row.id = r.ID
	}
	key, err := keys.CreateMyKey(owner, "Assistente", []domain.KeyScope{{CollectionID: a}})
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
	call := func(tool string, args any) (map[string]any, bool) {
		t.Helper()
		id++
		raw, _ := json.Marshal(args)
		out := post(key.Secret, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, tool, raw), 200)
		res, _ := out["result"].(map[string]any)
		if res == nil {
			t.Fatalf("%s: no result: %v", tool, out)
		}
		failed, _ := res["isError"].(bool)
		got, _ := res["structuredContent"].(map[string]any)
		return got, failed
	}

	post("", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401)
	post("dsk_nope", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401)
	list := post(key.Secret, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 200)
	if tools, _ := list["result"].(map[string]any)["tools"].([]any); len(tools) != 6 {
		t.Fatalf("tools: %v", list)
	}

	got, _ := call("list_collections", map[string]any{})
	if cols := got["collections"].([]any); len(cols) != 1 || cols[0].(map[string]any)["id"] != a || cols[0].(map[string]any)["access"] != "read" {
		t.Fatalf("collections: %v", got)
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
