package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
)

// fieldUse is a tool argument that names fields of the collection.
type fieldUse struct {
	arg   string
	names []string
}

// checkFields refuses field keys the collection lacks and option values it does not offer.
// A model's slip (a field's name for its key, a value in the wrong case) would otherwise
// match nothing and read as "no records"; the error names what to send instead.
// A collection without fields in its schema is left unchecked.
func (t tools) checkFields(ctx context.Context, collectionID string, where *domain.Where, uses ...fieldUse) error {
	conds := conditions(where)
	if len(conds) == 0 && !slices.ContainsFunc(uses, func(u fieldUse) bool { return len(u.names) > 0 }) {
		return nil
	}
	c, err := t.app.Collections.Get(ctx, collectionID)
	if err != nil {
		return err
	}
	fields := schema.Fields(c.Schema)
	if len(fields) == 0 {
		return nil
	}
	for _, u := range uses {
		for _, name := range u.names {
			if _, err := field(fields, u.arg, name); err != nil {
				return err
			}
		}
	}
	for _, c := range conds {
		if c.Field == domain.FieldCreatedAt || c.Field == domain.FieldUpdatedAt {
			continue
		}
		f, err := field(fields, "rules", c.Field)
		if err != nil {
			return err
		}
		if err := optionValues(f, c); err != nil {
			return err
		}
	}
	return nil
}

// one is a single optional field name as a list.
func one(name string) []string {
	if name == "" {
		return nil
	}
	return []string{name}
}

func conditions(w *domain.Where) []domain.Condition {
	if w == nil {
		return nil
	}
	out := slices.Clone(w.Rules)
	for _, g := range w.Groups {
		out = append(out, g.Rules...)
	}
	return out
}

// field finds a field by key, or says which key was meant and lists them all.
func field(fields []schema.Field, arg, name string) (schema.Field, error) {
	keys := make([]string, len(fields))
	for i, f := range fields {
		if f.Key == name {
			return f, nil
		}
		keys[i] = f.Key
	}
	msg := fmt.Sprintf("%s: the collection has no field %q", arg, name)
	for _, f := range fields {
		if fold(f.Key) == fold(name) || (f.Name != "" && fold(f.Name) == fold(name)) {
			msg += fmt.Sprintf(`; use its key %q`, f.Key)
			break
		}
	}
	return schema.Field{}, apperror.New(apperror.CodeInvalidInput, msg+"; field keys: "+list(keys)).WithField(arg)
}

// optionValues refuses eq, neq, in and not_in values an option field does not offer.
func optionValues(f schema.Field, c domain.Condition) error {
	if f.Type != "option" || len(f.Options) == 0 {
		return nil
	}
	var values []any
	switch c.Op {
	case domain.OpEq, domain.OpNeq:
		var v any
		if json.Unmarshal(c.Value, &v) == nil {
			values = []any{v}
		}
	case domain.OpIn, domain.OpNotIn:
		_ = json.Unmarshal(c.Value, &values)
	}
	for _, v := range values {
		s, ok := v.(string)
		if !ok || slices.Contains(f.Options, s) {
			continue
		}
		msg := fmt.Sprintf("rules: %q is not an option of %s", s, f.Key)
		for _, o := range f.Options {
			if fold(o) == fold(s) {
				msg += fmt.Sprintf("; use %q", o)
				break
			}
		}
		return apperror.New(apperror.CodeInvalidInput, msg+"; options: "+list(f.Options)).WithField("rules")
	}
	return nil
}

// list joins names for an error, the first 60 of them.
func list(names []string) string {
	if len(names) > 60 {
		return strings.Join(names[:60], ", ") + ", …"
	}
	return strings.Join(names, ", ")
}

// fold compares names ignoring case and accents: "Condição" and "condicao" are the same.
func fold(s string) string {
	out, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), s)
	if err != nil {
		out = s
	}
	return strings.ToLower(strings.TrimSpace(out))
}
