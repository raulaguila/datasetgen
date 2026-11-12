package collection

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// A nil repo and tx prove that bad input never reaches the database.
func TestCreateValidation(t *testing.T) {
	svc := New(nil, nil)
	for name, cmd := range map[string]input.CreateCollectionCommand{
		"empty name":   {Name: " ", Schema: json.RawMessage(`{}`)},
		"bad schema":   {Name: "Wired", Schema: json.RawMessage(`{"type":"nope"}`)},
		"array schema": {Name: "Wired", Schema: json.RawMessage(`[]`)},
		"no schema":    {Name: "Wired"},
	} {
		if _, err := svc.Create(context.Background(), cmd); !apperror.IsCode(err, apperror.CodeInvalidInput) {
			t.Errorf("%s: expected invalid input, got %v", name, err)
		}
	}
}

func TestMalformedIDIsInvalidInput(t *testing.T) {
	svc := New(nil, nil)
	if _, err := svc.Get(context.Background(), "abc"); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("get: %v", err)
	}
	if err := svc.DeleteMany(context.Background(), []string{"abc"}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Update(context.Background(), input.UpdateCollectionCommand{ID: "abc"}); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("update: %v", err)
	}
}
