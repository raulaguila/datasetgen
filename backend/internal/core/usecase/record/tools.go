package record

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxVersions   = 50
	maxImport     = 5000 // records in one import request; the 8 MB body limit caps it sooner
	maxImportErrs = 100
	maxBatchKey   = 100
	// maxExportKeys bounds the CSV columns; keys past it stay in the JSON Lines export.
	maxExportKeys = 1000
	exportBatch   = 1000
	maxMatchKeys  = 10
	maxMatchValue = 500
)

// Versions lists the record's earlier versions, newest first. Viewers.
func (s *Service) Versions(ctx context.Context, id string) ([]domain.RecordVersion, error) {
	if _, err := s.Get(ctx, id); err != nil { // checks the viewer role
		return nil, err
	}
	return s.Repo.Versions(ctx, id, maxVersions)
}

// Restore writes an earlier version back as a new version. The old data must
// match the current schema. Editors.
func (s *Service) Restore(ctx context.Context, cmd input.RestoreRecordCommand) (*domain.Record, error) {
	if err := validate.ID(cmd.ID); err != nil {
		return nil, err
	}
	var restored *domain.Record
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		current, err := uow.Records().FindByID(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := editor(ctx, uow, current.CollectionID); err != nil {
			return err
		}
		data, err := uow.Records().Version(ctx, cmd.ID, cmd.Version)
		if err != nil {
			return err
		}
		raw, data, err := checkData(ctx, uow, current.CollectionID, cmd.ID, data)
		if err != nil {
			return err
		}
		if restored, err = uow.Records().Update(ctx, cmd.ID, data, cmd.CurrentVersion); err != nil {
			return err
		}
		details := auditDetails(current.CollectionID, raw, data)
		details["version"] = cmd.Version
		if err := policy.Audit(ctx, uow, "record.restore", cmd.ID, details); err != nil {
			return err
		}
		return policy.Notify(ctx, uow, current.CollectionID, domain.EventRecordUpdated, hookData(restored))
	})
	return restored, err
}

// importBatchesKept is how long a saved batch key answers a retry.
const importBatchesKept = 7 * 24 * time.Hour

// PruneImportBatches drops batch keys older than a week; the job worker runs it hourly.
func (s *Service) PruneImportBatches(ctx context.Context) error {
	return s.Repo.PruneImportBatches(ctx, time.Now().Add(-importBatchesKept))
}

