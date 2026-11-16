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

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// An owner's MCP tool shows up for the keys that reach its collection, typed
// from the schema, and runs its fixed conditions plus the given arguments.
func TestOwnersPublishMCPTools(t *testing.T) {
	e := newEnv(t)
	collections, records := collectionServices(e)
	integ, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Produtos", Schema: json.RawMessage(
		`{"type":"object","properties":{"nome":{"type":"string"},"cidade":{"type":"string"},"preco":{"type":"number"},"tags":{"type":"array","items":{"type":"string"}}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		`{"nome":"lapis","cidade":"Manaus","preco":2,"tags":["escola"]}`,
		`{"nome":"caneta","cidade":"Manaus","preco":12,"tags":["escola","escritorio"]}`,
		`{"nome":"mesa","cidade":"Belem","preco":300,"tags":["escritorio"]}`,
	} {
		if _, err := records.Create(owner, input.CreateRecordCommand{CollectionID: c.ID, Data: json.RawMessage(d)}); err != nil {
			t.Fatal(err)
		}
	}
	tool := input.MCPToolInput{
		Name: "produtos_caros", Description: "Produtos acima de 5 reais de uma cidade.", Enabled: true,
		Conditions: domain.Where{Rules: []domain.Condition{{Field: "preco", Op: "gt", Value: json.RawMessage(`5`)}}},
		Params: []domain.MCPToolParam{
			{Name: "cidade", Field: "cidade", Op: "eq", Required: true, Description: "nome da cidade"},
			{Name: "tag", Field: "tags", Op: "has"},
			{Name: "nomes", Field: "nome", Op: "in"},
		},
		Fields: []string{"nome"}, Sort: "data.preco", Order: "asc",
	}
	for name, bad := range map[string]func(*input.MCPToolInput){
		"builtin name":   func(in *input.MCPToolInput) { in.Name = "search_records" },
		"no description": func(in *input.MCPToolInput) { in.Description = " " },
		"unknown field":  func(in *input.MCPToolInput) { in.Params = []domain.MCPToolParam{{Name: "x", Field: "nada", Op: "eq"}} },
		"contains a number": func(in *input.MCPToolInput) {
			in.Params = []domain.MCPToolParam{{Name: "x", Field: "preco", Op: "contains"}}
		},
		"page param": func(in *input.MCPToolInput) {
			in.Params = []domain.MCPToolParam{{Name: "page", Field: "nome", Op: "eq"}}
		},
		"unknown return": func(in *input.MCPToolInput) { in.Fields = []string{"nada"} },
		"bad sort":       func(in *input.MCPToolInput) { in.Sort = "data.nada; drop" },
	} {
		in := tool
		bad(&in)
		if _, err := integ.CreateMCPTool(owner, c.ID, in); !apperror.IsCode(err, apperror.CodeInvalidInput) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	made, err := integ.CreateMCPTool(owner, c.ID, tool)
	if err != nil {
		t.Fatal(err)
	}
	if made.Params[0].Type != "string" || made.Params[1].Type != "string" {
		t.Fatalf("param types: %+v", made.Params)
	}
	_, err = integ.CreateMCPTool(owner, c.ID, tool)
	wantCode(t, err, apperror.CodeAlreadyExists)

	key, err := integ.CreateMyKey(owner, "Assistente", []domain.KeyScope{{CollectionID: c.ID}})
	if err != nil {
		t.Fatal(err)
	}
	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	id := 0
	rpc := func(method, params string) map[string]any {
		t.Helper()
		id++
		req := httptest.NewRequest("POST", "/v1/mcp", strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+key.Secret)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		var out map[string]any
		if json.Unmarshal(data, &out) != nil || out["result"] == nil {
			t.Fatalf("%s: %d %s", method, res.StatusCode, data)
		}
		return out["result"].(map[string]any)
	}
	listed := func() []any {
		return rpc("tools/list", "{}")["tools"].([]any)
	}
	call := func(args string) (map[string]any, bool) {
		res := rpc("tools/call", `{"name":"produtos_caros","arguments":`+args+`}`)
		failed, _ := res["isError"].(bool)
		got, _ := res["structuredContent"].(map[string]any)
		if failed {
			got = map[string]any{"error": res["content"]}
		}
		return got, failed
	}

	all := listed()
	i := slices.IndexFunc(all, func(x any) bool { return x.(map[string]any)["name"] == "produtos_caros" })
	if len(all) != 10 || i < 0 {
		t.Fatalf("tools: %v", all)
	}
	sch := all[i].(map[string]any)["inputSchema"].(map[string]any)
	if req := sch["required"].([]any); len(req) != 1 || req[0] != "cidade" {
		t.Fatalf("schema: %v", sch)
	}
	if nomes := sch["properties"].(map[string]any)["nomes"].(map[string]any); nomes["type"] != "array" {
		t.Fatalf("in param: %v", nomes)
	}

	if _, failed := call(`{}`); !failed {
		t.Fatal("ran without the required argument")
	}
	if _, failed := call(`{"cidade":3}`); !failed {
		t.Fatal("ran with a number for a text field")
	}
	got, failed := call(`{"cidade":"Manaus"}`)
	recs, _ := got["records"].([]any)
	if failed || got["total"].(float64) != 1 || len(recs) != 1 {
		t.Fatalf("call: %v", got)
	}
	if data := recs[0].(map[string]any)["data"].(map[string]any); data["nome"] != "caneta" || data["preco"] != nil {
		t.Fatalf("projection: %v", data)
	}
	if got, _ = call(`{"cidade":"Belem","tag":"escola"}`); got["total"].(float64) != 0 {
		t.Fatalf("has: %v", got)
	}
	if got, _ = call(`{"cidade":"Belem","nomes":["mesa","x"]}`); got["total"].(float64) != 1 {
		t.Fatalf("in: %v", got)
	}

	// A disabled tool leaves the list; a key for another collection never sees it.
	tool.Enabled = false
	if _, err := integ.UpdateMCPTool(owner, c.ID, made.ID, tool); err != nil {
		t.Fatal(err)
	}
	if n := len(listed()); n != 9 {
		t.Fatalf("disabled tool still listed: %d", n)
	}
	if err := integ.DeleteMCPTool(owner, c.ID, made.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := integ.MCPTools(owner, c.ID); len(list) != 0 {
		t.Fatalf("after delete: %v", list)
	}
}
