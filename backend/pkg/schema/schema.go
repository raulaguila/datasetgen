// Package schema compiles user JSON Schemas and validates documents against them.
package schema

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/raulaguila/dataset/pkg/apperror"
)

const (
	location  = "mem://schema.json"
	maxIssues = 20
)

// Issue is one failed rule, pointing at the value that broke it.
type Issue struct {
	Path    string `json:"path"` // JSON pointer into the document, "" for the root
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
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return apperror.New(apperror.CodeInvalidInput, "data must be JSON").WithField("data")
	}
	err = compiled.Validate(doc)
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	issues := []Issue{}
	for _, unit := range ve.BasicOutput().Errors {
		if unit.Error == nil || len(issues) == maxIssues {
			continue
		}
		issues = append(issues, Issue{Path: unit.InstanceLocation, Message: unit.Error.String()})
	}
	return apperror.New(apperror.CodeValidationFailed, "data does not match the collection schema").
		WithField("data").WithDetails("issues", issues)
}