// Export checks access and reads the collection's name, schema and keys now;
// its Each then streams the records in id order, in batches, so an export of
// any size holds one batch in memory. ValidOnly leaves out records that break
// the schema. Viewers.
func (s *Service) Export(ctx context.Context, collectionID string, options input.ExportOptions) (*input.Export, error) {
	if _, err := policy.CollectionRole(ctx, s.Collections, collectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	c, err := s.Collections.FindByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	var compiled *jsonschema.Schema
	if options.ValidOnly {
		compiled, err = schema.Compile(c.Schema)
		if err != nil {
			return nil, err
		}
	}
	var keys []string
	if options.CSV {
		keys, err = s.Repo.Keys(ctx, collectionID, maxExportKeys)
		if err != nil {
			return nil, err
		}
	}

	each := func(fn func(json.RawMessage) error) error {
		after := ""
		for {
			batch, err := s.Repo.Scan(ctx, collectionID, after, exportBatch)
			if err != nil {
				return err
			}
			for _, r := range batch {
				if options.ValidOnly {
					issues, err := schema.Check(compiled, r.Data)
					if err != nil {
						return err
					}
					if len(issues) > 0 {
						continue
					}
				}
				if err := fn(r.Data); err != nil {
					return err
				}
			}
			if len(batch) < exportBatch {
				return nil
			}
			after = batch[len(batch)-1].ID
		}
	}
	return &input.Export{Name: c.Name, Description: c.Description, Icon: c.Icon, Schema: c.Schema, Keys: keys, Each: each}, nil
}

// checkMatch accepts a JSON object of up to 10 top-level fields with plain values.
func checkMatch(match json.RawMessage) error {
	if len(match) == 0 {
		return nil
	}
	bad := apperror.New(apperror.CodeInvalidInput, "filter must be a JSON object of up to 10 fields with text, number or true/false values").WithField("filter")
	var fields map[string]any
	if err := json.Unmarshal(match, &fields); err != nil || len(fields) > maxMatchKeys {
		return bad
	}
	for k, v := range fields {
		if k == "" || len(k) > 200 {
			return bad
		}
		switch v := v.(type) {
		case string:
			if len(v) > maxMatchValue {
				return bad
			}
		case float64, bool:
		case []any:
			// One item of a list of objects: [{"subfield": plain value}] (the backlinks of a list's references).
			item, ok := map[string]any(nil), len(v) == 1
			if ok {
				item, ok = v[0].(map[string]any)
			}
			if !ok || len(item) != 1 {
				return bad
			}
			for sk, sv := range item {
				if s, isText := sv.(string); sk == "" || len(sk) > 200 || !isText || len(s) > maxMatchValue {
					return bad
				}
			}
		default:
			return bad
		}
	}
	return nil
}

// BulkUpdate sets the same top-level values in every listed record of one
// collection (null removes the field); every record must still pass its
// schema, or none changes. Each keeps its previous version. Editors.
func (s *Service) BulkUpdate(ctx context.Context, cmd input.BulkUpdateCommand) (int, error) {
	if len(cmd.IDs) == 0 || len(cmd.IDs) > maxBatch {
		return 0, apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	if len(cmd.Set) == 0 || len(cmd.Set) > 50 {
		return 0, apperror.New(apperror.CodeInvalidInput, "set must hold 1 to 50 fields").WithField("set")
	}
	ids := slices.Compact(slices.Sorted(slices.Values(cmd.IDs)))
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return 0, err
		}
	}
	for k, v := range cmd.Set {
		if k == "" || len(k) > 200 || !json.Valid(v) {
			return 0, apperror.New(apperror.CodeInvalidInput, "set must map field names to JSON values").WithField("set")
		}
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		owners, err := uow.Records().CollectionsOf(ctx, ids)
		if err != nil {
			return err
		}
		if len(owners) != len(ids) {
			return apperror.New(apperror.CodeNotFound, "record not found")
		}
		collectionID := owners[ids[0]]
		for _, c := range owners {
			if c != collectionID {
				return apperror.New(apperror.CodeInvalidInput, "the records must belong to one collection").WithField("ids")
			}
		}
		if err := editor(ctx, uow, collectionID); err != nil {
			return err
		}
		raw, err := uow.Collections().SchemaForWrite(ctx, collectionID)
		if err != nil {
			return err
		}
		docs, err := prepareBulkUpdate(ctx, uow, collectionID, raw, ids, cmd.Set)
		if err != nil {
			return err
		}
		missing, err := linksIn(ctx, uow, raw, collectionID, docs)
		if err != nil {
			return err
		}
		for i, id := range ids {
			if len(missing[i]) > 0 {
				return apperror.New(apperror.CodeValidationFailed, "a value points at something that does not exist").
					WithField("set").WithDetails("issues", missing[i]).WithDetails("id", id)
			}
			updated, err := uow.Records().Update(ctx, id, docs[i], nil)
			if err != nil {
				return err
			}
			details := auditDetails(collectionID, raw, docs[i])
			details["bulk"] = true
			if err := policy.Audit(ctx, uow, "record.update", id, details); err != nil {
				return err
			}
			if err := policy.Notify(ctx, uow, collectionID, domain.EventRecordUpdated, hookData(updated)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func prepareBulkUpdate(ctx context.Context, uow output.UnitOfWork, collectionID string, raw []byte, ids []string, fields map[string]json.RawMessage) ([][]byte, error) {
	if len(ids) > 1 {
		for _, k := range schema.Metadata(raw).Unique {
			if v, ok := fields[k]; ok && string(v) != "null" {
				return nil, apperror.New(apperror.CodeInvalidInput, "a unique field cannot be filled in bulk; edit its records one by one").WithField("set")
			}
		}
	}
	compiled, err := schema.Compile(raw)
	if err != nil {
		return nil, err
	}
	docs := make([][]byte, len(ids))
	for i, id := range ids {
		current, err := uow.Records().FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(current.Data, &data); err != nil || data == nil {
			data = map[string]json.RawMessage{}
		}
		for k, v := range fields {
			if string(v) == "null" {
				delete(data, k)
			} else {
				data[k] = v
			}
		}
		if docs[i], err = json.Marshal(data); err != nil {
			return nil, err
		}
	}
	// Formulas and "changed by" follow the new values.
	if err := fill(ctx, uow, raw, collectionID, docs, false); err != nil {
		return nil, err
	}
	for i, id := range ids {
		if issues, err := schema.Check(compiled, docs[i]); err != nil || len(issues) > 0 {
			if err == nil {
				err = apperror.New(apperror.CodeValidationFailed, "a record would break the collection's fields").
					WithField("set").WithDetails("issues", issues).WithDetails("id", id)
			}
			return nil, err
		}
	}
	if len(ids) == 1 { // one record may take a unique value nobody else holds
		taken, err := uniqueIssues(ctx, uow, collectionID, ids[0], schema.Metadata(raw).Unique, docs[0])
		if err != nil {
			return nil, err
		}
		if len(taken) > 0 {
			return nil, apperror.New(apperror.CodeValidationFailed, "a unique field repeats another record's value").WithField("set").WithDetails("issues", taken)
		}
	}
	return docs, nil
}

const (
	maxWhereRules   = 20 // across the groups too
	maxWhereGroups  = 10
	maxAggregateNum = 5
	maxStatsFields  = 60 // per part of a stats query
	statsTop        = 8  // values a profile lists by default
	maxValuesPage   = 100
	maxValuesSkip   = 100_000
)

// checkWhere refuses unknown operators, odd field names and values too large
// to compare; a value must fit its operator (an array for in, none for empty).
func checkWhere(w *domain.Where) error {
	if w == nil {
		return nil
	}
	bad := func(msg string) error { return apperror.New(apperror.CodeInvalidInput, msg).WithField("where") }
	rules := w.Rules
	if len(w.Groups) > maxWhereGroups {
		return bad("where holds up to 10 groups")
	}
	for _, g := range w.Groups {
		if len(g.Groups) > 0 {
			return bad("groups do not nest")
		}
		rules = append(rules[:len(rules):len(rules)], g.Rules...)
	}
	if len(rules) > maxWhereRules {
		return bad("where holds up to 20 rules")
	}
	for _, c := range rules {
		if c.Field == "" || len(c.Field) > 200 || strings.ContainsRune(c.Field, 0) {
			return bad("each rule needs a field name")
		}
		if !slices.Contains(domain.WhereOps, c.Op) {
			return bad("unknown operator " + c.Op)
		}
		if len(c.Value) > maxMatchValue*4 {
			return bad("value too long")
		}
		if c.Op == domain.OpEmpty || c.Op == domain.OpNotEmpty {
			continue
		}
		var v any
		if len(c.Value) == 0 || json.Unmarshal(c.Value, &v) != nil {
			return bad("rule " + c.Field + " needs a JSON value")
		}
		list, isList := v.([]any)
		switch c.Op {
		case domain.OpIn, domain.OpNotIn, domain.OpHasAny:
			if !isList {
				return bad(c.Op + " takes an array")
			}
		case domain.OpBetween:
			if !isList || len(list) != 2 || !orderable(list[0]) || !orderable(list[1]) {
				return bad("between takes [from, to]: numbers, texts or dates")
			}
		case domain.OpContains, domain.OpNotContains, domain.OpStartsWith:
			if _, isText := v.(string); !isText {
				return bad(c.Op + " takes text")
			}
		default: // eq, neq, gt, gte, lt, lte, has
			if isList {
				return bad(c.Op + " takes a single value")
			}
			if m, isObj := v.(map[string]any); isObj && m["today"] != nil && isToday(v) == nil {
				return bad(`{"today": n} takes whole days, up to 3650 either way`)
			}
		}
	}
	return nil
}

// maxTodayDays bounds {"today": n}: ten years either way.
const maxTodayDays = 3650

// isToday reads {"today": n}, a date relative to the caller's today; nil when v is not one.
func isToday(v any) *int {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return nil
	}
	n, ok := m["today"].(float64)
	if !ok || n != float64(int(n)) || n < -maxTodayDays || n > maxTodayDays {
		return nil
	}
	days := int(n)
	return &days
}

// orderable is a value between can order: a number, a text or {"today": n}.
func orderable(v any) bool {
	switch v.(type) {
	case float64, string:
		return true
	}
	return isToday(v) != nil
}

// Aggregate counts the selected records, by a group field or all together,
// with sums and averages of up to five numeric fields.
func (s *Service) Aggregate(ctx context.Context, filter domain.RecordFilter, q domain.GroupQuery) ([]domain.RecordGroup, error) {
	group, numeric := q.Group, q.Numeric
	filter.Page = domain.NewPage(1, 1)
	if err := checkFilter(&filter); err != nil {
		return nil, err
	}
	if len(group) > 200 || len(numeric) > maxAggregateNum {
		return nil, apperror.New(apperror.CodeInvalidInput, "group one field; sum up to 5").WithField("fields")
	}
	for _, f := range numeric {
		if f == "" || len(f) > 200 {
			return nil, apperror.New(apperror.CodeInvalidInput, "field names are 1-200 characters").WithField("fields")
		}
	}
	if q.Bucket != "" && (group == "" || q.Elements || !slices.Contains(domain.StatsBuckets, q.Bucket)) {
		return nil, apperror.New(apperror.CodeInvalidInput, "bucket is day, week, month or year, with a date field to group by").WithField("bucket")
	}
	if _, err := policy.CollectionRole(ctx, s.Collections, filter.CollectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	var err error
	if filter.Where, err = s.resolveWhere(ctx, filter); err != nil {
		return nil, err
	}
	return s.Repo.Aggregate(ctx, filter, q)
}

// Stats profiles the selected records: how many fill each field, the range of
// numbers and dates, a date series and the commonest values, for viewers.
func (s *Service) Stats(ctx context.Context, filter domain.RecordFilter, q domain.StatsQuery) (*domain.RecordStats, error) {
	filter.Page = domain.NewPage(1, 1)
	if err := checkFilter(&filter); err != nil {
		return nil, err
	}
	bad := func(msg string) error { return apperror.New(apperror.CodeInvalidInput, msg).WithField("fields") }
	if q.Bucket == "" {
		q.Bucket = "month"
	}
	if q.Top == 0 {
		q.Top = statsTop
	}
	if q.Top < 1 || q.Top > maxValuesPage || q.Skip < 0 || q.Skip > maxValuesSkip {
		return nil, apperror.New(apperror.CodeInvalidInput, "values come 1 to 100 at a time, from the first 100 000").WithField("limit")
	}
	if !slices.Contains(domain.StatsBuckets, q.Bucket) {
		return nil, apperror.New(apperror.CodeInvalidInput, "bucket is day, week, month or year").WithField("bucket")
	}
	for _, part := range [][]string{q.Filled, q.Numbers, q.Dates, q.Values} {
		if len(part) > maxStatsFields {
			return nil, bad("up to 60 fields per part")
		}
		for _, f := range part {
			if f == "" || len(f) > 200 || strings.ContainsRune(f, 0) {
				return nil, bad("field names are 1-200 characters")
			}
		}
	}
	if _, err := policy.CollectionRole(ctx, s.Collections, filter.CollectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	var err error
	if filter.Where, err = s.resolveWhere(ctx, filter); err != nil {
		return nil, err
	}
	return s.Repo.Stats(ctx, filter, q)
}
