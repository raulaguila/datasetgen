package policy

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/schema"
	"github.com/raulaguila/dataset/pkg/validate"
)

// MissingLinks returns, per document, the reference and file fields whose value
// names a record of the referenced collection, or a file of this one, that
// does not exist. Saving a record refuses them; the quality check reports them.
func MissingLinks(ctx context.Context, tools output.ToolsRepository, raw []byte, collectionID string, docs [][]byte) ([][]schema.Issue, error) {
	out := make([][]schema.Issue, len(docs))
	meta := schema.Metadata(raw)
	refs, files, itemRefs := meta.Refs, meta.Files, meta.ItemRefs
	if len(refs) == 0 && len(files) == 0 && len(itemRefs) == 0 {
		return out, nil
	}
	parsed := make([]map[string]json.RawMessage, len(docs))
	for i, doc := range docs {
		_ = json.Unmarshal(doc, &parsed[i]) // not an object: the schema already refused it
	}
	check := func(key, target string, idsOf func(json.RawMessage) []string, exist func(context.Context, string, []string) (map[string]bool, error)) error {
		var ids []string
		for _, p := range parsed {
			ids = append(ids, idsOf(p[key])...)
		}
		if len(ids) == 0 {
			return nil
		}
		found, err := exist(ctx, target, slices.Compact(slices.Sorted(slices.Values(ids))))
		if err != nil {
			return err
		}
		for i, p := range parsed {
			for _, id := range idsOf(p[key]) {
				if !found[id] {
					out[i] = append(out[i], schema.MissingIssue(key))
					break
				}
			}
		}
		return nil
	}
	// A reference holds one record id, or a list of them (several records).
	refIDs := func(v json.RawMessage) []string {
		var one string
		if json.Unmarshal(v, &one) == nil {
			if validate.ID(one) != nil {
				return nil
			}
			return []string{one}
		}
		var many []string
		if json.Unmarshal(v, &many) != nil {
			return nil
		}
		return slices.DeleteFunc(many, func(id string) bool { return validate.ID(id) != nil })
	}
	// A file field holds one {"id", ...} or a list of them (several files).
	fileID := func(v json.RawMessage) []string {
		type ref struct {
			ID string `json:"id"`
		}
		var many []ref
		if json.Unmarshal(v, &many) != nil {
			var one ref
			if json.Unmarshal(v, &one) != nil {
				return nil
			}
			many = []ref{one}
		}
		var ids []string
		for _, f := range many {
			if validate.ID(f.ID) == nil {
				ids = append(ids, f.ID)
			}
		}
		return ids
	}
	for _, key := range slices.Sorted(maps.Keys(refs)) {
		// shortcut: existence only; the caller may not read the referenced collection.
		if err := check(key, refs[key], refIDs, tools.RecordsExist); err != nil {
			return nil, err
		}
	}
	for _, key := range files {
		if err := check(key, collectionID, fileID, tools.FilesExist); err != nil {
			return nil, err
		}
	}
	// A reference subfield of a list of objects: the ids its items hold.
	for _, key := range slices.Sorted(maps.Keys(itemRefs)) {
		for _, sub := range slices.Sorted(maps.Keys(itemRefs[key])) {
			itemIDs := func(v json.RawMessage) []string {
				var items []map[string]json.RawMessage
				if json.Unmarshal(v, &items) != nil {
					return nil
				}
				var ids []string
				for _, it := range items {
					ids = append(ids, refIDs(it[sub])...)
				}
				return ids
			}
			if err := check(key, itemRefs[key][sub], itemIDs, tools.RecordsExist); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
