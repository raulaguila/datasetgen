package schema

import (
	"encoding/json"
	"slices"
	"strings"
)

// AutoKeyword marks a top-level property the server fills: "number" (the next
// whole number of the collection), "created_by" (who made the record) or
// "updated_by" (who changed it last). FormulaKeyword holds arithmetic over
// other number fields ("price * qty"), computed on every write.
const (
	AutoKeyword    = "x-auto"
	FormulaKeyword = "x-formula"

	AutoNumber    = "number"
	AutoCreatedBy = "created_by"
	AutoUpdatedBy = "updated_by"
)

// AutoField is a property the server fills.
type AutoField struct {
	Key  string
	Kind string // AutoNumber, AutoCreatedBy or AutoUpdatedBy
}

func topProps(raw []byte) map[string]map[string]any {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	return doc.Properties
}

// AutoKeys lists the filled properties, sorted by key.
func AutoKeys(raw []byte) []AutoField {
	var out []AutoField
	for k, p := range topProps(raw) {
		if kind, _ := p[AutoKeyword].(string); kind == AutoNumber || kind == AutoCreatedBy || kind == AutoUpdatedBy {
			out = append(out, AutoField{Key: k, Kind: kind})
		}
	}
	slices.SortFunc(out, func(a, b AutoField) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// FormulaSpec is a formula property: its expression and the JSON type it
// holds (number, string or boolean; a date is a string).
type FormulaSpec struct {
	Expr string
	Type string
}

// Formulas maps each formula property to its expression and type.
func Formulas(raw []byte) map[string]FormulaSpec {
	out := map[string]FormulaSpec{}
	for k, p := range topProps(raw) {
		if f, _ := p[FormulaKeyword].(string); f != "" {
			t, _ := p["type"].(string)
			out[k] = FormulaSpec{Expr: f, Type: t}
		}
	}
	return out
}
