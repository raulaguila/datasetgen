// Package schema compiles user JSON Schemas and validates documents against them.
package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

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

// compiled caches Compile by the schema's text: a collection's schema
// changes rarely and every record write compiles it.
var compiled = struct {
	sync.Mutex
	m map[[32]byte]*jsonschema.Schema
}{m: map[[32]byte]*jsonschema.Schema{}}

// maxCompiled bounds the cache; past it the cache starts over.
const maxCompiled = 512

// Compile parses a JSON Schema (draft 2020-12 unless $schema says otherwise).
// A compiled schema is read only, so callers share the cached one.
func Compile(raw []byte) (*jsonschema.Schema, error) {
	sum := sha256.Sum256(raw)
	compiled.Lock()
	hit := compiled.m[sum]
	compiled.Unlock()
	if hit != nil {
		return hit, nil
	}
	out, err := compile(raw)
	if err != nil {
		return nil, err
	}
	compiled.Lock()
	if len(compiled.m) >= maxCompiled {
		clear(compiled.m)
	}
	compiled.m[sum] = out
	compiled.Unlock()
	return out, nil
}

func compile(raw []byte) (*jsonschema.Schema, error) {
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
	for key, src := range Formulas(raw) {
		if _, err := ParseFormula(src); err != nil {
			return nil, invalid("formula of " + key + ": " + err.Error())
		}
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

// RefKeyword holds, on a top-level string (one record) or array of strings
// (several) property, the id of the collection whose records its value names; FileKeyword marks a property holding an attached
// file ({"id", "name", "size", "type"}). The server checks both exist.
const (
	RefKeyword  = "x-ref"
	FileKeyword = "x-file"
)

// RefKeys maps each reference property to the collection it points at.
func RefKeys(raw []byte) map[string]string {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	out := map[string]string{}
	for k, p := range doc.Properties {
		if id, _ := p[RefKeyword].(string); id != "" {
			out[k] = id
		}
	}
	return out
}

// ItemRefKeys finds references inside a list of objects: for each top-level
// array property, its item subfields marked with RefKeyword and the
// collection each names.
func ItemRefKeys(raw []byte) map[string]map[string]string {
	var doc struct {
		Properties map[string]struct {
			Type  any `json:"type"`
			Items struct {
				Properties map[string]map[string]any `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	out := map[string]map[string]string{}
	for k, p := range doc.Properties {
		for sub, sp := range p.Items.Properties {
			if id, _ := sp[RefKeyword].(string); id != "" {
				if out[k] == nil {
					out[k] = map[string]string{}
				}
				out[k][sub] = id
			}
		}
	}
	return out
}

// FileKeys lists the properties holding attached files, sorted.
func FileKeys(raw []byte) []string {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var keys []string
	for k, p := range doc.Properties {
		if on, _ := p[FileKeyword].(bool); on {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Optional drops keys from the top-level "required" list; raw comes back
// unchanged when none of them is required.
func Optional(raw []byte, keys []string) []byte {
	var doc map[string]json.RawMessage
	var req []string
	if len(keys) == 0 || json.Unmarshal(raw, &doc) != nil || json.Unmarshal(doc["required"], &req) != nil {
		return raw
	}
	kept := slices.DeleteFunc(slices.Clone(req), func(k string) bool { return slices.Contains(keys, k) })
	if len(kept) == len(req) {
		return raw
	}
	doc["required"], _ = json.Marshal(kept)
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// MissingIssue names a reference or file the value points at that does not exist.
func MissingIssue(key string) Issue {
	return Issue{Path: "/" + pointerEscape.Replace(key), Keyword: "exists", Message: "points at a record or file that does not exist"}
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

// Leftovers returns a check for keys a document holds that the schema no
// longer has, such as the value of a removed field, at the top level and
// inside groups and lists of objects. JSON Schema lets them pass unless the
// schema says otherwise, so an object that sets "additionalProperties" itself
// gets no extra check (nil for the whole schema when the top level sets it).
func Leftovers(raw []byte) func(data []byte) []Issue {
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	if _, set := doc["additionalProperties"]; set {
		return nil
	}
	root := shapeOf(raw, true)
	return func(data []byte) []Issue {
		var v any
		if json.Unmarshal(data, &v) != nil {
			return nil
		}
		var issues []Issue
		root.walk("", v, &issues)
		slices.SortFunc(issues, func(a, b Issue) int { return strings.Compare(a.Path, b.Path) })
		return issues
	}
}

// shape is the part of a schema Leftovers walks: the known keys of an object
// (closed when other keys are leftovers) and the shape of a list's items.
type shape struct {
	props  map[string]*shape
	closed bool
	items  *shape
}

func shapeOf(raw []byte, top bool) *shape {
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Items      json.RawMessage            `json:"items"`
		Additional json.RawMessage            `json:"additionalProperties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	// A nested object without properties is free-form JSON, not a group.
	s := &shape{props: map[string]*shape{}, closed: doc.Additional == nil && (top || doc.Properties != nil)}
	for k, p := range doc.Properties {
		s.props[k] = shapeOf(p, false)
	}
	if len(doc.Items) > 0 && doc.Items[0] == '{' {
		s.items = shapeOf(doc.Items, false)
	}
	return s
}

func (s *shape) walk(path string, v any, out *[]Issue) {
	if s == nil {
		return
	}
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			p := path + "/" + pointerEscape.Replace(k)
			if sub, ok := s.props[k]; ok {
				sub.walk(p, child, out)
			} else if s.closed {
				*out = append(*out, Issue{Path: p, Keyword: "additionalProperties", Message: "field no longer in the schema"})
			}
		}
	case []any:
		for i, item := range v {
			s.items.walk(path+"/"+strconv.Itoa(i), item, out)
		}
	}
}
