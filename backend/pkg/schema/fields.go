package schema

import (
	"encoding/json"
	"slices"
)

// Field is a top-level field as an assistant reads it: its kind in the app's
// words and what it holds, without the JSON Schema around it.
type Field struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"` // the title the app shows, when it differs from the key
	// text, long_text, email, url, number, integer, boolean, date, datetime, option,
	// text_list, reference, references, file, files, lookup, rollup, formula,
	// auto_number, created_by, updated_by, group, list, json; "other" for anything else.
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Unique      bool     `json:"unique,omitempty"`
	Options     []string `json:"options,omitempty"`   // an option field's choices
	Target      string   `json:"target,omitempty"`    // a reference's collection id
	Formula     string   `json:"formula,omitempty"`   // a formula's expression
	Result      string   `json:"result,omitempty"`    // what a formula gives: number, text, date or boolean
	Lookup      *Lookup  `json:"lookup,omitempty"`    // the reference and the field it shows
	Rollup      *Rollup  `json:"rollup,omitempty"`    // the collection, reference, function and field it sums up
	Subfields   []string `json:"subfields,omitempty"` // a group's or a list's subfield keys
}

// Fields lists the top-level fields in the schema's order.
func Fields(raw []byte) []Field {
	var doc struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &doc)
	props := topProps(raw)
	out := make([]Field, 0, len(props))
	for _, key := range PropertyOrder(raw) {
		p, ok := props[key]
		if !ok {
			continue
		}
		f := Field{Key: key, Required: slices.Contains(doc.Required, key)}
		if title, _ := p["title"].(string); title != "" && title != key {
			f.Name = title
		}
		f.Description, _ = p["description"].(string)
		f.Unique, _ = p[UniqueKeyword].(bool)
		describe(&f, p)
		out = append(out, f)
	}
	return out
}

// describe fills the field's type and what goes with it, in the order the app reads a property.
func describe(f *Field, p map[string]any) {
	typ, _ := p["type"].(string)
	str := func(k string) string { s, _ := p[k].(string); return s }
	if list, ok := p["enum"].([]any); ok {
		f.Type = "option"
		for _, v := range list {
			if s, ok := v.(string); ok {
				f.Options = append(f.Options, s)
			}
		}
		return
	}
	var l Lookup
	if decode(p[LookupKeyword], &l) {
		f.Type, f.Lookup = "lookup", &l
		return
	}
	var r Rollup
	if decode(p[RollupKeyword], &r) {
		f.Type, f.Rollup = "rollup", &r
		return
	}
	if expr := str(FormulaKeyword); expr != "" {
		f.Type, f.Formula = "formula", expr
		f.Result = map[string]string{"number": "number", "boolean": "boolean", "string": "text"}[typ]
		if typ == "string" && str("format") == "date" {
			f.Result = "date"
		}
		return
	}
	switch str(AutoKeyword) {
	case "number":
		f.Type = "auto_number"
		return
	case "created_by", "updated_by":
		f.Type = str(AutoKeyword)
		return
	}
	if target := str(RefKeyword); target != "" {
		f.Type, f.Target = "reference", target
		if typ == "array" {
			f.Type = "references"
		}
		return
	}
	if file, _ := p[FileKeyword].(bool); file {
		f.Type = "file"
		if typ == "array" {
			f.Type = "files"
		}
		return
	}
	switch typ {
	case "string":
		f.Type = map[string]string{"date": "date", "date-time": "datetime", "email": "email", "uri": "url"}[str("format")]
		if f.Type == "" {
			f.Type = "text"
			if n, _ := p["maxLength"].(float64); n >= 500 {
				f.Type = "long_text"
			}
		}
	case "number", "integer", "boolean":
		f.Type = typ
	case "array":
		items, _ := p["items"].(map[string]any)
		if sub, ok := items["properties"].(map[string]any); ok {
			f.Type, f.Subfields = "list", sortedKeys(sub)
		} else if it, _ := items["type"].(string); it == "string" {
			f.Type = "text_list"
		} else {
			f.Type = "other"
		}
	case "object":
		if sub, ok := p["properties"].(map[string]any); ok {
			f.Type, f.Subfields = "group", sortedKeys(sub)
		} else {
			f.Type = "json"
		}
	default:
		f.Type = "other"
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
