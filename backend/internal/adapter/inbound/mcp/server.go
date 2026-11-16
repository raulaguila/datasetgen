// Package mcp serves the Model Context Protocol at /v1/mcp: tools that let an AI
// assistant read, and with a write scope change, the collections an API key reaches. Every tool runs through
// the same services as the REST API, so the key's scopes and the member's
// roles apply unchanged (policy.CollectionRole).
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

const maxRecords = 50 // per search_records call: enough to read, small enough for a model's context

// Handler answers MCP over streamable HTTP, stateless: each POST is one
// request, so nothing is kept between calls. Only API keys get in. Each request
// gets its own server, since the owners' tools differ by key.
func Handler(app *input.Application, version string) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return newServer(r.Context(), app, version) },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return keyOnly(app.Integrations, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Answers are always JSON (JSONResponse), so a client asking for JSON alone, as Swagger does, gets in too.
		if r.Method == http.MethodPost {
			r.Header.Set("Accept", "application/json, text/event-stream")
		}
		h.ServeHTTP(w, r)
	}))
}

func newServer(ctx context.Context, app *input.Application, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "dataset", Version: version}, &mcp.ServerOptions{
		Instructions: "Read the collections this API key reaches. Start with list_collections, " +
			"read a collection's fields with get_collection, then find records with search_records. " +
			"Tools other than the built-in ones were made by the collections' owners for common questions: prefer them when they fit.",
	})
	t := tools{app}
	mcp.AddTool(s, &mcp.Tool{Name: "list_collections", Description: "The collections this API key reaches, with the access it has to each (read or write).",
		Annotations: readOnly}, t.listCollections)
	mcp.AddTool(s, &mcp.Tool{Name: "get_collection", Description: "A collection's name, description and the JSON Schema its records follow: read it to learn the field names before searching.",
		Annotations: readOnly}, t.getCollection)
	mcp.AddTool(s, &mcp.Tool{Name: "search_records", Description: "Find records of a collection by text and by rules on its top-level fields, newest change first unless sorted. Returns at most 50 per page and the total.",
		Annotations: readOnly}, t.searchRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "get_record", Description: "One record by id: its data, version and dates.",
		Annotations: readOnly}, t.getRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "related_records", Description: "Records of other collections whose reference fields point at this record, grouped by collection and field, newest few of each with the total.",
		Annotations: readOnly}, t.relatedRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "collection_stats", Description: "A collection's totals: records, members, last change, records created per day and top contributors over the last 30 days (UTC).",
		Annotations: readOnly}, t.collectionStats)

	// Writing tools: the key's write scope and the member's editor role apply (services check both).
	mcp.AddTool(s, &mcp.Tool{Name: "create_record", Description: "Create a record in a collection the key may write. data must follow the collection's JSON Schema (get_collection); fields the server fills (auto numbers, formulas, created_by) are left out.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false)}}, t.createRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "update_record", Description: "Change some top-level fields of one record, keeping the others: set holds the new values, null removes a field. The result must still follow the schema. Pass the version you read to refuse the change if someone else changed the record since.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false), IdempotentHint: true}}, t.updateRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_records", Description: "Move up to 100 records of one collection to the trash, all or none; a record of another collection refuses the whole call. An owner can restore them from the trash.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(true), IdempotentHint: true}}, t.deleteRecords)

	t.addOwnTools(ctx, s)
	s.AddReceivingMiddleware(t.logCalls)
	return s
}

var readOnly = &mcp.ToolAnnotations{ReadOnlyHint: true}

