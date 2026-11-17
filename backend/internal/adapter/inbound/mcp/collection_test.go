package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

const testCollectionID = "58f8d719-cd56-4ee0-9cac-0aa0e1af6e9b"

type collectionKeyStub struct{ input.IntegrationService }

func (collectionKeyStub) AuthenticateKey(context.Context, string) (*domain.KeyGrant, error) {
	return &domain.KeyGrant{UserID: "user", Scopes: map[string]bool{testCollectionID: false}}, nil
}

func (collectionKeyStub) KeyMCPTools(context.Context) ([]domain.MCPTool, error) {
	return nil, nil
}

func (collectionKeyStub) LogMCPCalls(context.Context, []domain.MCPCall) error { return nil }

type collectionReadStub struct{ input.CollectionService }

func (collectionReadStub) Overview(context.Context, []string) ([]domain.Collection, error) {
	return []domain.Collection{{ID: testCollectionID, Name: "Example collection", Role: domain.RoleViewer}}, nil
}

func (collectionReadStub) Get(context.Context, string) (*domain.Collection, error) {
	return nil, apperror.New(apperror.CodeNotFound, "not found")
}

type collectionRecordsStub struct {
	input.RecordService
	listed int
}

func (s *collectionRecordsStub) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	s.listed++
	key, _ := reqctx.Key(ctx)
	if _, allowed := key.Scopes[filter.CollectionID]; !allowed {
		return nil, 0, apperror.New(apperror.CodeNotFound, "not found")
	}
	return []domain.Record{{ID: "record", Data: json.RawMessage(`{"nome":"Example"}`)}}, 1, nil
}

// Exercise the real SDK schemas, decoding and tool-error envelope over HTTP.
func collectionRPC(t *testing.T, h http.Handler, method string, params any) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["error"] != nil {
		t.Fatalf("protocol error instead of MCP result: %s", rec.Body.String())
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(envelope["result"], &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCollectionIDGuidanceInInstructionsAndCatalog(t *testing.T) {
	h := Handler(&input.Application{Integrations: collectionKeyStub{}}, "test")
	init := collectionRPC(t, h, "initialize", map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	var instructions string
	_ = json.Unmarshal(init["instructions"], &instructions)
	if !strings.Contains(instructions, "id UUID exactly from workspace_overview") || !strings.Contains(instructions, "never send its name or slug") {
		t.Fatalf("missing collection-id instructions: %s", instructions)
	}
	listed := collectionRPC(t, h, "tools/list", map[string]any{})
	var tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
			Required []string `json:"required"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal(listed["tools"], &tools); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, tool := range tools {
		if prop, ok := tool.InputSchema.Properties["collection_id"]; ok {
			checked++
			for _, want := range []string{"UUID", "workspace_overview", "never its name or slug"} {
				if !strings.Contains(prop.Description, want) {
					t.Errorf("%s collection_id description lacks %q: %s", tool.Name, want, prop.Description)
				}
			}
			if !slices.Contains(tool.InputSchema.Required, "collection_id") {
				t.Errorf("%s no longer requires collection_id", tool.Name)
			}
		}
	}
	if checked != 13 {
		t.Fatalf("checked %d collection_id schemas, want 13", checked)
	}
}

func TestCollectionNamesReturnRecoverableToolErrors(t *testing.T) {
	// Services are deliberately absent: a bad id must never reach any lookup or write.
	h := Handler(&input.Application{Integrations: collectionKeyStub{}}, "test")
	for _, name := range []string{
		"get_collection", "search_records", "collection_stats", "create_record", "delete_records",
		"aggregate_records", "field_values", "get_records", "list_views", "list_trash",
		"create_records", "update_records", "restore_records",
	} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{"collection_id": "wireds"}
			switch name {
			case "field_values":
				args["field"] = "tipo"
			case "get_records", "delete_records", "restore_records":
				args["record_ids"] = []string{testCollectionID}
			case "create_record":
				args["data"] = map[string]any{"nome": "Example"}
			case "create_records":
				args["records"] = []any{map[string]any{"nome": "Example"}}
			case "update_records":
				args["set"], args["search"] = map[string]any{"nome": "Example"}, "example"
			}
			result := collectionRPC(t, h, "tools/call", map[string]any{"name": name, "arguments": args})
			if string(result["isError"]) != "true" {
				t.Fatalf("name accepted: %s", result)
			}
			var content []struct{ Text string }
			_ = json.Unmarshal(result["content"], &content)
			if len(content) != 1 {
				t.Fatalf("unexpected content: %s", result["content"])
			}
			for _, want := range []string{"[INVALID_INPUT] collection_id:", "UUID from workspace_overview", "not its name or slug", "copy the collection's id into collection_id, then retry"} {
				if !strings.Contains(content[0].Text, want) {
					t.Errorf("error lacks %q: %s", want, content[0].Text)
				}
			}
		})
	}
}

func TestCollectionIDCanBeCorrectedUsingOverview(t *testing.T) {
	records := &collectionRecordsStub{}
	h := Handler(&input.Application{Integrations: collectionKeyStub{}, Collections: collectionReadStub{}, Records: records}, "test")
	call := func(name string, args any) map[string]json.RawMessage {
		return collectionRPC(t, h, "tools/call", map[string]any{"name": name, "arguments": args})
	}
	if result := call("search_records", map[string]any{"collection_id": "wireds"}); string(result["isError"]) != "true" || records.listed != 0 {
		t.Fatalf("invalid id reached records: %s", result)
	}
	overview := call("workspace_overview", map[string]any{})
	var workspace struct {
		Collections []struct{ ID string }
	}
	if err := json.Unmarshal(overview["structuredContent"], &workspace); err != nil || len(workspace.Collections) != 1 {
		t.Fatalf("overview: %s, err: %v", overview, err)
	}
	result := call("search_records", map[string]any{"collection_id": workspace.Collections[0].ID})
	if string(result["isError"]) == "true" || records.listed != 1 {
		t.Fatalf("corrected id failed: %s", result)
	}
	var search struct{ Total int }
	if err := json.Unmarshal(result["structuredContent"], &search); err != nil || search.Total != 1 {
		t.Fatalf("corrected result: %s, err: %v", result, err)
	}
	// Syntax validation must not resolve names or grant access to another UUID.
	result = call("search_records", map[string]any{"collection_id": "6f7b5494-abcd-4e89-ad33-2bdc51af1826"})
	if string(result["isError"]) != "true" || !strings.Contains(string(result["content"]), "[NOT_FOUND]") {
		t.Fatalf("hidden collection was exposed: %s", result)
	}
}
