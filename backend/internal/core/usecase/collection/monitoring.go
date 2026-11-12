package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
)

const (
	scanBatch    = 500
	scanCap      = 500_000 // records one check reads
	invalidList  = 100     // invalid records stored with their issues
	qualityStale = 15 * time.Minute
	maxFieldName = 200
	// JobQuality checks a collection's records in the background.
	JobQuality = "collection.quality"
)

// AuditLog is the collection's slice of the audit log, for its owners.
func (s *Service) AuditLog(ctx context.Context, id string, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleOwner); err != nil {
		return nil, 0, err
	}
	filter.CollectionID = id
	filter.Page = domain.NewPage(filter.Page.Number, filter.Page.Limit)
	return s.Audit.List(ctx, filter)
}

// Quality is the latest check of the records against the schema, since a
// schema change does not revalidate them. Editors see it: they fix records.
func (s *Service) Quality(ctx context.Context, id string) (*input.QualityState, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleEditor); err != nil {
		return nil, err
	}
	return s.qualityState(ctx, id)
}

func (s *Service) qualityState(ctx context.Context, id string) (*input.QualityState, error) {
	c, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	run, err := s.Repo.Quality(ctx, id, false)
	if err != nil {
		return nil, err
	}
	return &input.QualityState{Run: run, CurrentVersion: c.Version}, nil
}

// StartQuality queues a check unless one started less than 15 minutes ago is still running.
func (s *Service) StartQuality(ctx context.Context, id string) (*input.QualityState, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleEditor); err != nil {
		return nil, err
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		c, err := uow.Collections().FindByID(ctx, id)
		if err != nil {
			return err
		}
		started, err := uow.Collections().StartQuality(ctx, id, c.Version, qualityStale)
		if err != nil || !started {
			return err
		}
		return policy.EnqueueJob(ctx, uow, JobQuality, qualityJob{ID: id})
	})
	if err != nil {
		return nil, err
	}
	return s.qualityState(ctx, id)
}

type qualityJob struct {
	ID string `json:"id"`
}

// Jobs returns the collection module's job handlers, registered with the job worker.
func Jobs(repo output.CollectionRepository, records output.RecordRepository) map[string]policy.JobHandler {
	return map[string]policy.JobHandler{
		JobQuality: func(ctx context.Context, payload []byte) error {
			var job qualityJob
			if err := json.Unmarshal(payload, &job); err != nil {
				return policy.PermanentJobError(err)
			}
			run := checkRecords(ctx, repo, records, job.ID)
			if err := repo.FinishQuality(ctx, job.ID, run); err != nil {
				return err // retried: the check runs again
			}
			return nil
		},
	}
}

// checkRecords walks the records by id (idx_app_record_collection_key) in
// batches; a failure is stored in the run rather than retried.
func checkRecords(ctx context.Context, repo output.CollectionRepository, records output.RecordRepository, id string) domain.QualityRun {
	run := domain.QualityRun{Status: domain.QualityDone}
	fail := func(err error) domain.QualityRun {
		return domain.QualityRun{Status: domain.QualityFailed, Error: err.Error()}
	}
	c, err := repo.FindByID(ctx, id)
	if err != nil {
		return fail(err)
	}
	compiled, err := schema.Compile(c.Schema)
	if err != nil {
		return fail(err)
	}
	invalid := []input.InvalidRecord{}
	after := ""
	for {
		if run.Checked >= scanCap {
			run.Truncated = true
			break
		}
		batch, err := records.Scan(ctx, id, after, min(scanBatch, scanCap-run.Checked))
		if err != nil {
			return fail(err)
		}
		for _, r := range batch {
			issues, err := schema.Check(compiled, r.Data)
			if err != nil {
				return fail(err)
			}
			if len(issues) > 0 {
				run.InvalidCount++
				run.InvalidIDs = append(run.InvalidIDs, r.ID)
				if len(invalid) < invalidList {
					invalid = append(invalid, input.InvalidRecord{ID: r.ID, Data: r.Data, UpdatedAt: r.UpdatedAt, Issues: issues})
				}
			}
		}
		run.Checked += len(batch)
		if len(batch) < scanBatch {
			break
		}
		after = batch[len(batch)-1].ID
	}
	if run.Invalid, err = json.Marshal(invalid); err != nil {
		return fail(err)
	}
	return run
}

