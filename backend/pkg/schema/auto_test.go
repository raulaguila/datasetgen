package schema

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFormulas(t *testing.T) {
	vals := map[string]json.RawMessage{"preco": json.RawMessage(`2.5`), "qtd": json.RawMessage(`4`), "nome": json.RawMessage(`"Ana Lima"`),
		"zero": json.RawMessage(`0`), "pago": json.RawMessage(`true`), "venc": json.RawMessage(`"2026-11-20"`), "em": json.RawMessage(`"2026-11-10T15:00:00Z"`)}
	today := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	for src, want := range map[string]any{
		"preco * qtd":                         10.0,
		"preco*qtd + 1":                       11.0,
		"(preco + 0.5) * qtd":                 12.0,
		"-qtd + 10 / 2":                       1.0,
		"qtd - 1 - 1":                         2.0,
		`IF(qtd > 3, "muito", "pouco")`:       "muito",
		`IF(falta > 3, "muito", "pouco")`:     "pouco",
		"ROUND(10 / 3, 2)":                    3.33,
		"ROUND(2.5)":                          3.0,
		`nome & " (" & qtd & ")"`:             "Ana Lima (4)",
		`CONCAT(UPPER(nome), "-", LEN(nome))`: "ANA LIMA-8",
		"AND(pago, qtd = 4, NOT(zero))":       true,
		"OR(zero, falta)":                     false,
		`COALESCE(falta, "", nome)`:           "Ana Lima",
		"MIN(qtd, preco, 9)":                  2.5,
		"DAYS(venc, TODAY())":                 4.0,
		"DAYS(venc, em)":                      10.0,
		"DATEADD(venc, 15)":                   "2026-12-05",
		"YEAR(venc) * 100 + MONTH(venc)":      202611.0,
		`nome <> "Ana"`:                       true,
		"qtd != 4":                            false,
		"preco >= 2.5":                        true,
	} {
		f, err := ParseFormula(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got, ok := f.Eval(vals, today); !ok || got != want {
			t.Errorf("%s = %v (%v), want %v", src, got, ok, want)
		}
	}
	for _, src := range []string{"nome * 2", "falta + 1", "qtd / zero", "UPPER(falta)", "DAYS(nome, venc)", "COALESCE(falta)"} {
		f, err := ParseFormula(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if v, ok := f.Eval(vals, today); ok {
			t.Errorf("%s = %v: want no value", src, v)
		}
	}
	for _, src := range []string{"", "1 +", "(1", "a $ b", "1 2", "2..3", "NADA(1)", "IF(1, 2)", `"aberto`, "ROUND(1,", "TODAY(1)"} {
		if _, err := ParseFormula(src); err == nil {
			t.Errorf("%q: want a parse error", src)
		}
	}
	f, _ := ParseFormula("a * (b + c) + IF(d, e, 1)")
	if got := f.Fields(); len(got) != 5 || got[0] != "a" || got[4] != "e" {
		t.Errorf("fields: %v", got)
	}
}

func TestAutoKeys(t *testing.T) {
	raw := []byte(`{"properties":{"n":{"type":"integer","x-auto":"number"},"by":{"type":"string","x-auto":"created_by"},"x":{"x-auto":"nope"},"t":{"type":"number","x-formula":"n * 2"}}}`)
	if got := AutoKeys(raw); len(got) != 2 || got[0] != (AutoField{"by", AutoCreatedBy}) || got[1] != (AutoField{"n", AutoNumber}) {
		t.Fatalf("auto: %+v", got)
	}
	if got := Formulas(raw); got["t"] != (FormulaSpec{"n * 2", "number"}) || len(got) != 1 {
		t.Fatalf("formulas: %v", got)
	}
}
