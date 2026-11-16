package record

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/schema"
)

// fill writes the fields the server owns into each doc (in place): the next
// number, who made or changed the record, and formulas; lookups and rollups
// are dropped. creating is true for
// new records. Docs that are not objects are left for the schema to refuse.
func fill(ctx context.Context, uow output.UnitOfWork, raw []byte, collectionID string, docs [][]byte, creating bool) error {
	autos, formulas, virtual := schema.AutoKeys(raw), schema.Formulas(raw), schema.VirtualKeys(raw)
	if len(autos) == 0 && len(formulas) == 0 && len(virtual) == 0 {
		return nil
	}
	parsed := make(map[string]*schema.Formula, len(formulas))
	for k, spec := range formulas {
		f, err := schema.ParseFormula(spec.Expr)
		if err != nil {
			return err // Compile refused it already
		}
		parsed[k] = f
	}
	now := time.Now()
	var who json.RawMessage
	next := map[string]int64{}
	for _, a := range autos {
		switch {
		case a.Kind == schema.AutoNumber && creating:
			// The unique lock keeps two writers from taking the same number.
			if err := uow.Records().LockUnique(ctx, collectionID); err != nil {
				return err
			}
			n, err := uow.Records().MaxNumber(ctx, collectionID, a.Key)
			if err != nil {
				return err
			}
			next[a.Key] = n
		case (a.Kind == schema.AutoCreatedBy && creating) || a.Kind == schema.AutoUpdatedBy:
			if who == nil {
				name := "—"
				if actor := reqctx.Actor(ctx); actor != "" {
					if u, err := uow.Users().FindByID(ctx, actor); err == nil {
						name = u.Name
					}
				}
				who, _ = json.Marshal(name)
			}
		}
	}
	for i, d := range docs {
		var doc map[string]json.RawMessage
		if json.Unmarshal(d, &doc) != nil || doc == nil {
			continue
		}
		for _, k := range virtual {
			delete(doc, k) // computed on read, never stored
		}
		for _, a := range autos {
			switch {
			case a.Kind == schema.AutoNumber && creating:
				next[a.Key]++
				doc[a.Key], _ = json.Marshal(next[a.Key])
			case a.Kind == schema.AutoCreatedBy && creating, a.Kind == schema.AutoUpdatedBy:
				doc[a.Key] = who
			}
		}
		for k, f := range parsed {
			v, ok := f.Eval(doc, now)
			if x, isNum := v.(float64); isNum {
				v = math.Round(x*1e9) / 1e9 // drop float noise such as 0.30000000000000004
			}
			if ok && kindOf(v) == formulas[k].Type {
				doc[k], _ = json.Marshal(v)
			} else {
				delete(doc, k) // no value, or one the field's type cannot hold
			}
		}
		out, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		docs[i] = out
	}
	return nil
}

// kindOf names a formula value's JSON type.
func kindOf(v any) string {
	switch v.(type) {
	case float64:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	}
	return ""
}
