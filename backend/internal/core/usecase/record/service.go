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
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxSearchBytes = 120
	maxBatch       = 100
)

// Service checks every record against its collection's schema inside the
// transaction that writes it; the schema row stays share-locked until commit,
// so a concurrent schema change waits instead of slipping in between.
// Reading needs a viewer role in the record's collection, writing an editor role.
type Service struct {
	Repo        output.RecordRepository
	Collections output.CollectionRepository
	Tx          output.TxManager
}

var _ input.RecordService = (*Service)(nil)

func New(repo output.RecordRepository, collections output.CollectionRepository, tx output.TxManager) *Service {
	return &Service{Repo: repo, Collections: collections, Tx: tx}
}

func (s *Service) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	if err := checkFilter(&filter); err != nil {
		return nil, 0, err
	}
	filter.Page = domain.NewPage(filter.Page.Number, filter.Page.Limit)
	if _, err := policy.CollectionRole(ctx, s.Collections, filter.CollectionID, domain.RoleViewer); err != nil {
		return nil, 0, err
	}
	return s.Repo.List(ctx, filter)
}

// checkFilter checks and normalizes what selects records: search, sort, ids and rules.
func checkFilter(filter *domain.RecordFilter) error {
	if err := validate.ID(filter.CollectionID); err != nil {
		return apperror.New(apperror.CodeInvalidInput, "collection_id is required").WithField("collection_id")
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return err
	}
	if field := filter.SortField(); field != "" {
		// Any top-level field; the value is a bind parameter, never SQL text.
		if len(field) > 200 || strings.ContainsRune(field, 0) {
			return apperror.New(apperror.CodeInvalidInput, "sort field is too long").WithField("sort")
		}
		if filter.Order == "" {
			filter.Order = "asc"
		}
		if filter.Order != "asc" && filter.Order != "desc" {
			return apperror.New(apperror.CodeInvalidInput, "order must be asc or desc").WithField("order")
		}
	} else if err := validate.Sort(&filter.Sort, &filter.Order, []string{"created_at", "updated_at"}, "created_at", "desc"); err != nil {
		return err
	}
	if len(filter.IDs) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold up to 100 ids").WithField("ids")
	}
	for _, id := range filter.IDs {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	if err := checkMatch(filter.Match); err != nil {
		return err
	}
	if err := checkWhere(filter.Where); err != nil {
		return err
	}
	return nil
}

// linksShown is how many pointing records Links returns per collection and field.
const linksShown = 10

