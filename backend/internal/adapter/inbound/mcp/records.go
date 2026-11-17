package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// maxBulk caps the records one update_records or restore_records call changes, as the record service does.
const maxBulk = 100

// addRecordTools adds the tools that sum up, trace and change many records at once.
func (t tools) addRecordTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "aggregate_records", Description: "Count the matching records and sum, average, min and max up to 5 number fields, " +
		"in total or per value of one field (group_by; a list field counts each element), or per day, week, month or year of a date field (bucket), biggest groups first, at most 100 groups. Use it to count or add up records instead of reading them; for a collection's activity over time, collection_stats.",
		Annotations: readOnly}, t.aggregateRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "field_values", Description: "Every value one field holds over the matching records, commonest first, with how many records hold each (a list field counts each element) " +
		"(null: records without it), a page at a time; also how many records fill the field and how many values differ. " +
		"Read it to learn what a text field holds before filtering on it with eq or in.",
		Annotations: readOnly}, t.fieldValues)
	mcp.AddTool(s, &mcp.Tool{Name: "get_records", Description: "Up to 50 records of one collection by id in one call; ids not found come back in missing.",
		Annotations: readOnly}, t.getRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "record_history", Description: "A record's versions, newest first, with the fields each one changed (from and to) and when; " +
		"the oldest version listed comes with its whole data. Up to the last 50 changes.",
		Annotations: readOnly}, t.recordHistory)
	mcp.AddTool(s, &mcp.Tool{Name: "list_views", Description: "The saved views of a collection the key's member may see (their own and the shared ones): " +
		"name, search text, rules (the same shape search_records takes) and sort.",
		Annotations: readOnly}, t.listViews)
	mcp.AddTool(s, &mcp.Tool{Name: "search_everywhere", Description: "Find free text in every collection this key reaches at once, when you do not know which collection holds it: " +
		"per collection with matches, how many and the newest 3. Then search that collection with search_records.",
		Annotations: readOnly}, t.searchEverywhere)
	mcp.AddTool(s, &mcp.Tool{Name: "list_trash", Description: "The newest 50 records in a collection's trash, with when and by whom they were deleted. Needs write access.",
		Annotations: readOnly}, t.listTrash)

	mcp.AddTool(s, &mcp.Tool{Name: "create_records", Description: "Create up to 100 records in one collection at once, all or none: " +
		"if one breaks the schema, none is made and the error lists each bad record by its index. Each record maps field keys to values, as in create_record.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false)}}, t.createRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "update_records", Description: "Set the same top-level fields on every record matching search and rules (null removes a field), " +
		"up to 100 records, all or none. Run it first with dry_run to see how many records match and a few of them. " +
		"A filter is required; a unique field cannot be set this way.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(true), IdempotentHint: true}}, t.updateRecords)
	mcp.AddTool(s, &mcp.Tool{Name: "restore_records", Description: "Put up to 100 records back from a collection's trash (list_trash gives their ids), all or none; " +
		"a unique value taken since refuses the call.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: pointer.Of(false)}}, t.restoreRecords)
}

type aggregateIn struct {
	filterIn
	GroupBy string   `json:"group_by,omitempty" jsonschema:"a field key: one row per value of it; left out for one total row"`
	Numbers []string `json:"numbers,omitempty" jsonschema:"up to 5 keys of number fields to sum, average, min and max"`
	Bucket  string   `json:"bucket,omitempty" jsonschema:"with group_by on a date field: day, week, month or year, one row per span keyed by its first day (YYYY-MM-DD)"`
}

func (t tools) aggregateRecords(ctx context.Context, _ *mcp.CallToolRequest, in aggregateIn) (*mcp.CallToolResult, any, error) {
	filter, err := in.filter()
	if err != nil {
		return nil, nil, err
	}
	if err := t.checkFields(ctx, in.CollectionID, filter.Where, fieldUse{"group_by", one(in.GroupBy)}, fieldUse{"numbers", in.Numbers}); err != nil {
		return nil, nil, err
	}
	groups, err := t.app.Records.Aggregate(ctx, filter, domain.GroupQuery{Group: in.GroupBy, Numeric: in.Numbers, Bucket: in.Bucket, Elements: in.Bucket == ""})
	if err != nil {
		return nil, nil, err
	}
	out := make([]map[string]any, len(groups))
	for i, g := range groups {
		row := map[string]any{"count": g.Count}
		if in.GroupBy != "" {
			row["value"] = g.Key
		}
		for _, f := range in.Numbers {
			row[f] = map[string]float64{"sum": g.Sums[f], "avg": g.Avgs[f], "min": g.Mins[f], "max": g.Maxs[f]}
		}
		out[i] = row
	}
	return nil, map[string]any{"groups": out}, nil
}

type fieldValuesIn struct {
	filterIn
	Field string `json:"field" jsonschema:"a field key from workspace_overview"`
	Page  int    `json:"page,omitempty" jsonschema:"page number, from 1"`
	Limit int    `json:"limit,omitempty" jsonschema:"values per page, 1 to 100 (default 20)"`
}

func (t tools) fieldValues(ctx context.Context, _ *mcp.CallToolRequest, in fieldValuesIn) (*mcp.CallToolResult, any, error) {
	if in.Field == "" {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "field is required").WithField("field")
	}
	filter, err := in.filter()
	if err != nil {
		return nil, nil, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	page := max(in.Page, 1)
	q := domain.StatsQuery{Filled: []string{in.Field}, Values: []string{in.Field}, Top: limit, Skip: (page - 1) * limit, Elements: true}
	if err := t.checkFields(ctx, in.CollectionID, filter.Where, fieldUse{"field", one(in.Field)}); err != nil {
		return nil, nil, err
	}
	st, err := t.app.Records.Stats(ctx, filter, q)
	if err != nil {
		return nil, nil, err
	}
	values := []map[string]any{}
	var filled, distinct int64
	if f := st.Fields[in.Field]; f != nil {
		for _, g := range f.Top {
			values = append(values, map[string]any{"value": g.Key, "count": g.Count})
		}
		filled, distinct = f.Filled, f.Distinct
	}
	return nil, map[string]any{"records": st.Total, "filled": filled, "distinct": distinct, "page": page,
		"values": values, "more": int64(q.Skip+len(values)) < distinct}, nil
}

type idsIn struct {
	CollectionID string   `json:"collection_id" jsonschema:"the collection's id, from workspace_overview"`
	RecordIDs    []string `json:"record_ids" jsonschema:"record ids of that collection"`
}

type getRecordsIn struct {
	idsIn
	fieldsIn
	expandIn
}

func (t tools) getRecords(ctx context.Context, _ *mcp.CallToolRequest, in getRecordsIn) (*mcp.CallToolResult, any, error) {
	if len(in.RecordIDs) == 0 || len(in.RecordIDs) > maxRecords {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "record_ids must hold 1 to 50 ids").WithField("record_ids")
	}
	if err := t.checkFields(ctx, in.CollectionID, nil, fieldUse{"fields", in.Fields}); err != nil {
		return nil, nil, err
	}
	items, _, err := t.app.Records.List(ctx, domain.RecordFilter{CollectionID: in.CollectionID, IDs: in.RecordIDs, Page: domain.NewPage(1, maxRecords)})
	if err != nil {
		return nil, nil, err
	}
	found := map[string]bool{}
	for _, r := range items {
		found[r.ID] = true
	}
	missing := []string{}
	for _, id := range in.RecordIDs {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	records, err := in.keep(items)
	if err != nil {
		return nil, nil, err
	}
	if err := t.expand(ctx, in.CollectionID, in.Expand, records, items); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"records": records, "missing": missing}, nil
}

type change struct {
	From json.RawMessage `json:"from"` // null when the field was added
	To   json.RawMessage `json:"to"`   // null when the field was removed
}

type versionOut struct {
	Version int               `json:"version"`
	SavedAt time.Time         `json:"saved_at"`
	Changes map[string]change `json:"changes,omitempty"`
	Data    json.RawMessage   `json:"data,omitempty"` // the oldest version listed only
}

// changes lists the top-level fields that differ between two versions of a record's data.
func changes(before, after json.RawMessage) map[string]change {
	var a, b map[string]json.RawMessage
	_ = json.Unmarshal(before, &a)
	_ = json.Unmarshal(after, &b)
	out := map[string]change{}
	for k, v := range b {
		if old, ok := a[k]; !ok || !jsonEqual(old, v) {
			out[k] = change{From: a[k], To: v}
		}
	}
	for k, v := range a {
		if _, ok := b[k]; !ok {
			out[k] = change{From: v}
		}
	}
	return out
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	return json.Compact(&x, a) == nil && json.Compact(&y, b) == nil && bytes.Equal(x.Bytes(), y.Bytes())
}

func (t tools) recordHistory(ctx context.Context, _ *mcp.CallToolRequest, in recordIn) (*mcp.CallToolResult, any, error) {
	current, err := t.app.Records.Get(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	touch(ctx, current.CollectionID)
	old, err := t.app.Records.Versions(ctx, in.RecordID)
	if err != nil {
		return nil, nil, err
	}
	// Saved versions are the data each change replaced, newest first; the current one leads.
	all := append([]domain.RecordVersion{{Version: current.Version, Data: current.Data, SavedAt: current.UpdatedAt}}, old...)
	out := make([]versionOut, len(all))
	for i, v := range all {
		out[i] = versionOut{Version: v.Version, SavedAt: v.SavedAt}
		if i == len(all)-1 {
			out[i].Data = v.Data
		} else {
			out[i].Changes = changes(all[i+1].Data, v.Data)
		}
	}
	return nil, map[string]any{"record_id": current.ID, "versions": out}, nil
}

type viewOut struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Shared bool            `json:"shared"`
	Mine   bool            `json:"mine"`
	Search string          `json:"search,omitempty"`
	Where  json.RawMessage `json:"where,omitempty"`
	Sort   string          `json:"sort,omitempty"`
	Order  string          `json:"order,omitempty"`
}

func (t tools) listViews(ctx context.Context, _ *mcp.CallToolRequest, in collectionIn) (*mcp.CallToolResult, any, error) {
	views, err := t.app.Tools.Views(ctx, in.CollectionID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]viewOut, len(views))
	for i, v := range views {
		q, _ := url.ParseQuery(v.Query)
		out[i] = viewOut{ID: v.ID, Name: v.Name, Shared: v.Shared, Mine: v.Mine, Search: q.Get("q"), Sort: q.Get("sort"), Order: q.Get("order")}
		if w := q.Get("where"); json.Valid([]byte(w)) {
			out[i].Where = json.RawMessage(w)
		}
	}
	return nil, map[string]any{"views": out}, nil
}

const trashShown = 50

type trashIn struct {
	collectionIn
	fieldsIn
}

func (t tools) listTrash(ctx context.Context, _ *mcp.CallToolRequest, in trashIn) (*mcp.CallToolResult, any, error) {
	if err := in.check(); err != nil {
		return nil, nil, err
	}
	if err := t.checkFields(ctx, in.CollectionID, nil, fieldUse{"fields", in.Fields}); err != nil {
		return nil, nil, err
	}
	items, err := t.app.Tools.TrashedRecords(ctx, in.CollectionID)
	if err != nil {
		return nil, nil, err
	}
	more := len(items) > trashShown
	items = items[:min(len(items), trashShown)]
	out := make([]map[string]any, len(items))
	for i, r := range items {
		out[i] = map[string]any{"id": r.ID, "data": domain.KeepFields(r.Data, in.Fields), "deleted_at": r.DeletedAt, "deleted_by": r.DeletedByName}
	}
	return nil, map[string]any{"records": out, "more": more}, nil
}

type updateManyIn struct {
	filterIn
	Set    map[string]any `json:"set" jsonschema:"field keys to set on every matching record and their values; null removes the field"`
	DryRun bool           `json:"dry_run,omitempty" jsonschema:"count the matching records and show a few, changing nothing"`
}

func (t tools) updateRecords(ctx context.Context, _ *mcp.CallToolRequest, in updateManyIn) (*mcp.CallToolResult, any, error) {
	if len(in.Set) == 0 {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "set names no field").WithField("set")
	}
	set := make(map[string]json.RawMessage, len(in.Set))
	for k, v := range in.Set {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, nil, apperror.New(apperror.CodeInvalidInput, "a value in set is not JSON").WithField("set")
		}
		set[k] = raw
	}
	filter, err := in.filter()
	if err != nil {
		return nil, nil, err
	}
	if filter.Where.Empty() && in.Search == "" { // a slip must not rewrite a whole collection
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "give search or rules to pick the records").WithField("rules")
	}
	if err := t.checkFields(ctx, in.CollectionID, filter.Where); err != nil {
		return nil, nil, err
	}
	filter.Page = domain.NewPage(1, maxBulk)
	items, total, err := t.app.Records.List(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	if in.DryRun {
		return nil, map[string]any{"matched": total, "sample": recordsOf(items[:min(len(items), 5)])}, nil
	}
	if total > maxBulk {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "more than 100 records match; narrow the rules and run it again").WithField("rules")
	}
	if total == 0 {
		return nil, map[string]any{"updated": 0}, nil
	}
	ids := make([]string, len(items))
	for i, r := range items {
		ids[i] = r.ID
	}
	n, err := t.app.Records.BulkUpdate(ctx, input.BulkUpdateCommand{IDs: ids, Set: set})
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"updated": n}, nil
}

