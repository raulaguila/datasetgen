package product

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestCreateValidation(t *testing.T) {
	_, err := New(nil, nil).Create(context.Background(), input.CreateProductCommand{Name: "", Price: "1"})
	if !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestNormalizePrice(t *testing.T) {
	for raw, want := range map[string]string{"1": "1.00", "19.9": "19.90", " 0.05 ": "0.05", "9999999999.99": "9999999999.99"} {
		if got, err := normalizePrice(raw); err != nil || got != want {
			t.Errorf("%q: got %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "NaN", "Inf", "+Inf", "-1", "0x10", "1e3", "1.999", "12345678901"} {
		if _, err := normalizePrice(raw); !apperror.IsCode(err, apperror.CodeInvalidInput) {
			t.Errorf("%q: expected invalid input, got %v", raw, err)
		}
	}
}

func TestMalformedIDIsInvalidInput(t *testing.T) {
	svc := New(nil, nil) // a nil repo proves the bad id never reaches the database
	if _, err := svc.Get(context.Background(), "abc"); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("get: %v", err)
	}
	if err := svc.DeleteMany(context.Background(), []string{"abc"}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Update(context.Background(), input.UpdateProductCommand{ID: "abc"}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("update: %v", err)
	}
}