func (s *Service) Links(ctx context.Context, id string) ([]domain.RecordLinks, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	of, err := s.Repo.CollectionsOf(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	notFound := apperror.New(apperror.CodeNotFound, "record not found")
	collectionID, ok := of[id]
	if !ok {
		return nil, notFound
	}
	if _, err := policy.CollectionRole(ctx, s.Collections, collectionID, domain.RoleViewer); err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, notFound
		}
		return nil, err
	}
	referrers, err := s.Collections.Referrers(ctx, collectionID, reqctx.Actor(ctx))
	if err != nil {
		return nil, err
	}
	value, _ := json.Marshal(id)
	out := []domain.RecordLinks{}
	for _, ref := range referrers {
		filter := domain.RecordFilter{CollectionID: ref.CollectionID, Page: domain.NewPage(1, linksShown), Sort: "updated_at", Order: "desc"}
		if ref.Sub != "" {
			// An item of the list holding it.
			filter.Match, _ = json.Marshal(map[string]any{ref.Field: []map[string]string{{ref.Sub: id}}})
		} else {
			// One id or a list holding it.
			filter.Where = &domain.Where{Any: true, Rules: []domain.Condition{{Field: ref.Field, Op: domain.OpEq, Value: value}, {Field: ref.Field, Op: domain.OpHas, Value: value}}}
		}
		items, total, err := s.List(ctx, filter)
		if apperror.IsCode(err, apperror.CodeNotFound) {
			continue // an API key sees only its own collection
		}
		if err != nil {
			return nil, err
		}
		if total > 0 {
			out = append(out, domain.RecordLinks{Referrer: ref, Total: total, Records: items})
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Record, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	r, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := policy.CollectionRole(ctx, s.Collections, r.CollectionID, domain.RoleViewer); err != nil {
		if apperror.IsCode(err, apperror.CodeNotFound) {
			return nil, apperror.New(apperror.CodeNotFound, "record not found")
		}
		return nil, err
	}
	return r, nil
}

func (s *Service) Create(ctx context.Context, cmd input.CreateRecordCommand) (*domain.Record, error) {
	if err := validate.ID(cmd.CollectionID); err != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "collection_id is required").WithField("collection_id")
	}
	if len(cmd.Data) == 0 {
		return nil, apperror.New(apperror.CodeInvalidInput, "data is required").WithField("data")
	}
	r := &domain.Record{CollectionID: cmd.CollectionID, Data: cmd.Data}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), cmd.CollectionID, domain.RoleEditor); err != nil {
			return err
		}
		raw, data, err := checkData(ctx, uow, cmd.CollectionID, "", cmd.Data)
		if err != nil {
			return err
		}
		r.Data = data
		if err := uow.Records().Create(ctx, r); err != nil {
			return err
		}
		if err := policy.Audit(ctx, uow, "record.create", r.ID, auditDetails(r.CollectionID, raw, cmd.Data)); err != nil {
			return err
		}
		return policy.Notify(ctx, uow, r.CollectionID, domain.EventRecordCreated, hookData(r))
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) Update(ctx context.Context, cmd input.UpdateRecordCommand) (*domain.Record, error) {
	if err := validate.ID(cmd.ID); err != nil {
		return nil, err
	}
	if len(cmd.Data) == 0 {
		return nil, apperror.New(apperror.CodeInvalidInput, "data is required").WithField("data")
	}
	var updated *domain.Record
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		current, err := uow.Records().FindByID(ctx, cmd.ID)
		if err != nil {
			return err
		}
		if err := editor(ctx, uow, current.CollectionID); err != nil {
			return err
		}
		raw, data, err := checkData(ctx, uow, current.CollectionID, cmd.ID, cmd.Data)
		if err != nil {
			return err
		}
		if updated, err = uow.Records().Update(ctx, cmd.ID, data, cmd.Version); err != nil {
			return err
		}
		if err := policy.Audit(ctx, uow, "record.update", updated.ID, auditDetails(updated.CollectionID, raw, cmd.Data)); err != nil {
			return err
		}
		return policy.Notify(ctx, uow, updated.CollectionID, domain.EventRecordUpdated, hookData(updated))
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteMany(ctx context.Context, ids []string) error {
	if len(ids) == 0 || len(ids) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		owners, err := uow.Records().CollectionsOf(ctx, ids)
		if err != nil {
			return err
		}
		checked := map[string]bool{}
		for _, c := range owners {
			if !checked[c] {
				if err := editor(ctx, uow, c); err != nil {
					return err
				}
				checked[c] = true
			}
		}
		// Deleted records wait in the trash (domain.TrashRetention) before they go for good.
		if err := uow.Tools().TrashRecords(ctx, ids, reqctx.Actor(ctx)); err != nil {
			return err
		}
		for _, id := range ids {
			// collection_id puts the delete in the collection's own log.
			if err := policy.Audit(ctx, uow, "record.delete", id, map[string]any{"collection_id": owners[id]}); err != nil {
				return err
			}
			if err := policy.Notify(ctx, uow, owners[id], domain.EventRecordDeleted, map[string]any{"id": id}); err != nil {
				return err
			}
		}
		return nil
	})
}

