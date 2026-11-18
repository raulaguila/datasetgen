package record

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Repeated upsert keys depend on the preceding accepted change. Earlier
// segments are staged in the same transaction, including for previews. Rolling
// it back preserves both dry-run semantics and all-or-nothing imports.
var errDiscardImport = errors.New("discard staged import")

func (s *Service) Import(ctx context.Context, cmd input.ImportRecordsCommand) (*input.ImportResult, error) {
	if err := validate.ID(cmd.CollectionID); err != nil {
		return nil, err
	}
	if len(cmd.Records) == 0 || len(cmd.Records) > maxImport {
		return nil, apperror.New(apperror.CodeInvalidInput, "records must hold 1 to 5000 records").WithField("records")
	}
	if len(cmd.BatchKey) > maxBatchKey || strings.ContainsFunc(cmd.BatchKey, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return nil, apperror.New(apperror.CodeInvalidInput, "batch_key is up to 100 printable characters").WithField("batch_key")
	}
	res := &input.ImportResult{Errors: []input.ImportError{}}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), cmd.CollectionID, domain.RoleEditor); err != nil {
			return err
		}
		claim := cmd.BatchKey != "" && !cmd.DryRun
		if claim {
			prev, err := uow.Records().ClaimImportBatch(ctx, cmd.CollectionID, cmd.BatchKey)
			if err != nil {
				return err
			}
			if prev != nil {
				res.Created, res.Updated, res.Replayed = prev.Created, prev.Updated, true
				return nil
			}
		}
		if err := runImport(ctx, uow, cmd, res); err != nil {
			return err
		}
		if claim {
			return uow.Records().FinishImportBatch(ctx, cmd.CollectionID, cmd.BatchKey, res.Created, res.Updated)
		}
		return nil
	})
	if errors.Is(err, errDiscardImport) {
		res.Created, res.Updated = 0, 0
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

type importBatch struct {
	collection string
	schema     []byte
	validator  *jsonschema.Schema
	unique     []string
	upsert     string
}

func runImport(ctx context.Context, uow output.UnitOfWork, cmd input.ImportRecordsCommand, res *input.ImportResult) error {
	raw, err := uow.Collections().SchemaForWrite(ctx, cmd.CollectionID)
	if err != nil {
		return err
	}
	validator, err := schema.Compile(raw)
	if err != nil {
		return err
	}
	batch := importBatch{collection: cmd.CollectionID, schema: raw, validator: validator, unique: schema.Metadata(raw).Unique, upsert: cmd.UpsertKey}
	if batch.upsert != "" && !slices.Contains(batch.unique, batch.upsert) {
		return apperror.New(apperror.CodeInvalidInput, "upsert_key must be a unique field").WithField("upsert_key")
	}
	if len(batch.unique) > 0 {
		if err := uow.Records().LockUnique(ctx, batch.collection); err != nil {
			return err
		}
	}
	matches, err := batch.matches(ctx, uow, cmd.Records)
	if err != nil {
		return err
	}
	start := 0
	for _, end := range segmentEnds(matches, len(cmd.Records)) {
		rows := slices.Clone(cmd.Records[start:end])
		if start > 0 {
			matches, err = batch.matches(ctx, uow, rows)
			if err != nil {
				return err
			}
		}
		existing := map[int]domain.Record{}
		for _, m := range matches {
			if m.Index < len(rows) && m.Record.ID != "" {
				existing[m.Index] = m.Record
			}
		}
		issues, err := batch.prepare(ctx, uow, rows, existing)
		if err != nil {
			return err
		}
		for i, problems := range issues {
			if len(problems) == 0 {
				continue
			}
			res.Invalid++
			if len(res.Errors) < maxImportErrs {
				res.Errors = append(res.Errors, input.ImportError{Index: start + i, Issues: problems})
			}
		}
		// Intermediate segments must be visible to subsequent matches. External
		// notifications are transactional too, but previews do not need them.
		if end < len(cmd.Records) || (!cmd.DryRun && (cmd.SkipInvalid || res.Invalid == 0)) {
			if err := batch.save(ctx, uow, rows, existing, issues, !cmd.DryRun, res); err != nil {
				return err
			}
		}
		start = end
	}
	if cmd.DryRun || (!cmd.SkipInvalid && res.Invalid > 0) {
		return errDiscardImport
	}
	if res.Created+res.Updated == 0 {
		return nil
	}
	if err := policy.Audit(ctx, uow, "record.import", batch.collection, map[string]any{"collection_id": batch.collection, "count": res.Created, "updated": res.Updated}); err != nil {
		return err
	}
	if res.Created > 0 {
		return policy.Notify(ctx, uow, batch.collection, domain.EventRecordsImported, map[string]any{"count": res.Created})
	}
	return nil
}

func segmentEnds(matches []output.RecordMatch, total int) []int {
	// A segment has at most one change to each upsert target.
	start := 0
	seen := map[int]int{}
	ends := []int{}
	for _, m := range matches {
		if last, ok := seen[m.Group]; ok && last >= start {
			ends = append(ends, m.Index)
			start = m.Index
		}
		seen[m.Group] = m.Index
	}
	return append(ends, total)
}

func (b importBatch) matches(ctx context.Context, uow output.UnitOfWork, rows []json.RawMessage) ([]output.RecordMatch, error) {
	if b.upsert == "" {
		return nil, nil
	}
	values := fieldValues(rows, []string{b.upsert})
	return uow.Records().Matching(ctx, b.collection, b.upsert, values[0])
}

func fieldValues(rows []json.RawMessage, keys []string) [][]json.RawMessage {
	values := make([][]json.RawMessage, len(keys))
	for k := range keys {
		values[k] = make([]json.RawMessage, len(rows))
	}
	for i, raw := range rows {
		var doc map[string]json.RawMessage
		if json.Unmarshal(raw, &doc) != nil {
			continue
		}
		for k, key := range keys {
			values[k][i] = doc[key]
		}
	}
	return values
}

func (b importBatch) prepare(ctx context.Context, uow output.UnitOfWork, rows []json.RawMessage, existing map[int]domain.Record) ([][]schema.Issue, error) {
	for i, r := range existing {
		var base, patch map[string]json.RawMessage
		if err := json.Unmarshal(r.Data, &base); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rows[i], &patch); err != nil {
			return nil, err
		}
		maps.Copy(base, patch)
		merged, err := json.Marshal(base)
		if err != nil {
			return nil, err
		}
		rows[i] = merged
	}
	for _, creating := range []bool{true, false} {
		var docs [][]byte
		var at []int
		for i, row := range rows {
			_, found := existing[i]
			if found != creating {
				docs = append(docs, row)
				at = append(at, i)
			}
		}
		if len(docs) == 0 {
			continue
		}
		if err := fill(ctx, uow, b.schema, b.collection, docs, creating); err != nil {
			return nil, err
		}
		for n, i := range at {
			rows[i] = docs[n]
		}
	}
	issues := make([][]schema.Issue, len(rows))
	big, err := uow.Records().Oversized(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, i := range big {
		issues[i] = []schema.Issue{{Keyword: "too_large", Message: "record data is over 1 MB"}}
	}
	var valid [][]byte
	var at []int
	for i, row := range rows {
		if len(issues[i]) > 0 {
			continue
		}
		issues[i], err = schema.Check(b.validator, row)
		if err != nil {
			return nil, err
		}
		if len(issues[i]) == 0 {
			valid = append(valid, row)
			at = append(at, i)
		}
	}
	missing, err := linksIn(ctx, uow, b.schema, b.collection, valid)
	if err != nil {
		return nil, err
	}
	for n, i := range at {
		issues[i] = missing[n]
	}
	err = checkImportUnique(ctx, uow.Records(), b.collection, b.unique, rows, existing, issues)
	return issues, err
}

type uniqueClaim struct {
	key   string
	group int
	owner string
}

// Each key gets one batched probe. Only accepted rows reserve values;
// an accepted update can also release its old value for a later row.
func checkImportUnique(ctx context.Context, repo output.RecordRepository, collection string, keys []string, rows []json.RawMessage, existing map[int]domain.Record, issues [][]schema.Issue) error {
	if len(keys) == 0 {
		return nil
	}
	values := fieldValues(rows, keys)
	claims := make([][]uniqueClaim, len(rows))
	for k, key := range keys {
		found, err := repo.UniqueValues(ctx, collection, key, values[k])
		if err != nil {
			return err
		}
		for _, v := range found {
			claims[v.Index] = append(claims[v.Index], uniqueClaim{key, v.Group, v.Owner})
		}
	}
	accepted := map[string]bool{}
	type fieldGroup struct {
		key   string
		group int
	}
	reserved := map[fieldGroup]bool{}
	for i, prior := range issues {
		row := claims[i]
		if len(prior) > 0 {
			continue
		}
		id := existing[i].ID
		for _, v := range row {
			if reserved[fieldGroup{v.key, v.group}] || (v.owner != "" && v.owner != id && !accepted[v.owner]) {
				issues[i] = append(issues[i], schema.UniqueIssue(v.key))
			}
		}
		if len(issues[i]) > 0 {
			continue
		}
		if id != "" {
			accepted[id] = true
		}
		for _, v := range row {
			reserved[fieldGroup{v.key, v.group}] = true
		}
	}
	return nil
}

func (b importBatch) save(ctx context.Context, uow output.UnitOfWork, rows []json.RawMessage, existing map[int]domain.Record, issues [][]schema.Issue, notify bool, res *input.ImportResult) error {
	var fresh []string
	changes := make([]output.RecordChange, 0, len(existing))
	for i, data := range rows {
		if len(issues[i]) != 0 {
			continue
		}
		if rec, ok := existing[i]; ok {
			changes = append(changes, output.RecordChange{ID: rec.ID, Version: rec.Version, Data: data})
		} else {
			fresh = append(fresh, string(data))
		}
	}
	updated, err := uow.Records().UpdateMany(ctx, changes)
	if err != nil {
		return err
	}
	res.Updated += int64(len(updated))
	if notify && len(updated) > 0 {
		events := make([]map[string]any, len(updated))
		for i := range updated {
			events[i] = hookData(&updated[i])
		}
		if err := policy.NotifyMany(ctx, uow, b.collection, domain.EventRecordUpdated, events); err != nil {
			return err
		}
	}
	if len(fresh) > 0 {
		n, err := uow.Records().CreateMany(ctx, b.collection, fresh)
		if err != nil {
			return err
		}
		res.Created += n
	}
	return nil
}
