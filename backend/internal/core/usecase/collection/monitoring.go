package collection

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
)

const (
	scanBatch   = 500
	scanCap     = 50_000 // records checked in one request
	invalidList = 100    // invalid records returned with their issues
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

// Quality checks every stored record against the current schema, since a
// schema change does not revalidate them. Editors see it: they fix records.
// shortcut: runs in the request and stops at 50 000 records (Truncated); move it
// to a job with stored results when collections outgrow that.
func (s *Service) Quality(ctx context.Context, id string) (*input.QualityReport, error) {
	if _, err := policy.CollectionRole(ctx, s.Repo, id, domain.RoleEditor); err != nil {
		return nil, err
	}
	c, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	compiled, err := schema.Compile(c.Schema)
	if err != nil {
		return nil, err
	}
	report := &input.QualityReport{Version: c.Version, Invalid: []input.InvalidRecord{}}
	after := ""
	for report.Checked < scanCap {
		batch, err := s.Records.Scan(ctx, id, after, min(scanBatch, scanCap-report.Checked))
		if err != nil {
			return nil, err
		}
		for _, r := range batch {
			issues, err := schema.Check(compiled, r.Data)
			if err != nil {
				return nil, err
			}
			if len(issues) > 0 {
				report.InvalidCount++
				if len(report.Invalid) < invalidList {
					report.Invalid = append(report.Invalid, input.InvalidRecord{ID: r.ID, Data: r.Data, UpdatedAt: r.UpdatedAt, Issues: issues})
				}
			}
		}
		report.Checked += len(batch)
		if len(batch) < scanBatch {
			return report, nil
		}
		after = batch[len(batch)-1].ID
	}
	report.Truncated = true
	return report, nil
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