// editor checks the caller may write records of the collection; a hidden
// collection reads as a missing record, so ids do not leak.
func editor(ctx context.Context, uow output.UnitOfWork, collectionID string) error {
	_, err := policy.CollectionRole(ctx, uow.Collections(), collectionID, domain.RoleEditor)
	if apperror.IsCode(err, apperror.CodeNotFound) {
		return apperror.New(apperror.CodeNotFound, "record not found")
	}
	return err
}

// auditDetails puts a record write in its collection's log, named by the record's
// title so the entry still reads after a delete.
func auditDetails(collectionID string, raw, data []byte) map[string]any {
	d := map[string]any{"collection_id": collectionID}
	if title := schema.Title(raw, data); title != "" {
		d["name"] = title
	}
	return d
}

// checkData validates data against the collection's schema, then its unique
// fields against the other records (all but recordID, "" when creating), and
// returns the schema it checked. A share form leaves required reference and
// file fields optional: whoever fills it cannot pick or upload them, and
// Quality lists the records still missing them.
func checkData(ctx context.Context, uow output.UnitOfWork, collectionID, recordID string, data []byte) (raw, filled []byte, err error) {
	raw, err = uow.Collections().SchemaForWrite(ctx, collectionID)
	if err != nil {
		return nil, nil, err
	}
	docs := [][]byte{data}
	if err := fill(ctx, uow, raw, collectionID, docs, recordID == ""); err != nil {
		return nil, nil, err
	}
	data = docs[0]
	check := raw
	if k, ok := reqctx.Key(ctx); ok && k.Form {
		check = schema.Optional(raw, append(slices.Collect(maps.Keys(schema.RefKeys(raw))), schema.FileKeys(raw)...))
	}
	compiled, err := schema.Compile(check)
	if err != nil {
		return nil, nil, err
	}
	if err := schema.Validate(compiled, data); err != nil {
		return nil, nil, err
	}
	issues, err := takenIn(ctx, uow, collectionID, recordID, schema.UniqueKeys(raw), [][]byte{data})
	if err != nil {
		return nil, nil, err
	}
	missing, err := linksIn(ctx, uow, raw, collectionID, [][]byte{data})
	if err != nil {
		return nil, nil, err
	}
	issues[0] = append(issues[0], missing[0]...)
	if len(issues[0]) > 0 {
		return nil, nil, apperror.New(apperror.CodeValidationFailed, "a unique field repeats another record's value").
			WithField("data").WithDetails("issues", issues[0])
	}
	return raw, data, nil
}

// hookData is a record as a webhook sends it.
func hookData(r *domain.Record) map[string]any {
	return map[string]any{"id": r.ID, "data": r.Data, "version": r.Version, "updated_at": r.UpdatedAt}
}

