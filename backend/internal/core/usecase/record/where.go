package record

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/schema"
)

// resolveWhere returns a copy of the filter's rules ready for the query:
// {"today": n} becomes a date in the caller's time zone, and a lookup or
// rollup field names what it reads, once the caller may read that collection.
// The caller's Where stays as it was (an automation keeps its relative dates).
func (s *Service) resolveWhere(ctx context.Context, filter domain.RecordFilter) (*domain.Where, error) {
	if filter.Where.Empty() {
		return filter.Where, nil
	}
	loc := time.UTC
	if filter.TZ != "" {
		l, err := time.LoadLocation(filter.TZ)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidInput, "tz must be an IANA time zone").WithField("tz")
		}
		loc = l
	}
	today := time.Now().In(loc)
	var w domain.Where
	raw, _ := json.Marshal(filter.Where) // a Where always marshals
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	var lookups map[string]schema.Lookup
	var rollups map[string]schema.Rollup
	var refs map[string]string
	loaded := false
	readable := map[string]bool{}
	resolve := func(c *domain.Condition) error {
		c.Value = withDates(c.Value, today)
		if c.Field == domain.FieldCreatedAt || c.Field == domain.FieldUpdatedAt {
			return nil
		}
		if !loaded {
			col, err := s.Collections.FindByID(ctx, filter.CollectionID)
			if err != nil {
				return err
			}
			lookups, rollups, refs, loaded = schema.Lookups(col.Schema), schema.Rollups(col.Schema), schema.RefKeys(col.Schema), true
		}
		target := ""
		if l, ok := lookups[c.Field]; ok {
			target = refs[l.Ref]
			c.Lookup = &domain.LookupRule{Collection: target, Ref: l.Ref, Field: l.Field}
		} else if r, ok := rollups[c.Field]; ok {
			target = r.Collection
			c.Rollup = &domain.RollupRule{Collection: r.Collection, Ref: r.Ref, Fn: r.Fn, Field: r.Field}
		} else {
			return nil
		}
		ok, seen := readable[target]
		if !seen {
			_, err := policy.CollectionRole(ctx, s.Collections, target, domain.RoleViewer)
			ok = err == nil && target != ""
			readable[target] = ok
		}
		if !ok {
			return apperror.New(apperror.CodeForbidden, "the field "+c.Field+" reads a collection you cannot see").WithField("where")
		}
		return nil
	}
	for _, rules := range append([][]domain.Condition{w.Rules}, groupRules(w.Groups)...) {
		for i := range rules {
			if err := resolve(&rules[i]); err != nil {
				return nil, err
			}
		}
	}
	return &w, nil
}

func groupRules(groups []domain.Where) [][]domain.Condition {
	out := make([][]domain.Condition, len(groups))
	for i := range groups {
		out[i] = groups[i].Rules
	}
	return out
}

// withDates turns {"today": n}, alone or inside [from, to], into YYYY-MM-DD.
func withDates(value json.RawMessage, today time.Time) json.RawMessage {
	var v any
	if json.Unmarshal(value, &v) != nil {
		return value
	}
	date := func(x any) any {
		if n := isToday(x); n != nil {
			return today.AddDate(0, 0, *n).Format(time.DateOnly)
		}
		return x
	}
	if list, ok := v.([]any); ok {
		for i := range list {
			list[i] = date(list[i])
		}
		v = list
	} else {
		v = date(v)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return value
	}
	return out
}
