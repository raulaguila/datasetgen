package record

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/pkg/schema"
)

// withVirtual fills the lookups and rollups of records of one collection (in
// place). A field reading a collection the caller cannot see stays empty.
// shortcut: computed per read, so lists cannot sort or chart by them (conditions filter through a subquery per record); store them if that is asked for.
func (s *Service) withVirtual(ctx context.Context, collectionID string, recs []domain.Record) error {
	if len(recs) == 0 {
		return nil
	}
	c, err := s.Collections.FindByID(ctx, collectionID)
	if err != nil {
		return err
	}
	lookups, rollups := schema.Lookups(c.Schema), schema.Rollups(c.Schema)
	if len(lookups) == 0 && len(rollups) == 0 {
		return nil
	}
	refs := schema.RefKeys(c.Schema)
	seen := map[string]bool{}
	readable := func(id string) bool {
		ok, done := seen[id]
		if !done {
			_, err := policy.CollectionRole(ctx, s.Collections, id, domain.RoleViewer)
			ok = err == nil
			seen[id] = ok
		}
		return ok
	}
	docs := make([]map[string]json.RawMessage, len(recs))
	for i, r := range recs {
		if json.Unmarshal(r.Data, &docs[i]) != nil || docs[i] == nil {
			docs[i] = map[string]json.RawMessage{}
		}
	}
	for key, l := range lookups {
		target := refs[l.Ref]
		if target == "" || !readable(target) {
			continue
		}
		named := make([][]string, len(docs))
		var ids []string
		for i, d := range docs {
			named[i] = refIDs(d[l.Ref])
			ids = append(ids, named[i]...)
		}
		vals, err := s.Repo.LookupValues(ctx, target, ids, l.Field)
		if err != nil {
			return err
		}
		for i, d := range docs {
			var single bool
			if raw := d[l.Ref]; len(raw) > 0 && raw[0] == '"' {
				single = true
			}
			if single {
				if v, ok := vals[named[i][0]]; ok {
					d[key] = v
				}
				continue
			}
			list := []json.RawMessage{}
			for _, id := range named[i] {
				if v, ok := vals[id]; ok {
					list = append(list, v)
				}
			}
			if len(named[i]) > 0 {
				d[key], _ = json.Marshal(list)
			}
		}
	}
	for key, r := range rollups {
		if !readable(r.Collection) {
			continue
		}
		ids := make([]string, len(recs))
		for i, rec := range recs {
			ids[i] = rec.ID
		}
		vals, err := s.Repo.Rollup(ctx, r.Collection, r.Ref, ids, r.Fn, r.Field)
		if err != nil {
			return err
		}
		for i, rec := range recs {
			if v, ok := vals[rec.ID]; ok {
				docs[i][key] = v
			} else if r.Fn == "count" {
				docs[i][key] = json.RawMessage("0")
			}
		}
	}
	for i := range recs {
		out, err := json.Marshal(docs[i])
		if err != nil {
			return err
		}
		recs[i].Data = out
	}
	return nil
}

// refIDs reads a reference value: one id or a list of ids.
func refIDs(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(raw, &many)
	return many
}
