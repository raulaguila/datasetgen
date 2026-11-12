package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/internal/core/usecase/audit"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type repo struct {
	output.AuditRepository
	called bool
}

func (r *repo) List(context.Context, domain.AuditFilter) ([]domain.AuditRecord, int64, error) {
	r.called = true
	return nil, 0, nil
}

func TestListRefusesFiltersThatReachSQLUnchecked(t *testing.T) {
	now := time.Now()
	for name, f := range map[string]domain.AuditFilter{
		"like wildcard": {Action: "role%"},
		"bad user id":   {UserID: "x"},
		"empty range":   {From: &now, To: &now},
	} {
		r := &repo{}
		_, _, err := audit.New(r).List(context.Background(), f)
		if !apperror.IsCode(err, apperror.CodeInvalidInput) || r.called {
			t.Errorf("%s: err %v, reached repo %v", name, err, r.called)
		}
	}
	r := &repo{}
	if _, _, err := audit.New(r).List(context.Background(), domain.AuditFilter{Action: "user."}); err != nil || !r.called {
		t.Errorf("valid filter: %v", err)
	}
}
