package domain

import (
	"encoding/json"
	"testing"
)

func TestKeepFields(t *testing.T) {
	data := json.RawMessage(`{"nome":"a","tipo":"x","desc":"longa"}`)
	if got := string(KeepFields(data, []string{"tipo", "nome", "falta"})); got != `{"nome":"a","tipo":"x"}` {
		t.Fatalf("kept: %s", got)
	}
	if got := string(KeepFields(data, nil)); got != string(data) {
		t.Fatalf("no fields: %s", got)
	}
	if got := string(KeepFields(json.RawMessage(`[1]`), []string{"a"})); got != `[1]` {
		t.Fatalf("not an object: %s", got)
	}
}
