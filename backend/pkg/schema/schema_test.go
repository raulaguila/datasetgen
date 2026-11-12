package schema

import (
	"errors"
	"testing"

	"github.com/raulaguila/dataset/pkg/apperror"
)

const wired = `{
  "type": "object",
  "required": ["nome", "categoria"],
  "properties": {
    "nome": {"type": "string", "minLength": 1},
    "categoria": {"enum": ["gatilho", "efeito", "condicao"]},
    "parametros": {"type": "array", "items": {"type": "object", "required": ["nome"]}}
  }
}`

func TestValidate(t *testing.T) {
	s, err := Compile([]byte(wired))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s, []byte(`{"nome":"Ao clicar","categoria":"gatilho","parametros":[{"nome":"x"}]}`)); err != nil {
		t.Fatalf("valid: %v", err)
	}
	err = Validate(s, []byte(`{"nome":"","categoria":"outro","parametros":[{}]}`))
	if !apperror.IsCode(err, apperror.CodeValidationFailed) {
		t.Fatalf("invalid: %v", err)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.Error, got %T", err)
	}
	if issues := appErr.Details["issues"].([]Issue); len(issues) < 3 {
		t.Fatalf("want one issue per broken rule, got %+v", issues)
	}
}

func TestCompileRefusesBadSchemas(t *testing.T) {
	for _, raw := range []string{
		`[]`, `not json`, `{"type": "nope"}`,
		`{"$ref": "file:///etc/passwd"}`,
		`{"$ref": "https://example.com/s.json"}`,
	} {
		if _, err := Compile([]byte(raw)); !apperror.IsCode(err, apperror.CodeInvalidInput) {
			t.Errorf("%s: expected invalid input, got %v", raw, err)
		}
	}
}

func TestValidateAssertsFormat(t *testing.T) {
	s, err := Compile([]byte(`{"type":"object","properties":{"dia":{"type":"string","format":"date"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s, []byte(`{"dia":"2026-11-12"}`)); err != nil {
		t.Fatalf("valid date: %v", err)
	}
	if err := Validate(s, []byte(`{"dia":"ontem"}`)); !apperror.IsCode(err, apperror.CodeValidationFailed) {
		t.Fatalf("bad date accepted: %v", err)
	}
}

func TestRequiredPointsAtTheMissingField(t *testing.T) {
	s, err := Compile([]byte(`{"type":"object","properties":{"ps":{"type":"array","items":{"required":["a/b","tipo"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var appErr *apperror.Error
	if !errors.As(Validate(s, []byte(`{"ps":[{}]}`)), &appErr) {
		t.Fatal("want a validation error")
	}
	got := appErr.Details["issues"].([]Issue)
	want := []Issue{{Path: "/ps/0/a~1b", Keyword: "required", Message: "missing property"}, {Path: "/ps/0/tipo", Keyword: "required", Message: "missing property"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("issues %+v", got)
	}
}
