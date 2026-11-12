// Package audit reads the log of administrative changes.
package audit

import (
	"context"
	"regexp"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

// actionPrefix keeps LIKE wildcards out of the filter.
var actionPrefix = regexp.MustCompile(`^[a-z_.]{1,60}$`)

type Service struct {
	Repo output.AuditRepository
}

var _ input.AuditService = (*Service)(nil)

func New(repo output.AuditRepository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error) {
	if filter.Action != "" && !actionPrefix.MatchString(filter.Action) {
		return nil, 0, apperror.New(apperror.CodeInvalidInput, "invalid action").WithField("action")
	}
	if filter.UserID != "" {
		if err := validate.ID(filter.UserID); err != nil {
			return nil, 0, err
		}
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return nil, 0, apperror.New(apperror.CodeInvalidInput, "from must be before to").WithField("from")
	}
	return s.Repo.List(ctx, filter)
}
