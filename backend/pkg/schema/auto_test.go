package schema

import (
	"encoding/json"
	"testing"
)

func TestFormulas(t *testing.T) {
	vals := map[string]json.RawMessage{"preco": json.RawMessage(`2.5`), "qtd": json.RawMessage(`4`), "nome": json.RawMessage(`"x"`), "zero": json.RawMessage(`0`)}
	for src, want := range map[string]float64{
		"preco * qtd":         10,
		"preco*qtd + 1":       11,
		"(preco + 0.5) * qtd": 12,
		"-qtd + 10 / 2":       1,
		"qtd - 1 - 1":         2,
	} {
		f, err := ParseFormula(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got, ok := f.Eval(vals); !ok || got != want {
			t.Errorf("%s = %v (%v), want %v", src, got, ok, want)
		}
	}
	for _, src := range []string{"nome * 2", "falta + 1", "qtd / zero"} {
		f, _ := ParseFormula(src)
		if _, ok := f.Eval(vals); ok {
			t.Errorf("%s: want no value", src)
		}
	}
	for _, src := range []string{"", "1 +", "(1", "a $ b", "1 2", "2..3"} {
		if _, err := ParseFormula(src); err == nil {
			t.Errorf("%q: want a parse error", src)
		}
	}
	f, _ := ParseFormula("a * (b + c)")
	if got := f.Fields(); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("fields: %v", got)
	}
}

func TestAutoKeys(t *testing.T) {
	raw := []byte(`{"properties":{"n":{"type":"integer","x-auto":"number"},"by":{"type":"string","x-auto":"created_by"},"x":{"x-auto":"nope"},"t":{"type":"number","x-formula":"n * 2"}}}`)
	if got := AutoKeys(raw); len(got) != 2 || got[0] != (AutoField{"by", AutoCreatedBy}) || got[1] != (AutoField{"n", AutoNumber}) {
		t.Fatalf("auto: %+v", got)
	}
	if got := Formulas(raw); got["t"] != "n * 2" || len(got) != 1 {
		t.Fatalf("formulas: %v", got)
	}
}
