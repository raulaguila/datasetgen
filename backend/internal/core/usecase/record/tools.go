package record

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

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
	maxImport     = 1000 // records in one import request; the body limit caps it sooner
	maxImportErrs = 100
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

// Import checks every record against the schema and, unless DryRun, inserts
// the valid ones; with any invalid and not SkipInvalid it inserts none. Editors.
func (s *Service) Import(ctx context.Context, cmd input.ImportRecordsCommand) (*input.ImportResult, error) {
	if err := validate.ID(cmd.CollectionID); err != nil {
		return nil, err
	}
	if len(cmd.Records) == 0 || len(cmd.Records) > maxImport {
		return nil, apperror.New(apperror.CodeInvalidInput, "records must hold 1 to 1000 records").WithField("records")
	}
	res := &input.ImportResult{Errors: []input.ImportError{}}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), cmd.CollectionID, domain.RoleEditor); err != nil {
			return err
		}
		raw, err := uow.Collections().SchemaForWrite(ctx, cmd.CollectionID)
		if err != nil {
			return err
		}
		compiled, err := schema.Compile(raw) // once for the whole batch
		if err != nil {
			return err
		}
		uniques := schema.UniqueKeys(raw)
		// Upsert: a row whose key value a record holds updates that record.
		existing := map[int]domain.Record{}
		if cmd.UpsertKey != "" {
			if !slices.Contains(uniques, cmd.UpsertKey) {
				return apperror.New(apperror.CodeInvalidInput, "upsert_key must be a unique field").WithField("upsert_key")
			}
			if existing, err = upsertTargets(ctx, uow, cmd.CollectionID, cmd.UpsertKey, cmd.Records); err != nil {
				return err
			}
		}
		// The server's own fields: new rows get the next number and their maker.
		var fresh, again [][]byte
		var freshAt, againAt []int
		for i, d := range cmd.Records {
			if _, ok := existing[i]; ok {
				again, againAt = append(again, d), append(againAt, i)
			} else {
				fresh, freshAt = append(fresh, d), append(freshAt, i)
			}
		}
		if err := fill(ctx, uow, raw, cmd.CollectionID, fresh, true); err != nil {
			return err
		}
		if err := fill(ctx, uow, raw, cmd.CollectionID, again, false); err != nil {
			return err
		}
		for n, i := range freshAt {
			cmd.Records[i] = fresh[n]
		}
		for n, i := range againAt {
			cmd.Records[i] = again[n]
		}
		// Schema first; the records that pass are then checked for unique values,
		// against the collection and each other.
		passed := make([]int, 0, len(cmd.Records))
		docs := make([][]byte, 0, len(cmd.Records))
		bad := map[int][]schema.Issue{}
		for i, data := range cmd.Records {
			issues, err := schema.Check(compiled, data)
			if err != nil {
				return err
			}
			if len(issues) > 0 {
				bad[i] = issues
				continue
			}
			passed = append(passed, i)
			docs = append(docs, data)
		}
		// New rows are checked together; an updated record against the others only.
		// shortcut: one query per updated record and unique field; batch them if upserts of thousands get slow.
		var newDocs [][]byte
		var newAt []int
		for n, i := range passed {
			if rec, ok := existing[i]; ok {
				taken, err := takenIn(ctx, uow, cmd.CollectionID, rec.ID, uniques, [][]byte{docs[n]})
				if err != nil {
					return err
				}
				if len(taken[0]) > 0 {
					bad[i] = taken[0]
				}
				continue
			}
			newDocs, newAt = append(newDocs, docs[n]), append(newAt, i)
		}
		taken, err := takenIn(ctx, uow, cmd.CollectionID, "", uniques, newDocs)
		if err != nil {
			return err
		}
		for n, issues := range taken {
			if len(issues) > 0 {
				bad[newAt[n]] = issues
			}
		}
		missing, err := linksIn(ctx, uow, raw, cmd.CollectionID, docs)
		if err != nil {
			return err
		}
		for n, issues := range missing {
			if len(issues) > 0 {
				bad[passed[n]] = append(bad[passed[n]], issues...)
			}
		}
		valid := make([]string, 0, len(cmd.Records))
		var updates []int
		for i, data := range cmd.Records {
			if issues, ok := bad[i]; ok {
				res.Invalid++
				if len(res.Errors) < maxImportErrs {
					res.Errors = append(res.Errors, input.ImportError{Index: i, Issues: issues})
				}
				continue
			}
			if _, ok := existing[i]; ok {
				updates = append(updates, i)
				continue
			}
			valid = append(valid, string(data))
		}
		if cmd.DryRun || len(valid)+len(updates) == 0 || (res.Invalid > 0 && !cmd.SkipInvalid) {
			return nil
		}
		if len(valid) > 0 {
			if res.Created, err = uow.Records().CreateMany(ctx, cmd.CollectionID, valid); err != nil {
				return err
			}
		}
		for _, i := range updates {
			updated, err := uow.Records().Update(ctx, existing[i].ID, cmd.Records[i], nil)
			if err != nil {
				return err
			}
			res.Updated++
			if err := policy.Notify(ctx, uow, cmd.CollectionID, domain.EventRecordUpdated, hookData(updated)); err != nil {
				return err
			}
		}
		if err := policy.Audit(ctx, uow, "record.import", cmd.CollectionID, map[string]any{"collection_id": cmd.CollectionID, "count": res.Created, "updated": res.Updated}); err != nil {
			return err
		}
		if res.Created == 0 {
			return nil
		}
		return policy.Notify(ctx, uow, cmd.CollectionID, domain.EventRecordsImported, map[string]any{"count": res.Created})
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// upsertTargets finds the records holding each row's key value and merges the
// row into a copy of that record's data (in place in rows): the fields the row
// leaves out stay as they were.
func upsertTargets(ctx context.Context, uow output.UnitOfWork, collectionID, key string, rows []json.RawMessage) (map[int]domain.Record, error) {
	var values []json.RawMessage
	var at []int
	for i, row := range rows {
		var doc map[string]json.RawMessage
		if json.Unmarshal(row, &doc) != nil || doc[key] == nil || string(doc[key]) == "null" {
			continue
		}
		values, at = append(values, doc[key]), append(at, i)
	}
	found, err := uow.Records().Matching(ctx, collectionID, key, values)
	if err != nil {
		return nil, err
	}
	out := map[int]domain.Record{}
	for n, rec := range found {
		i := at[n]
		var base, row map[string]json.RawMessage
		if json.Unmarshal(rec.Data, &base) != nil || base == nil {
			base = map[string]json.RawMessage{}
		}
		_ = json.Unmarshal(rows[i], &row) // an object: values found its key
		maps.Copy(base, row)
		merged, err := json.Marshal(base)
		if err != nil {
			return nil, err
		}
		rows[i] = merged
		out[i] = rec
	}
	return out, nil
}

