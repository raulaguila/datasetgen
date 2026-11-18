package schema

import (
	"encoding/json"
	"maps"
	"slices"
	"time"
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
	return slices.Clone(Metadata(raw).Autos)
}

// FormulaSpec is a formula property: its expression and the JSON type it
// holds (number, string or boolean; a date is a string with format "date").
type FormulaSpec struct {
	Expr   string
	Type   string
	Format string
}

// Formulas maps each formula property to its expression and type.
func Formulas(raw []byte) map[string]FormulaSpec {
	return maps.Clone(Metadata(raw).Formulas)
}

// Holds reports whether a formula's value fits the field: its JSON type and,
// for a date formula, a real date. A value that does not fit is left out
// rather than stored, so the record still saves.
func (f FormulaSpec) Holds(v any) bool {
	switch v := v.(type) {
	case float64:
		return f.Type == "number"
	case bool:
		return f.Type == "boolean"
	case string:
		if f.Type != "string" {
			return false
		}
		if f.Format == "date" {
			_, err := time.Parse(time.DateOnly, v)
			return err == nil
		}
		return true
	}
	return false
}

// ComputedChanged reports whether a schema change gives stored records server
// filled values to compute: a new auto number, or a formula that is new or
// changed. Records written before it are filled in the background.
func ComputedChanged(before, after []byte) bool {
	numbers := func(raw []byte) map[string]bool {
		out := map[string]bool{}
		for _, a := range AutoKeys(raw) {
			if a.Kind == AutoNumber {
				out[a.Key] = true
			}
		}
		return out
	}
	old := numbers(before)
	for k := range numbers(after) {
		if !old[k] {
			return true
		}
	}
	was := Formulas(before)
	for k, f := range Formulas(after) {
		if w, ok := was[k]; !ok || w != f {
			return true
		}
	}
	return false
}
