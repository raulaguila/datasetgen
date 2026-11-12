package record

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// A nil repo and tx prove that bad input never reaches the database.
func TestInputValidation(t *testing.T) {
	svc := New(nil, nil)
	ctx := context.Background()
	if _, _, err := svc.List(ctx, domain.RecordFilter{}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Errorf("list without collection: %v", err)
	}
	if _, err := svc.Create(ctx, input.CreateRecordCommand{CollectionID: "abc", Data: []byte(`{}`)}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Errorf("create with bad collection: %v", err)
	}
	if _, err := svc.Create(ctx, input.CreateRecordCommand{CollectionID: "0193f1f0-0000-7000-8000-000000000000"}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Errorf("create without data: %v", err)
	}
	if _, err := svc.Get(ctx, "abc"); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Errorf("get: %v", err)
	}
	if err := svc.DeleteMany(ctx, nil); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Errorf("delete none: %v", err)
	}
}
