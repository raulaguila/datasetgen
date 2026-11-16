package schema

import (
	"encoding/json"
	"fmt"
	"slices"
)

// LookupKeyword marks a top-level property that shows a field of the record a
// reference property of the same record names: {"ref": key, "field": key}.
// RollupKeyword marks one that sums up the records of another collection
// whose reference field names this record:
// {"collection": id, "ref": key, "fn": "count"|"sum"|"avg"|"min"|"max", "field": key}.
// Both are computed when records are read and never stored: a write drops them.
const (
	LookupKeyword = "x-lookup"
	RollupKeyword = "x-rollup"
)

// Rollup functions.
var RollupFns = []string{"count", "sum", "avg", "min", "max"}

type Lookup struct {
	Ref   string `json:"ref"`
	Field string `json:"field"`
}

type Rollup struct {
	Collection string `json:"collection"`
	Ref        string `json:"ref"`
	Fn         string `json:"fn"`
	Field      string `json:"field,omitempty"` // left out for count
}

// Lookups maps each lookup property to its settings; malformed ones are left out.
func Lookups(raw []byte) map[string]Lookup {
	out := map[string]Lookup{}
	for k, p := range topProps(raw) {
		var l Lookup
		if decode(p[LookupKeyword], &l) && l.Ref != "" && l.Field != "" {
			out[k] = l
		}
	}
	return out
}

// Rollups maps each rollup property to its settings; malformed ones are left out.
func Rollups(raw []byte) map[string]Rollup {
	out := map[string]Rollup{}
	for k, p := range topProps(raw) {
		var r Rollup
		if decode(p[RollupKeyword], &r) && r.Collection != "" && r.Ref != "" && slices.Contains(RollupFns, r.Fn) && (r.Fn == "count" || r.Field != "") {
			out[k] = r
		}
	}
	return out
}

// VirtualKeys lists the properties computed on read.
func VirtualKeys(raw []byte) []string {
	var out []string
	for k, p := range topProps(raw) {
		if p[LookupKeyword] != nil || p[RollupKeyword] != nil {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func decode(v any, into any) bool {
	if v == nil {
		return false
	}
	b, err := json.Marshal(v)
	return err == nil && json.Unmarshal(b, into) == nil
}

// checkVirtual refuses lookups and rollups the reader could not compute: a
// lookup must name a reference property of this schema, a rollup a known function.
func checkVirtual(raw []byte) error {
	props := topProps(raw)
	refs := RefKeys(raw)
	lookups, rollups := Lookups(raw), Rollups(raw)
	for k, p := range props {
		if p[LookupKeyword] != nil {
			l, ok := lookups[k]
			if !ok {
				return fmt.Errorf("lookup %s needs {\"ref\", \"field\"}", k)
			}
			if _, ok := refs[l.Ref]; !ok {
				return fmt.Errorf("lookup %s: %s is not a reference field", k, l.Ref)
			}
		}
		if p[RollupKeyword] != nil {
			if _, ok := rollups[k]; !ok {
				return fmt.Errorf("rollup %s needs {\"collection\", \"ref\", \"fn\": count, sum, avg, min or max, \"field\"}", k)
			}
		}
		if p[LookupKeyword] != nil && p[RollupKeyword] != nil {
			return fmt.Errorf("%s cannot be both a lookup and a rollup", k)
		}
	}
	return nil
}

// ScalarType is the JSON type of a top-level property that holds one string,
// number, integer or boolean; for a list of them, the items' type and list true.
// ok is false for anything else (objects, files, missing keys).
func ScalarType(raw []byte, key string) (typ string, list, ok bool) {
	p, found := topProps(raw)[key]
	if !found {
		return "", false, false
	}
	scalar := func(t any) (string, bool) {
		s, _ := t.(string)
		return s, s == "string" || s == "number" || s == "integer" || s == "boolean"
	}
	if p["type"] == "array" {
		items, _ := p["items"].(map[string]any)
		t, ok := scalar(items["type"])
		return t, true, ok
	}
	t, ok := scalar(p["type"])
	return t, false, ok
}
