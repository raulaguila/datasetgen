package schema

import (
	"encoding/json"
	"testing"
)

func TestFields(t *testing.T) {
	raw := []byte(`{"type":"object","required":["nome"],"properties":{
		"nome":{"type":"string","title":"Nome","x-unique":true,"description":"Quem paga"},
		"obs":{"type":"string","maxLength":5000},
		"conteudo":{"type":"string","x-multiline":true},
		"venc":{"type":"string","format":"date","title":"Vencimento"},
		"status":{"enum":["aberto","pago"]},
		"tags":{"type":"array","items":{"type":"string"}},
		"cliente":{"type":"string","format":"uuid","x-ref":"c1"},
		"itens":{"type":"array","items":{"type":"object","required":["qtd"],"properties":{"qtd":{"type":"number"},"tam":{"enum":["P","G"]}}}},
		"fone":{"x-lookup":{"ref":"cliente","field":"fone"}},
		"pagos":{"type":"number","x-rollup":{"collection":"c2","ref":"conta","fn":"count"}},
		"total":{"type":"number","x-formula":"qtd * 2"},
		"n":{"type":"integer","x-auto":"number"},
		"anexo":{"type":"object","x-file":true},
		"extra":{"type":"object"}}}`)
	got := Fields(raw)
	want := []Field{
		{Key: "nome", Name: "Nome", Type: "text", Description: "Quem paga", Required: true, Unique: true},
		{Key: "obs", Type: "long_text"},
		{Key: "conteudo", Type: "long_text"},
		{Key: "venc", Name: "Vencimento", Type: "date"},
		{Key: "status", Type: "option", Options: []string{"aberto", "pago"}},
		{Key: "tags", Type: "text_list"},
		{Key: "cliente", Type: "reference", Target: "c1"},
		{Key: "itens", Type: "list", Subfields: []Field{{Key: "qtd", Type: "number", Required: true}, {Key: "tam", Type: "option", Options: []string{"P", "G"}}}},
		{Key: "fone", Type: "lookup", Lookup: &Lookup{Ref: "cliente", Field: "fone"}},
		{Key: "pagos", Type: "rollup", Rollup: &Rollup{Collection: "c2", Ref: "conta", Fn: "count"}},
		{Key: "total", Type: "formula", Formula: "qtd * 2", Result: "number"},
		{Key: "n", Type: "auto_number"},
		{Key: "anexo", Type: "file"},
		{Key: "extra", Type: "json"},
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("got  %s\nwant %s", a, b)
	}
	if Fields([]byte(`not json`)) == nil || len(Fields([]byte(`{}`))) != 0 {
		t.Fatal("bad schema: want an empty list")
	}
}
