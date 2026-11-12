// Package schema compiles user JSON Schemas and validates documents against them.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