// linksIn returns, per document, the reference and file fields whose value
// names a record of the referenced collection, or a file of this one, that
// does not exist.
func linksIn(ctx context.Context, uow output.UnitOfWork, raw []byte, collectionID string, docs [][]byte) ([][]schema.Issue, error) {
	out := make([][]schema.Issue, len(docs))
	refs, files, itemRefs := schema.RefKeys(raw), schema.FileKeys(raw), schema.ItemRefKeys(raw)
	if len(refs) == 0 && len(files) == 0 && len(itemRefs) == 0 {
		return out, nil
	}
	parsed := make([]map[string]json.RawMessage, len(docs))
	for i, doc := range docs {
		_ = json.Unmarshal(doc, &parsed[i]) // not an object: the schema already refused it
	}
	check := func(key, target string, idsOf func(json.RawMessage) []string, exist func(context.Context, string, []string) (map[string]bool, error)) error {
		var ids []string
		for _, p := range parsed {
			ids = append(ids, idsOf(p[key])...)
		}
		if len(ids) == 0 {
			return nil
		}
		found, err := exist(ctx, target, slices.Compact(slices.Sorted(slices.Values(ids))))
		if err != nil {
			return err
		}
		for i, p := range parsed {
			for _, id := range idsOf(p[key]) {
				if !found[id] {
					out[i] = append(out[i], schema.MissingIssue(key))
					break
				}
			}
		}
		return nil
	}
	// A reference holds one record id, or a list of them (several records).
	refIDs := func(v json.RawMessage) []string {
		var one string
		if json.Unmarshal(v, &one) == nil {
			if validate.ID(one) != nil {
				return nil
			}
			return []string{one}
		}
		var many []string
		if json.Unmarshal(v, &many) != nil {
			return nil
		}
		return slices.DeleteFunc(many, func(id string) bool { return validate.ID(id) != nil })
	}
	// A file field holds one {"id", ...} or a list of them (several files).
	fileID := func(v json.RawMessage) []string {
		type ref struct {
			ID string `json:"id"`
		}
		var many []ref
		if json.Unmarshal(v, &many) != nil {
			var one ref
			if json.Unmarshal(v, &one) != nil {
				return nil
			}
			many = []ref{one}
		}
		var ids []string
		for _, f := range many {
			if validate.ID(f.ID) == nil {
				ids = append(ids, f.ID)
			}
		}
		return ids
	}
	for _, key := range slices.Sorted(maps.Keys(refs)) {
		// shortcut: existence only; the caller may not read the referenced collection.
		if err := check(key, refs[key], refIDs, uow.Tools().RecordsExist); err != nil {
			return nil, err
		}
	}
	for _, key := range files {
		if err := check(key, collectionID, fileID, uow.Tools().FilesExist); err != nil {
			return nil, err
		}
	}
	// A reference subfield of a list of objects: the ids its items hold.
	for _, key := range slices.Sorted(maps.Keys(itemRefs)) {
		for _, sub := range slices.Sorted(maps.Keys(itemRefs[key])) {
			itemIDs := func(v json.RawMessage) []string {
				var items []map[string]json.RawMessage
				if json.Unmarshal(v, &items) != nil {
					return nil
				}
				var ids []string
				for _, it := range items {
					ids = append(ids, refIDs(it[sub])...)
				}
				return ids
			}
			if err := check(key, itemRefs[key][sub], itemIDs, uow.Tools().RecordsExist); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// takenIn returns, per document, the unique keys whose value another record
// already holds or an earlier document of the same batch repeats. It takes the
// collection's unique lock first, so the answer holds until commit.
func takenIn(ctx context.Context, uow output.UnitOfWork, collectionID, recordID string, keys []string, docs [][]byte) ([][]schema.Issue, error) {
	out := make([][]schema.Issue, len(docs))
	if len(keys) == 0 {
		return out, nil
	}
	if err := uow.Records().LockUnique(ctx, collectionID); err != nil {
		return nil, err
	}
	for _, key := range keys {
		var values []json.RawMessage
		var owners []int
		seen := map[string]bool{}
		for i, doc := range docs {
			var fields map[string]json.RawMessage
			if json.Unmarshal(doc, &fields) != nil {
				continue
			}
			v, ok := fields[key]
			if !ok || string(v) == "null" {
				continue // missing is not a value: only `required` asks for one
			}
			canon, err := canonical(v)
			if err != nil {
				return nil, err
			}
			if seen[canon] {
				out[i] = append(out[i], schema.UniqueIssue(key))
				continue
			}
			seen[canon] = true
			values = append(values, json.RawMessage(canon))
			owners = append(owners, i)
		}
		taken, err := uow.Records().TakenValues(ctx, collectionID, recordID, key, values)
		if err != nil {
			return nil, err
		}
		for _, n := range taken {
			out[owners[n]] = append(out[owners[n]], schema.UniqueIssue(key))
		}
	}
	return out, nil
}

// canonical rewrites a JSON value so equal values compare equal ("1.0" and "1", key order).
func canonical(v json.RawMessage) (string, error) {
	var x any
	if err := json.Unmarshal(v, &x); err != nil {
		return "", err
	}
	b, err := json.Marshal(x)
	return string(b), err
}
