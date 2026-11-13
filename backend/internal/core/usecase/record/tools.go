package record

import (
	"context"
	"encoding/json"

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
	maxExport     = 100_000
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
		raw, err := checkData(ctx, uow, current.CollectionID, cmd.ID, data)
		if err != nil {
			return err
		}
		if restored, err = uow.Records().Update(ctx, cmd.ID, data, cmd.CurrentVersion); err != nil {
			return err
		}
		details := auditDetails(current.CollectionID, raw, data)
		details["version"] = cmd.Version
		return policy.Audit(ctx, uow, "record.restore", cmd.ID, details)
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
		taken, err := takenIn(ctx, uow, cmd.CollectionID, "", schema.UniqueKeys(raw), docs)
		if err != nil {
			return err
		}
		for n, issues := range taken {
			if len(issues) > 0 {
				bad[passed[n]] = issues
			}
		}
		valid := make([]string, 0, len(cmd.Records))
		for i, data := range cmd.Records {
			if issues, ok := bad[i]; ok {
				res.Invalid++
				if len(res.Errors) < maxImportErrs {
					res.Errors = append(res.Errors, input.ImportError{Index: i, Issues: issues})
				}
				continue
			}
			valid = append(valid, string(data))
		}
		if cmd.DryRun || len(valid) == 0 || (res.Invalid > 0 && !cmd.SkipInvalid) {
			return nil
		}
		if res.Created, err = uow.Records().CreateMany(ctx, cmd.CollectionID, valid); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "record.import", cmd.CollectionID, map[string]any{"collection_id": cmd.CollectionID, "count": res.Created})
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Export returns the collection's records in id order, with the schema for the
// column order; ValidOnly leaves out records that break it. Viewers.
// shortcut: built in memory up to 100 000 records; stream it when exports grow past that.
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
	out := &input.Export{Name: c.Name, Schema: c.Schema}
	after, scanned := "", 0
	for {
		batch, err := s.Repo.Scan(ctx, collectionID, after, exportBatch)
		if err != nil {
			return nil, err
		}
		for _, r := range batch {
			if scanned++; scanned > maxExport {
				return nil, apperror.New(apperror.CodeInvalidInput, "the collection has more than 100 000 records to export")
			}
			if validOnly {
				issues, err := schema.Check(compiled, r.Data)
				if err != nil {
					return nil, err
				}
				if len(issues) > 0 {
					continue
				}
			}
			out.Records = append(out.Records, r.Data)
		}
		if len(batch) < exportBatch {
			return out, nil
		}
		after = batch[len(batch)-1].ID
	}
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
		default:
			return bad
		}
	}
	return nil
}