// Export checks access and reads the collection's name, schema and keys now;
// its Each then streams the records in id order, in batches, so an export of
// any size holds one batch in memory. ValidOnly leaves out records that break
// the schema. Viewers.
func (s *Service) Export(ctx context.Context, collectionID string, validOnly bool) (*input.Export, error) {
	if _, err := policy.CollectionRole(ctx, s.Collections, collectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	c, err := s.Collections.FindByID(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	compiled, err := schema.Compile(c.Schema)
	if err != nil {
		return nil, err
	}
	keys, err := s.Repo.Keys(ctx, collectionID, maxExportKeys)
	if err != nil {
		return nil, err
	}
	each := func(fn func(json.RawMessage) error) error {
		after := ""
		for {
			batch, err := s.Repo.Scan(ctx, collectionID, after, exportBatch)
			if err != nil {
				return err
			}
			for _, r := range batch {
				if validOnly {
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
	return &input.Export{Name: c.Name, Schema: c.Schema, Keys: keys, Each: each}, nil
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
		if len(ids) > 1 {
			for _, k := range schema.UniqueKeys(raw) {
				if v, ok := cmd.Set[k]; ok && string(v) != "null" {
					return apperror.New(apperror.CodeInvalidInput, "a unique field cannot be filled in bulk; edit its records one by one").WithField("set")
				}
			}
		}
		compiled, err := schema.Compile(raw)
		if err != nil {
			return err
		}
		docs := make([][]byte, len(ids))
		for i, id := range ids {
			current, err := uow.Records().FindByID(ctx, id)
			if err != nil {
				return err
			}
			var data map[string]json.RawMessage
			if err := json.Unmarshal(current.Data, &data); err != nil || data == nil {
				data = map[string]json.RawMessage{}
			}
			for k, v := range cmd.Set {
				if string(v) == "null" {
					delete(data, k)
				} else {
					data[k] = v
				}
			}
			if docs[i], err = json.Marshal(data); err != nil {
				return err
			}
		}
		// Formulas and "changed by" follow the new values.
		if err := fill(ctx, uow, raw, collectionID, docs, false); err != nil {
			return err
		}
		for i, id := range ids {
			if issues, err := schema.Check(compiled, docs[i]); err != nil || len(issues) > 0 {
				if err == nil {
					err = apperror.New(apperror.CodeValidationFailed, "a record would break the collection's fields").
						WithField("set").WithDetails("issues", issues).WithDetails("id", id)
				}
				return err
			}
		}
		if len(ids) == 1 { // one record may take a unique value nobody else holds
			taken, err := takenIn(ctx, uow, collectionID, ids[0], schema.UniqueKeys(raw), docs)
			if err != nil {
				return err
			}
			if len(taken[0]) > 0 {
				return apperror.New(apperror.CodeValidationFailed, "a unique field repeats another record's value").WithField("set").WithDetails("issues", taken[0])
			}
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

const (
	maxWhereRules   = 20 // across the groups too
	maxWhereGroups  = 10
	maxAggregateNum = 5
	maxStatsFields  = 60 // per part of a stats query
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
		if _, isList := v.([]any); (c.Op == domain.OpIn) != isList {
			return bad("in takes an array; other operators a single value")
		}
		if _, isText := v.(string); c.Op == domain.OpContains && !isText {
			return bad("contains takes text")
		}
	}
	return nil
}

// Aggregate counts the selected records, by a group field or all together,
// with sums and averages of up to five numeric fields.
func (s *Service) Aggregate(ctx context.Context, filter domain.RecordFilter, group string, numeric []string) ([]domain.RecordGroup, error) {
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
	if _, err := policy.CollectionRole(ctx, s.Collections, filter.CollectionID, domain.RoleViewer); err != nil {
		return nil, err
	}
	return s.Repo.Aggregate(ctx, filter, group, numeric)
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
	return s.Repo.Stats(ctx, filter, q)
}
