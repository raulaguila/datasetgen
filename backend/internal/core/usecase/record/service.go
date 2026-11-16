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
	items, total, err := s.Repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return items, total, s.withVirtual(ctx, filter.CollectionID, items)
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
	one := []domain.Record{*r}
	if err := s.withVirtual(ctx, r.CollectionID, one); err != nil {
		return nil, err
	}
	return &one[0], nil
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
	return s.deleteMany(ctx, "", ids)
}

// DeleteManyIn is DeleteMany for one collection: every record must be in it, or none goes.
func (s *Service) DeleteManyIn(ctx context.Context, collectionID string, ids []string) error {
	if err := validate.ID(collectionID); err != nil {
		return apperror.New(apperror.CodeInvalidInput, "invalid collection").WithField("collection_id")
	}
	return s.deleteMany(ctx, collectionID, ids)
}

// deleteMany trashes ids; a non-empty only refuses records of any other collection.
func (s *Service) deleteMany(ctx context.Context, only string, ids []string) error {
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
		if only != "" {
			for _, id := range ids {
				if owners[id] != only {
					return apperror.New(apperror.CodeNotFound, "record "+id+" is not in this collection").WithField("record_ids")
				}
			}
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

// linksIn returns, per document, the reference and file fields naming a record or file that does not exist.
func linksIn(ctx context.Context, uow output.UnitOfWork, raw []byte, collectionID string, docs [][]byte) ([][]schema.Issue, error) {
	return policy.MissingLinks(ctx, uow.Tools(), raw, collectionID, docs)
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