func (t tools) restoreRecords(ctx context.Context, _ *mcp.CallToolRequest, in idsIn) (*mcp.CallToolResult, any, error) {
	if len(in.RecordIDs) == 0 || len(in.RecordIDs) > maxBulk {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "record_ids must hold 1 to 100 ids").WithField("record_ids")
	}
	n, err := t.app.Tools.RestoreRecords(ctx, in.CollectionID, in.RecordIDs, nil)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"restored": n}, nil
}

type createManyIn struct {
	CollectionID string           `json:"collection_id" jsonschema:"the collection's id, from workspace_overview"`
	Records      []map[string]any `json:"records" jsonschema:"1 to 100 records, each mapping field keys to values"`
}

func (t tools) createRecords(ctx context.Context, _ *mcp.CallToolRequest, in createManyIn) (*mcp.CallToolResult, any, error) {
	if len(in.Records) == 0 || len(in.Records) > maxBulk {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "records must hold 1 to 100 records").WithField("records")
	}
	rows := make([]json.RawMessage, len(in.Records))
	for i, r := range in.Records {
		raw, err := json.Marshal(r)
		if err != nil || r == nil {
			return nil, nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("record %d is not a JSON object", i)).WithField("records")
		}
		rows[i] = raw
	}
	res, err := t.app.Records.Import(ctx, input.ImportRecordsCommand{CollectionID: in.CollectionID, Records: rows})
	if err != nil {
		return nil, nil, err
	}
	if res.Invalid > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%d of %d records break the schema, so none was created:", res.Invalid, len(rows))
		for _, e := range res.Errors[:min(len(res.Errors), 10)] {
			for _, is := range e.Issues {
				fmt.Fprintf(&b, " record %d %s: %s;", e.Index, is.Path, is.Message)
			}
		}
		return nil, nil, apperror.New(apperror.CodeInvalidInput, b.String()).WithField("records")
	}
	return nil, map[string]any{"created": res.Created}, nil
}

