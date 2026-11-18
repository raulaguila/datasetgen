package schema

import (
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strings"
	"sync"
)

// WriteMetadata is immutable once published. Each revision of a collection's
// schema has its own entry, including schemas read by an older transaction.
type WriteMetadata struct {
	Autos                  []AutoField
	Formulas               map[string]FormulaSpec
	Virtual, Unique, Files []string
	Refs                   map[string]string
	ItemRefs               map[string]map[string]string
}

var writeCache = struct {
	sync.RWMutex
	entries map[[32]byte]*WriteMetadata
	order   [maxCompiled][32]byte
	next    int
}{entries: make(map[[32]byte]*WriteMetadata)}

// Metadata returns shared, read-only write rules.
func Metadata(raw []byte) *WriteMetadata {
	key := sha256.Sum256(raw)
	writeCache.RLock()
	m := writeCache.entries[key]
	writeCache.RUnlock()
	if m != nil {
		return m
	}
	m = readMetadata(raw)
	writeCache.Lock()
	defer writeCache.Unlock()
	if existing := writeCache.entries[key]; existing != nil {
		return existing
	}
	delete(writeCache.entries, writeCache.order[writeCache.next])
	writeCache.entries[key] = m
	writeCache.order[writeCache.next] = key
	writeCache.next = (writeCache.next + 1) % len(writeCache.order)
	return m
}

func readMetadata(raw []byte) *WriteMetadata {
	var root struct {
		Properties map[string]any `json:"properties"`
	}
	m := &WriteMetadata{Formulas: map[string]FormulaSpec{}, Refs: map[string]string{}, ItemRefs: map[string]map[string]string{}}
	if json.Unmarshal(raw, &root) != nil {
		return m
	}
	for key, property := range root.Properties {
		p, _ := property.(map[string]any)
		kind, _ := p[AutoKeyword].(string)
		switch kind {
		case AutoNumber, AutoCreatedBy, AutoUpdatedBy:
			m.Autos = append(m.Autos, AutoField{key, kind})
		}
		if expr, _ := p[FormulaKeyword].(string); expr != "" {
			typ, _ := p["type"].(string)
			format, _ := p["format"].(string)
			m.Formulas[key] = FormulaSpec{Expr: expr, Type: typ, Format: format}
		}
		if p[LookupKeyword] != nil || p[RollupKeyword] != nil {
			m.Virtual = append(m.Virtual, key)
		}
		if on, _ := p[UniqueKeyword].(bool); on {
			m.Unique = append(m.Unique, key)
		}
		if on, _ := p[FileKeyword].(bool); on {
			m.Files = append(m.Files, key)
		}
		if id, _ := p[RefKeyword].(string); id != "" {
			m.Refs[key] = id
		}
		items, _ := p["items"].(map[string]any)
		props, _ := items["properties"].(map[string]any)
		for sub, prop := range props {
			child, _ := prop.(map[string]any)
			if id, _ := child[RefKeyword].(string); id != "" {
				if m.ItemRefs[key] == nil {
					m.ItemRefs[key] = map[string]string{}
				}
				m.ItemRefs[key][sub] = id
			}
		}
	}
	slices.SortFunc(m.Autos, func(a, b AutoField) int { return strings.Compare(a.Key, b.Key) })
	slices.Sort(m.Unique)
	slices.Sort(m.Virtual)
	slices.Sort(m.Files)
	return m
}
