package schema

import "testing"

func TestPrivateFields(t *testing.T) {
	raw := []byte(`{"type":"object","required":["nome","salario"],"properties":{"nome":{"type":"string"},"salario":{"type":"number","x-private":true}}}`)
	keys := PrivateKeys(raw)
	if len(keys) != 1 || keys[0] != "salario" {
		t.Fatalf("keys %v", keys)
	}
	if got := string(HideSchema(raw, keys)); got != `{"properties":{"nome":{"type":"string"}},"required":["nome"],"type":"object"}` {
		t.Fatalf("schema %s", got)
	}
	if got := string(HideData([]byte(`{"nome":"Ana","salario":10}`), keys)); got != `{"nome":"Ana"}` {
		t.Fatalf("data %s", got)
	}
	if !HasAny([]byte(`{"salario":1}`), keys) || HasAny([]byte(`{"nome":"x"}`), keys) || HasAny([]byte(`{"salario":1}`), nil) {
		t.Fatal("HasAny")
	}
	if got := string(HideData([]byte(`[1]`), keys)); got != `[1]` {
		t.Fatalf("non-object %s", got)
	}
}