type everywhereIn struct {
	Search string `json:"search" jsonschema:"the text to find, in any field of any record"`
}

const everywhereShown = 3

func (t tools) searchEverywhere(ctx context.Context, _ *mcp.CallToolRequest, in everywhereIn) (*mcp.CallToolResult, any, error) {
	if len(strings.TrimSpace(in.Search)) < 2 {
		return nil, nil, apperror.New(apperror.CodeInvalidInput, "search needs at least 2 characters").WithField("search")
	}
	key, _ := reqctx.Key(ctx)
	// shortcut: picks names from the member's first 1000 collections (Options); a key reaches at most 50.
	all, err := t.app.Collections.Options(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := []map[string]any{}
	// shortcut: one search per reachable collection (a key reaches at most 50); a single query across them if keys grow.
	for _, c := range all {
		if _, ok := key.Scopes[c.ID]; !ok {
			continue
		}
		items, total, err := t.app.Records.List(ctx, domain.RecordFilter{CollectionID: c.ID, Search: in.Search, Sort: "updated_at", Order: "desc", Page: domain.NewPage(1, everywhereShown)})
		if err != nil {
			if apperror.IsCode(err, apperror.CodeNotFound) || apperror.IsCode(err, apperror.CodeForbidden) {
				continue
			}
			return nil, nil, err
		}
		if total == 0 {
			continue
		}
		touch(ctx, c.ID)
		recs := recordsOf(items)
		for i := range recs {
			recs[i].CollectionID = ""
		}
		out = append(out, map[string]any{"collection_id": c.ID, "collection": c.Name, "total": total, "records": recs})
	}
	return nil, map[string]any{"collections": out}, nil
}
