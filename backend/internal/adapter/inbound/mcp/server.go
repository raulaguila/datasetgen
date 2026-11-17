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
	"github.com/raulaguila/dataset/pkg/schema"
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
		Instructions: "Work in steps. " +
			"1. workspace_overview: every collection this key reaches, with its fields and the options of option fields. " +
			"Arguments name a field by its key, never by its display name. Not sure which collection holds something? search_everywhere. " +
			"2. To filter a text field, first learn its exact values with field_values. " +
			"3. search_records with rules [{field, op, value}] to keep records by a field's value (search is free text in any field, not a filter), " +
			"fields listing only the keys you need, and expand to bring referenced records along. " +
			"4. aggregate_records to count or sum by a field (each element of a list, or per month of a date) instead of reading every record. " +
			"An error says what to fix (the right key, the valid options): fix the call and try again. " +
			"Tools other than the built-in ones were made by the collections' owners for common questions: prefer them when they fit.",
	})
	t := tools{app}
	mcp.AddTool(s, &mcp.Tool{Name: "workspace_overview", Description: "Start here. Every collection this API key reaches in one call: " +
		"id, name, description, access, records and records in the trash, the newest record change, its fields " +
		"(key, name, type, required, unique, options, the collection a reference points at, formulas, lookups, rollups, " +
		"and the subfields of groups and lists described the same way) " +
		"and the fields of other collections that reference it.",
		Annotations: readOnly}, t.workspaceOverview)
	mcp.AddTool(s, &mcp.Tool{Name: "get_collection", Description: "A collection's full JSON Schema: limits, formats, defaults and nested fields. workspace_overview already lists every field; read this only when that is not enough.",
		Annotations: readOnly}, t.getCollection)
	mcp.AddTool(s, &mcp.Tool{Name: "search_records", Description: "Find records of a collection by rules on its fields (and free text), newest change first unless sorted. Returns at most 50 per page and the total; " +
		"read the next pages while more remain. To list the records whose field status is Pago, send rules [{\"field\":\"status\",\"op\":\"eq\",\"value\":\"Pago\"}], " +
		"not search: search matches the text in any field. Pass fields with the keys you need to keep the answer short, and expand with reference fields to bring the records they point at.",
		Annotations: readOnly}, t.searchRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "get_record", Description: "One record by id: its data, version and dates.",
		Annotations: readOnly}, t.getRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "related_records", Description: "Records of other collections whose reference fields point at this record, grouped by collection and field, newest few of each with the total.",
		Annotations: readOnly}, t.relatedRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "collection_stats", Description: "A collection's activity: records, members, last change, records created per day and top contributors over the last 30 days (UTC). To count or sum records by a field, use aggregate_records.",
		Annotations: readOnly}, t.collectionStats)

	// Writing tools: the key's write scope and the member's editor role apply (services check both).
	mcp.AddTool(s, &mcp.Tool{Name: "create_record", Description: "Create a record in a collection the key may write. data maps field keys to values and must follow the collection's schema; fields the server fills (auto numbers, formulas, created_by) are left out.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false)}}, t.createRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "update_record", Description: "Change some fields of one record, keeping the others: set maps field keys to new values, null removes a field. The result must still follow the schema. Pass the version you read to refuse the change if someone else changed the record since.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false), IdempotentHint: true}}, t.updateRecord)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_records", Description: "Move up to 100 records of one collection to the trash, all or none; a record of another collection refuses the whole call. An owner can restore them from the trash.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(true), IdempotentHint: true}}, t.deleteRecords)

	t.addRecordTools(s)
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

func access(canWrite bool) string {
	if canWrite {
		return "write"
	}
	return "read"
}

// overviewField is a field with the name of the collection a reference points at,
// left out when the key does not reach it.
type overviewField struct {
	schema.Field
	TargetName string `json:"target_name,omitempty"`
}

// inbound is a field of another collection that points at this one.
type inbound struct {
	CollectionID string `json:"collection_id"`
	Collection   string `json:"collection"`
	Field        string `json:"field"`
	Many         bool   `json:"many,omitempty"` // a list of references
}

