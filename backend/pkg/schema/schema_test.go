package schema

import (
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
	if issues := err.(*apperror.Error).Details["issues"].([]Issue); len(issues) < 3 {
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
