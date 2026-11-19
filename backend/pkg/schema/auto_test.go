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
}

func TestAutoKeys(t *testing.T) {
	raw := []byte(`{"properties":{"n":{"type":"integer","x-auto":"number"},"by":{"type":"string","x-auto":"created_by"},"x":{"x-auto":"nope"},"t":{"type":"number","x-formula":"n * 2"}}}`)
	if got := AutoKeys(raw); len(got) != 2 || got[0] != (AutoField{"by", AutoCreatedBy}) || got[1] != (AutoField{"n", AutoNumber}) {
		t.Fatalf("auto: %+v", got)
	}
	if got := Formulas(raw); got["t"] != (FormulaSpec{Expr: "n * 2", Type: "number"}) || len(got) != 1 {
		t.Fatalf("formulas: %v", got)
	}
}

func TestFormulaHolds(t *testing.T) {
	date := FormulaSpec{Type: "string", Format: "date"}
	cases := []struct {
		f    FormulaSpec
		v    any
		want bool
	}{
		{date, "2026-01-02", true},
		{date, "texto", false},
		{date, 3.0, false},
		{FormulaSpec{Type: "string"}, "texto", true},
		{FormulaSpec{Type: "number"}, 3.0, true},
		{FormulaSpec{Type: "number"}, "3", false},
		{FormulaSpec{Type: "boolean"}, true, true},
		{FormulaSpec{Type: "boolean"}, nil, false},
	}
	for _, c := range cases {
		if got := c.f.Holds(c.v); got != c.want {
			t.Errorf("%+v holds %v: got %v, want %v", c.f, c.v, got, c.want)
		}
	}
}

func TestComputedChanged(t *testing.T) {
	plain := []byte(`{"properties":{"a":{"type":"number"}}}`)
	num := []byte(`{"properties":{"a":{"type":"number"},"n":{"type":"integer","x-auto":"number"}}}`)
	f1 := []byte(`{"properties":{"a":{"type":"number"},"t":{"type":"number","x-formula":"a * 2"}}}`)
	f2 := []byte(`{"properties":{"a":{"type":"number"},"t":{"type":"number","x-formula":"a * 3"}}}`)
	by := []byte(`{"properties":{"a":{"type":"number"},"b":{"type":"string","x-auto":"created_by"}}}`)
	cases := []struct {
		name          string
		before, after []byte
		want          bool
	}{
		{"new number", plain, num, true},
		{"same number", num, num, false},
		{"new formula", plain, f1, true},
		{"changed formula", f1, f2, true},
		{"same formula", f1, f1, false},
		{"removed formula", f1, plain, false},
		{"who made it", plain, by, false},
	}
	for _, c := range cases {
		if got := ComputedChanged(c.before, c.after); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