type overviewItem struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	Access           string          `json:"access"` // read or write
	Role             string          `json:"role"`   // the key maker's role: viewer, editor or owner
	Records          int64           `json:"records"`
	InTrash          int64           `json:"in_trash"`
	LastRecordChange *time.Time      `json:"last_record_change,omitempty"`
	SchemaChangedAt  time.Time       `json:"schema_changed_at"`
	Fields           []overviewField `json:"fields"`
	ReferencedBy     []inbound       `json:"referenced_by,omitempty"`
}

func (t tools) workspaceOverview(ctx context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	key, _ := reqctx.Key(ctx)
	ids := make([]string, 0, len(key.Scopes))
	for id := range key.Scopes {
		ids = append(ids, id)
	}
	cs, err := t.app.Collections.Overview(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	names := make(map[string]string, len(cs))
	for _, c := range cs {
		names[c.ID] = c.Name
	}
	items := make([]overviewItem, 0, len(cs))
	at := make(map[string]int, len(cs))
	for _, c := range cs {
		fields := []overviewField{}
		for _, f := range schema.Fields(c.Schema) {
			fields = append(fields, overviewField{Field: f, TargetName: names[f.Target]})
		}
		at[c.ID] = len(items)
		items = append(items, overviewItem{
			ID: c.ID, Name: c.Name, Description: c.Description, Access: access(key.Scopes[c.ID]), Role: string(c.Role),
			Records: c.RecordCount, InTrash: c.TrashCount, LastRecordChange: c.LastRecordChange, SchemaChangedAt: c.UpdatedAt, Fields: fields,
		})
	}
	// Who points at whom, among the reachable collections only.
	for _, c := range items {
		for _, f := range c.Fields {
			if i, ok := at[f.Target]; ok && (f.Type == "reference" || f.Type == "references") {
				items[i].ReferencedBy = append(items[i].ReferencedBy, inbound{CollectionID: c.ID, Collection: c.Name, Field: f.Key, Many: f.Type == "references"})
			}
		}
	}
	return nil, map[string]any{"collections": items}, nil
}

type collectionIn struct {
	CollectionID string `json:"collection_id" jsonschema:"the collection's id, from workspace_overview"`
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
	Field string `json:"field" jsonschema:"a field key from workspace_overview (the key, not the display name), or $created_at / $updated_at for the record's own dates"`
	Op    string `json:"op" jsonschema:"how to compare. Any field: eq, neq, in and not_in (value is a list), empty and not_empty (no value). Text: contains, not_contains, starts_with (ignoring case). Numbers and dates: gt, gte, lt, lte, between (value is [from, to]). Lists (text_list, references): has (holds the value), has_any (holds one of the values)."`
	Value any    `json:"value,omitempty" jsonschema:"the value as stored: text in the same case and accents, an option exactly as listed in workspace_overview, a number as a number. A date is \"YYYY-MM-DD\" or {\"today\": n}, n days from today in UTC (-7 is a week ago); it matches date-times by day. Left out for empty and not_empty."`
}

type groupIn struct {
	Any   bool     `json:"any,omitempty" jsonschema:"join this group's rules by OR instead of AND"`
	Rules []ruleIn `json:"rules"`
}

// filterIn picks records of one collection; search_records and the tools that sum up or change records share it.
type filterIn struct {
	CollectionID string    `json:"collection_id" jsonschema:"the collection's id, from workspace_overview"`
	Search       string    `json:"search,omitempty" jsonschema:"free text found anywhere in the record, in any field (a description mentioning it matches too). To keep records whose field has a value, use rules instead: field status eq Pago is {\"field\":\"status\",\"op\":\"eq\",\"value\":\"Pago\"}"`
	Rules        []ruleIn  `json:"rules,omitempty" jsonschema:"rules on fields, joined by AND (or OR when any is set); the way to filter by a field's value, e.g. [{\"field\":\"status\",\"op\":\"eq\",\"value\":\"Pago\"}] or with op in and a list of values"`
	Groups       []groupIn `json:"groups,omitempty" jsonschema:"bracketed sets of rules, joined with the rules by any: (a AND b) OR (c AND d)"`
	Any          bool      `json:"any,omitempty" jsonschema:"join rules and groups by OR instead of AND"`
}

// filter is the record filter the input describes, without paging or sorting.
func (in filterIn) filter() (domain.RecordFilter, error) {
	where, err := whereOf(in)
	return domain.RecordFilter{CollectionID: in.CollectionID, Search: in.Search, Where: where}, err
}

// fieldsIn names the data fields a tool returns, so a listing costs the model only what it reads.
type fieldsIn struct {
	Fields []string `json:"fields,omitempty" jsonschema:"field keys to return in each record's data, e.g. [\"name\",\"status\"]; left out: all of them. Ask only for the fields you will use when listing or counting. id, version and dates always come back."`
}

const maxFields = 50

// keep checks the asked fields and cuts each record's data to them; the records share a collection the call already names.
func (in fieldsIn) keep(rs []domain.Record) ([]recordOut, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	out := recordsOf(rs)
	for i := range out {
		out[i].CollectionID, out[i].Data = "", domain.KeepFields(out[i].Data, in.Fields)
	}
	return out, nil
}

func (in fieldsIn) check() error {
	if len(in.Fields) > maxFields {
		return apperror.New(apperror.CodeInvalidInput, "fields holds up to 50 names").WithField("fields")
	}
	for _, f := range in.Fields {
		if f == "" || len(f) > 200 {
			return apperror.New(apperror.CodeInvalidInput, "field names are 1-200 characters").WithField("fields")
		}
	}
	return nil
}

type searchIn struct {
	filterIn
	fieldsIn
	expandIn
	Sort  string `json:"sort,omitempty" jsonschema:"created_at, updated_at (default) or data.<field key>"`
	Order string `json:"order,omitempty" jsonschema:"asc or desc (default desc)"`
	Page  int    `json:"page,omitempty" jsonschema:"page number, from 1"`
	Limit int    `json:"limit,omitempty" jsonschema:"records per page, 1 to 50 (default 20)"`
}

type recordOut struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id,omitempty"` // left out in a listing of the collection the call names
	Data         json.RawMessage `json:"data"`
	Version      int             `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// Expanded holds the records each asked reference field points at, by field key.
	Expanded map[string][]refOut `json:"expanded,omitempty"`
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

func whereOf(in filterIn) (*domain.Where, error) {
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
	filter, err := in.filter()
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
	filter.Sort, filter.Order, filter.Page = sort, order, domain.NewPage(in.Page, min(limit, maxRecords))
	if err := t.checkFields(ctx, in.CollectionID, filter.Where, fieldUse{"fields", in.Fields}, fieldUse{"sort", one(filter.SortField())}); err != nil {
		return nil, nil, err
	}
	items, total, err := t.app.Records.List(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	records, err := in.keep(items)
	if err != nil {
		return nil, nil, err
	}
	if err := t.expand(ctx, in.CollectionID, in.Expand, records, items); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"total": total, "page": filter.Page.Number, "records": records}, nil
}

type recordIn struct {
	RecordID string `json:"record_id" jsonschema:"the record's id"`
}

type getRecordIn struct {
	recordIn
	fieldsIn
}

func (t tools) getRecord(ctx context.Context, _ *mcp.CallToolRequest, in getRecordIn) (*mcp.CallToolResult, any, error) {
	if err := in.check(); err != nil {
		return nil, nil, err
	}
	r, err := t.app.Records.Get(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	touch(ctx, r.CollectionID)
	if err := t.checkFields(ctx, r.CollectionID, nil, fieldUse{"fields", in.Fields}); err != nil {
		return nil, nil, err
	}
	out := recordOf(*r)
	out.Data = domain.KeepFields(out.Data, in.Fields)
	return nil, out, nil
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
	CollectionID string         `json:"collection_id" jsonschema:"the collection's id, from workspace_overview"`
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
	Set      map[string]any `json:"set" jsonschema:"field keys and their new values; null removes the field"`
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
	CollectionID string   `json:"collection_id" jsonschema:"the collection's id, from workspace_overview; every record must be in it"`
	RecordIDs    []string `json:"record_ids" jsonschema:"1 to 100 record ids"`
}

func (t tools) deleteRecords(ctx context.Context, _ *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, any, error) {
	if err := t.app.Records.DeleteManyIn(ctx, in.CollectionID, in.RecordIDs); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"deleted": len(in.RecordIDs)}, nil
}
