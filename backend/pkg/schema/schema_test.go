package schema

import (
	"errors"
	"reflect"
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

// The field builder writes these rules; each must hold on the server too.
func TestBuilderRulesHold(t *testing.T) {
	s, err := Compile([]byte(`{"type":"object","properties":{
		"quando":{"title":"Quando","type":"string","format":"date-time","examples":["agora"]},
		"email":{"type":"string","format":"email"},
		"site":{"type":"string","format":"uri"},
		"nome":{"type":"string","minLength":2,"maxLength":5,"default":"abc","description":"Ajuda"},
		"nota":{"type":"integer","minimum":0,"maximum":10,"default":5},
		"tags":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s, []byte(`{"quando":"2026-11-12T10:00:00Z","email":"a@b.io","site":"https://x.io","nome":"abc","nota":10,"tags":["a"]}`)); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	for _, bad := range []string{
		`{"quando":"2026-11-12 10:00"}`, `{"email":"a@"}`, `{"site":"x.io"}`,
		`{"nome":"a"}`, `{"nome":"abcdef"}`, `{"nota":11}`, `{"nota":-1}`, `{"tags":[]}`, `{"tags":["a","b","c"]}`,
	} {
		if err := Validate(s, []byte(bad)); !apperror.IsCode(err, apperror.CodeValidationFailed) {
			t.Errorf("%s accepted: %v", bad, err)
		}
	}
}

func TestTitleFollowsFieldOrder(t *testing.T) {
	raw := []byte(`{"properties":{"n":{"type":"integer"},"b":{"type":"string"},"a":{"type":"string"}}}`)
	if got := Title(raw, []byte(`{"n":1,"a":"A","b":"  "}`)); got != "A" {
		t.Fatalf("Title = %q, want A (blank b skipped, order kept)", got)
	}
	if got := Title(raw, []byte(`{"n":1}`)); got != "" {
		t.Fatalf("Title = %q, want empty", got)
	}
}

func TestFieldChanges(t *testing.T) {
	before := []byte(`{"properties":{"a":{"type":"string","title":"A"},"b":{"type":"string"},"c":{"type":"string"}}}`)
	after := []byte(`{"properties":{"a":{"title":"A","type":"string"},"b":{"type":"integer","title":"Bê"},"d":{"type":"string"}}}`)
	got := FieldChanges(before, after)
	want := map[string][]string{"changed": {"Bê"}, "added": {"d"}, "removed": {"c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FieldChanges = %v, want %v", got, want)
	}
	if FieldChanges(before, before) != nil {
		t.Fatal("no change must be nil")
	}
}
