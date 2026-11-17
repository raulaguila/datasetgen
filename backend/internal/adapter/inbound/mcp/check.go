package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
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

const maxExpand = 5

// expandIn names reference fields whose records come along, so the model reads a client's name without another call.
type expandIn struct {
	Expand []string `json:"expand,omitempty" jsonschema:"keys of reference fields whose records come along in each record's expanded, e.g. [\"client\"]; \"client.name\" brings only that field of them. Up to 5."`
}

type refOut struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// expand fills out[i].Expanded from the references in raw[i] (the data before fields cut it),
// reading each referenced collection once per 100 ids, as the key's member.
func (t tools) expand(ctx context.Context, collectionID string, asked []string, out []recordOut, raw []domain.Record) error {
	if len(asked) == 0 || len(raw) == 0 {
		return nil
	}
	subs := map[string][]string{} // reference field -> fields of the referenced records to keep (none: all)
	var order []string
	for _, a := range asked {
		field, sub, _ := strings.Cut(a, ".")
		if _, seen := subs[field]; !seen {
			order = append(order, field)
			subs[field] = nil
		}
		if sub != "" {
			subs[field] = append(subs[field], sub)
		}
	}
	if len(order) > maxExpand {
		return apperror.New(apperror.CodeInvalidInput, "expand names up to 5 reference fields").WithField("expand")
	}
	c, err := t.app.Collections.Get(ctx, collectionID)
	if err != nil {
		return err
	}
	fields := schema.Fields(c.Schema)
	for _, name := range order {
		f, err := field(fields, "expand", name)
		if err != nil {
			return err
		}
		if f.Type != "reference" && f.Type != "references" {
			return apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("expand: %s is a %s field, not a reference", name, f.Type)).WithField("expand")
		}
		ids := make([][]string, len(raw))
		var all []string
		for i, r := range raw {
			ids[i] = refIDs(r.Data, name)
			all = append(all, ids[i]...)
		}
		slices.Sort(all)
		found := map[string]json.RawMessage{}
		for chunk := range slices.Chunk(slices.Compact(all), maxBulk) {
			items, _, err := t.app.Records.List(ctx, domain.RecordFilter{CollectionID: f.Target, IDs: chunk, Page: domain.NewPage(1, maxBulk)})
			if err != nil {
				if apperror.IsCode(err, apperror.CodeNotFound) {
					return apperror.New(apperror.CodeInvalidInput, "expand: "+name+" points at a collection this key cannot read").WithField("expand")
				}
				return err
			}
			for _, it := range items {
				found[it.ID] = domain.KeepFields(it.Data, subs[name])
			}
		}
		for i := range out {
			for _, id := range ids[i] {
				if data, ok := found[id]; ok {
					if out[i].Expanded == nil {
						out[i].Expanded = map[string][]refOut{}
					}
					out[i].Expanded[name] = append(out[i].Expanded[name], refOut{ID: id, Data: data})
				}
			}
		}
	}
	return nil
}

// refIDs reads the record ids a reference field holds: one id, or a list of them.
func refIDs(data json.RawMessage, field string) []string {
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var many []string
	var one string
	if json.Unmarshal(doc[field], &one) == nil {
		many = []string{one}
	} else {
		_ = json.Unmarshal(doc[field], &many)
	}
	// An id that is not a uuid points nowhere; leaving it out keeps the lookup from failing.
	return slices.DeleteFunc(many, func(id string) bool { return uuid.Validate(id) != nil })
}