// Fix applies one bulk change to the records' data without checking them
// against the schema: it exists to bring records back into format.
func (s *Service) Fix(ctx context.Context, id string, fix domain.RecordFix) (int64, error) {
	if err := checkFix(&fix); err != nil {
		return 0, err
	}
	var changed int64
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), id, domain.RoleEditor); err != nil {
			return err
		}
		var err error
		if changed, err = uow.Records().Fix(ctx, id, fix); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.fix", id, map[string]any{"collection_id": id, "op": fix.Op, "field": fix.Field, "to": fix.To, "count": changed})
	})
	return changed, err
}

func checkFix(fix *domain.RecordFix) error {
	field := func(name, key string) error {
		if name == "" || utf8.RuneCountInString(name) > maxFieldName || strings.ContainsRune(name, 0) {
			return apperror.New(apperror.CodeInvalidInput, "a field name is required").WithField(key)
		}
		return nil
	}
	if err := field(fix.Field, "field"); err != nil {
		return err
	}
	switch fix.Op {
	case domain.FixSetDefault:
		if !json.Valid(fix.Value) || len(fix.Value) == 0 {
			return apperror.New(apperror.CodeInvalidInput, "value must be JSON").WithField("value")
		}
	case domain.FixRename:
		if err := field(fix.To, "to"); err != nil {
			return err
		}
		if fix.To == fix.Field {
			return apperror.New(apperror.CodeInvalidInput, "the new name must differ").WithField("to")
		}
	case domain.FixRemove:
	default:
		return apperror.New(apperror.CodeInvalidInput, "op must be set_default, rename or remove").WithField("op")
	}
	return nil
}

// DeleteInvalid deletes the records the latest finished check of the current
// schema found invalid, except those changed after it started (maybe fixed).
func (s *Service) DeleteInvalid(ctx context.Context, id string) (int, error) {
	var deleted []string
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if _, err := policy.CollectionRole(ctx, uow.Collections(), id, domain.RoleEditor); err != nil {
			return err
		}
		c, err := uow.Collections().FindByID(ctx, id)
		if err != nil {
			return err
		}
		run, err := uow.Collections().Quality(ctx, id, true)
		if err != nil {
			return err
		}
		if run == nil || run.Status != domain.QualityDone || run.SchemaVersion != c.Version {
			return apperror.New(apperror.CodeConflict, "run the quality check again first: the last one is missing, unfinished or of older fields")
		}
		if len(run.InvalidIDs) == 0 {
			return nil
		}
		if deleted, err = uow.Records().DeleteListed(ctx, id, run.InvalidIDs, run.StartedAt); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.delete_invalid", id, map[string]any{"collection_id": id, "count": len(deleted)})
	})
	return len(deleted), err
}

// Duplicate copies name, description, icon and fields, and the records when
// asked, into a new collection the caller owns. It needs read access to the
// source; the handler checks the right to create collections.
func (s *Service) Duplicate(ctx context.Context, cmd input.DuplicateCollectionCommand) (*domain.Collection, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, cmd.ID, domain.RoleViewer); err != nil {
		return nil, err
	}
	src, err := s.Repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	name := cmd.Name
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s (copy)", src.Name)
	}
	var copied int64
	c, err := s.create(ctx, input.CreateCollectionCommand{Name: name, Description: src.Description, Icon: src.Icon, Schema: src.Schema},
		func(ctx context.Context, uow output.UnitOfWork, c *domain.Collection) (map[string]any, error) {
			if cmd.Records {
				var err error
				if copied, err = uow.Records().CopyAll(ctx, src.ID, c.ID); err != nil {
					return nil, err
				}
			}
			return map[string]any{"source_id": src.ID, "records": copied}, nil
		})
	if err != nil {
		return nil, err
	}
	c.RecordCount = copied
	return c, nil
}

const statsDays = 30

func (s *Service) Stats(ctx context.Context, id, tz string) (*domain.CollectionStats, error) {
	if tz == "" {
		tz = "UTC"
	}
	// Postgres reads the same IANA names; anything else would be a query error.
	if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
		return nil, apperror.New(apperror.CodeInvalidInput, "tz must be an IANA time zone").WithField("tz")
	}
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleViewer); err != nil {
		return nil, err
	}
	return s.Repo.Stats(ctx, id, tz, statsDays)
}