// keyOnly lets in a request carrying an API key ("Authorization: Bearer dsk_..."
// or "X-API-Key: dsk_..."); the key's member and scopes go on the context.
func keyOnly(keys input.IntegrationService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if secret == "" {
			secret = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		grant, err := keys.AuthenticateKey(r.Context(), secret)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="dataset", error="invalid_token"`)
			http.Error(w, "a valid API key is required: X-API-Key: dsk_... or Authorization: Bearer dsk_...", http.StatusUnauthorized)
			return
		}
		ctx := reqctx.WithActor(r.Context(), grant.UserID)
		ctx = reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: grant.KeyID, Name: grant.Name, Scopes: grant.Scopes})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type tools struct{ app *input.Application }

type collectionItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Access string `json:"access"` // read or write
}

func access(canWrite bool) string {
	if canWrite {
		return "write"
	}
	return "read"
}

func (t tools) listCollections(ctx context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	key, _ := reqctx.Key(ctx)
	// shortcut: picks from the member's first 1000 collections (Options); a key reaches at most 50.
	all, err := t.app.Collections.Options(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := []collectionItem{}
	for _, c := range all {
		if canWrite, ok := key.Scopes[c.ID]; ok {
			items = append(items, collectionItem{ID: c.ID, Name: c.Name, Access: access(canWrite)})
		}
	}
	return nil, map[string]any{"collections": items}, nil
}

type collectionIn struct {
	CollectionID string `json:"collection_id" jsonschema:"the collection's id, from list_collections"`
}

func (t tools) getCollection(ctx context.Context, _ *mcp.CallToolRequest, in collectionIn) (*mcp.CallToolResult, any, error) {
	c, err := t.app.Collections.Get(ctx, in.CollectionID)
	if err != nil {
		return nil, nil, err
	}
	key, _ := reqctx.Key(ctx)
	return nil, map[string]any{
		"id": c.ID, "name": c.Name, "description": c.Description, "schema": c.Schema,
		"access": access(key.Scopes[c.ID]), "updated_at": c.UpdatedAt,
	}, nil
}

type ruleIn struct {
	Field string `json:"field" jsonschema:"a top-level field of the record data"`
	Op    string `json:"op" jsonschema:"eq, neq, contains / not_contains / starts_with (text, ignoring case), gt, gte, lt, lte, between (value is [from, to]), empty, not_empty, in / not_in (value is a list), has (the field is a list holding value), has_any (the field is a list holding one of the values). A date value is YYYY-MM-DD or {'today': n} (with double quotes), days from today (UTC); it matches date-times by day. Field $created_at or $updated_at is the record's own date."`
	Value any    `json:"value,omitempty" jsonschema:"the JSON value to compare with; left out for empty and not_empty"`
}

type groupIn struct {
	Any   bool     `json:"any,omitempty" jsonschema:"join this group's rules by OR instead of AND"`
	Rules []ruleIn `json:"rules"`
}

type searchIn struct {
	CollectionID string    `json:"collection_id" jsonschema:"the collection's id, from list_collections"`
	Search       string    `json:"search,omitempty" jsonschema:"text found anywhere in the record JSON"`
	Rules        []ruleIn  `json:"rules,omitempty" jsonschema:"rules on fields, joined by AND (or OR when any is set)"`
	Groups       []groupIn `json:"groups,omitempty" jsonschema:"bracketed sets of rules, joined with the rules by any: (a AND b) OR (c AND d)"`
	Any          bool      `json:"any,omitempty" jsonschema:"join rules and groups by OR instead of AND"`
	Sort         string    `json:"sort,omitempty" jsonschema:"created_at, updated_at (default) or data.<field>"`
	Order        string    `json:"order,omitempty" jsonschema:"asc or desc (default desc)"`
	Page         int       `json:"page,omitempty" jsonschema:"page number, from 1"`
	Limit        int       `json:"limit,omitempty" jsonschema:"records per page, 1 to 50 (default 20)"`
}

type recordOut struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Data         json.RawMessage `json:"data"`
	Version      int             `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func recordOf(r domain.Record) recordOut {
	return recordOut{ID: r.ID, CollectionID: r.CollectionID, Data: r.Data, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func recordsOf(rs []domain.Record) []recordOut {
	out := make([]recordOut, len(rs))
	for i, r := range rs {
		out[i] = recordOf(r)
	}
	return out
}

func rulesOf(in []ruleIn) ([]domain.Condition, error) {
	out := make([]domain.Condition, len(in))
	for i, r := range in {
		out[i] = domain.Condition{Field: r.Field, Op: r.Op}
		if r.Value != nil {
			v, err := json.Marshal(r.Value)
			if err != nil {
				return nil, apperror.New(apperror.CodeInvalidInput, "a rule's value is not JSON").WithField("rules")
			}
			out[i].Value = v
		}
	}
	return out, nil
}

func whereOf(in searchIn) (*domain.Where, error) {
	if len(in.Rules) == 0 && len(in.Groups) == 0 {
		return nil, nil
	}
	rules, err := rulesOf(in.Rules)
	if err != nil {
		return nil, err
	}
	w := &domain.Where{Any: in.Any, Rules: rules}
	for _, g := range in.Groups {
		rules, err := rulesOf(g.Rules)
		if err != nil {
			return nil, err
		}
		w.Groups = append(w.Groups, domain.Where{Any: g.Any, Rules: rules})
	}
	return w, nil
}

func (t tools) searchRecords(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
	where, err := whereOf(in)
	if err != nil {
		return nil, nil, err
	}
	limit := in.Limit
	if limit < 1 {
		limit = 20
	}
	sort, order := in.Sort, in.Order
	if sort == "" {
		sort, order = "updated_at", "desc"
	}
	filter := domain.RecordFilter{
		CollectionID: in.CollectionID, Search: in.Search, Sort: sort, Order: order, Where: where,
		Page: domain.NewPage(in.Page, min(limit, maxRecords)),
	}
	items, total, err := t.app.Records.List(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"total": total, "page": filter.Page.Number, "records": recordsOf(items)}, nil
}

type recordIn struct {
	RecordID string `json:"record_id" jsonschema:"the record's id"`
}

func (t tools) getRecord(ctx context.Context, _ *mcp.CallToolRequest, in recordIn) (*mcp.CallToolResult, any, error) {
	r, err := t.app.Records.Get(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	touch(ctx, r.CollectionID)
	return nil, recordOf(*r), nil
}

type linksOut struct {
	CollectionID string      `json:"collection_id"`
	Collection   string      `json:"collection"`
	Field        string      `json:"field"`
	Total        int64       `json:"total"`
	Records      []recordOut `json:"records"`
}

func (t tools) relatedRecords(ctx context.Context, _ *mcp.CallToolRequest, in recordIn) (*mcp.CallToolResult, any, error) {
	links, err := t.app.Records.Links(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]linksOut, len(links))
	for i, l := range links {
		field := l.Field
		if l.Sub != "" {
			field += "[]." + l.Sub
		}
		out[i] = linksOut{CollectionID: l.CollectionID, Collection: l.Name, Field: field, Total: l.Total, Records: recordsOf(l.Records)}
	}
	return nil, map[string]any{"links": out}, nil
}

func (t tools) collectionStats(ctx context.Context, _ *mcp.CallToolRequest, in collectionIn) (*mcp.CallToolResult, any, error) {
	st, err := t.app.Collections.Stats(ctx, in.CollectionID, "UTC")
	if err != nil {
		return nil, nil, err
	}
	perDay := make([]map[string]any, len(st.PerDay))
	for i, d := range st.PerDay {
		perDay[i] = map[string]any{"day": d.Day, "count": d.Count}
	}
	people := make([]map[string]any, len(st.Contributors))
	for i, p := range st.Contributors {
		people[i] = map[string]any{"name": p.Name, "created": p.Created, "edited": p.Edited}
	}
	return nil, map[string]any{"records": st.Records, "members": st.Members, "last_update": st.LastUpdate, "per_day": perDay, "contributors": people}, nil
}

type createIn struct {
	CollectionID string         `json:"collection_id" jsonschema:"the collection's id, from list_collections"`
	Data         map[string]any `json:"data" jsonschema:"the record, a JSON object following the collection's schema"`
}

func (t tools) createRecord(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(in.Data)
	if err != nil || in.Data == nil {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "data must be a JSON object").WithField("data")
	}
	r, err := t.app.Records.Create(ctx, input.CreateRecordCommand{CollectionID: in.CollectionID, Data: data})
	if err != nil {
		return nil, nil, err
	}
	return nil, recordOf(*r), nil
}

type updateIn struct {
	RecordID string         `json:"record_id" jsonschema:"the record's id"`
	Set      map[string]any `json:"set" jsonschema:"the top-level fields to change and their new values; null removes the field"`
	Version  *int           `json:"version,omitempty" jsonschema:"the version you read; the change is refused if the record moved on"`
}

func (t tools) updateRecord(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, any, error) {
	if len(in.Set) == 0 {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "set names no field").WithField("set")
	}
	current, err := t.app.Records.Get(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	touch(ctx, current.CollectionID)
	data := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(current.Data))
	dec.UseNumber() // the fields left alone keep their exact numbers
	if err := dec.Decode(&data); err != nil {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "the record is not a JSON object")
	}
	for k, v := range in.Set {
		if v == nil {
			delete(data, k)
		} else {
			data[k] = v
		}
	}
	merged, _ := json.Marshal(data)
	version := in.Version
	if version == nil { // still refuse a change made between the read above and the write
		version = &current.Version
	}
	r, err := t.app.Records.Update(ctx, input.UpdateRecordCommand{ID: in.RecordID, Data: merged, Version: version})
	if err != nil {
		return nil, nil, err
	}
	return nil, recordOf(*r), nil
}

type deleteIn struct {
	CollectionID string   `json:"collection_id" jsonschema:"the collection's id, from list_collections; every record must be in it"`
	RecordIDs    []string `json:"record_ids" jsonschema:"1 to 100 record ids"`
}

func (t tools) deleteRecords(ctx context.Context, _ *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, any, error) {
	if err := t.app.Records.DeleteManyIn(ctx, in.CollectionID, in.RecordIDs); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"deleted": len(in.RecordIDs)}, nil
}
