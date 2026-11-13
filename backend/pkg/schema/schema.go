// Package schema compiles user JSON Schemas and validates documents against them.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/raulaguila/dataset/pkg/apperror"
)

// pointerEscape writes a property name as one JSON pointer segment (RFC 6901).
var pointerEscape = strings.NewReplacer("~", "~0", "/", "~1")

const (
	location  = "mem://schema.json"
	maxIssues = 20
)

// Issue is one failed rule, pointing at the value that broke it.
type Issue struct {
	Path    string `json:"path"`    // JSON pointer into the document, "" for the root
	Keyword string `json:"keyword"` // the failed rule, such as "required"; clients translate by it
	Message string `json:"message"`
}

// noLoader refuses every external $ref: a schema must not read server files or URLs.
type noLoader struct{}

func (noLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external $ref not allowed: %s", url)
}

// Compile parses a JSON Schema (draft 2020-12 unless $schema says otherwise).
func Compile(raw []byte) (*jsonschema.Schema, error) {
	invalid := func(msg string) error {
		return apperror.New(apperror.CodeInvalidInput, "invalid schema: "+msg).WithField("schema")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, invalid("not JSON")
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, invalid("must be a JSON object")
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(noLoader{})
	c.AssertFormat() // 2020-12 treats format as a note by default; a dataset wants it enforced
	if err := c.AddResource(location, doc); err != nil {
		return nil, invalid(err.Error())
	}
	compiled, err := c.Compile(location)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return compiled, nil
}

// Validate checks raw against compiled and answers VALIDATION_FAILED with the issues in details.
func Validate(compiled *jsonschema.Schema, raw []byte) error {
	issues, err := Check(compiled, raw)
	if err != nil || len(issues) == 0 {
		return err
	}
	return apperror.New(apperror.CodeValidationFailed, "data does not match the collection schema").
		WithField("data").WithDetails("issues", issues)
}

// Check returns the rules raw breaks, none when it matches (at most 20).
func Check(compiled *jsonschema.Schema, raw []byte) ([]Issue, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "data must be JSON").WithField("data")
	}
	err = compiled.Validate(doc)
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil, err
	}
	issues := []Issue{}
	for _, unit := range ve.BasicOutput().Errors {
		if unit.Error == nil {
			continue
		}
		path := unit.Error.Kind.KeywordPath()
		keyword := ""
		if len(path) > 0 {
			keyword = path[len(path)-1]
		}
		// "required" fails on the parent object; point at each missing field instead.
		if r, ok := unit.Error.Kind.(*kind.Required); ok {
			for _, name := range r.Missing {
				issues = append(issues, Issue{Path: unit.InstanceLocation + "/" + pointerEscape.Replace(name), Keyword: keyword, Message: "missing property"})
			}
			continue
		}
		issues = append(issues, Issue{Path: unit.InstanceLocation, Keyword: keyword, Message: unit.Error.String()})
	}
	if len(issues) == 0 { // a failure must never read as a match
		issues = append(issues, Issue{Message: ve.Error()})
	}
	if len(issues) > maxIssues {
		issues = issues[:maxIssues]
	}
	return issues, nil
}

// PropertyOrder lists the top-level property names as the schema text writes
// them; Go maps would lose the order the form shows. Nil when there are none.
func PropertyOrder(raw []byte) []string {
	var doc struct {
		Properties json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Properties) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(doc.Properties))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		key, _ := t.(string)
		keys = append(keys, key)
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

// maxTitle caps the title an audit entry keeps for a record.
const maxTitle = 120

// Title is what a record reads as: its first top-level text value, in the
// schema's field order, as the app shows it. "" when it has none.
func Title(raw, data []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return ""
	}
	for _, key := range PropertyOrder(raw) {
		var s string
		if json.Unmarshal(fields[key], &s) == nil && strings.TrimSpace(s) != "" {
			if r := []rune(s); len(r) > maxTitle {
				return string(r[:maxTitle-1]) + "…"
			}
			return s
		}
	}
	return ""
}

// FieldChanges names the top-level fields a schema change added, removed or
// changed, each by its title (its key when untitled). Nil when none moved.
func FieldChanges(before, after []byte) map[string][]string {
	props := func(raw []byte) map[string]json.RawMessage {
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		_ = json.Unmarshal(raw, &doc)
		return doc.Properties
	}
	label := func(key string, p json.RawMessage) string {
		var f struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(p, &f) == nil && f.Title != "" {
			return f.Title
		}
		return key
	}
	canon := func(p json.RawMessage) string {
		var v any
		_ = json.Unmarshal(p, &v)
		b, _ := json.Marshal(v)
		return string(b)
	}
	old, cur := props(before), props(after)
	out := map[string][]string{}
	for _, key := range PropertyOrder(after) {
		p, ok := old[key]
		switch {
		case !ok:
			out["added"] = append(out["added"], label(key, cur[key]))
		case canon(p) != canon(cur[key]):
			out["changed"] = append(out["changed"], label(key, cur[key]))
		}
	}
	for _, key := range PropertyOrder(before) {
		if _, ok := cur[key]; !ok {
			out["removed"] = append(out["removed"], label(key, old[key]))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// UniqueKeyword marks a top-level property whose value no two records of a
// collection may share. JSON Schema has no such rule; the server checks it.
const UniqueKeyword = "x-unique"

// UniqueKeys lists the top-level properties marked unique, sorted.
func UniqueKeys(raw []byte) []string {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var keys []string
	for k, p := range doc.Properties {
		if on, _ := p[UniqueKeyword].(bool); on {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// UniqueIssue says a record repeats a value another record holds under a unique property.
func UniqueIssue(key string) Issue {
	return Issue{Path: "/" + pointerEscape.Replace(key), Keyword: "unique", Message: "value already used by another record"}
}
