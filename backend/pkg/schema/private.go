package schema

import (
	"encoding/json"
	"slices"
)

// PrivateKeyword marks a top-level property kept off share links: their list
// never shows it and their form never asks for it.
const PrivateKeyword = "x-private"

// PrivateKeys lists the private properties, sorted.
func PrivateKeys(raw []byte) []string {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var keys []string
	for k, p := range doc.Properties {
		if on, _ := p[PrivateKeyword].(bool); on {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// HideSchema drops the keys from the schema's properties and required list.
func HideSchema(raw []byte, keys []string) []byte {
	if len(keys) == 0 {
		return raw
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	var props map[string]json.RawMessage
	if json.Unmarshal(doc["properties"], &props) == nil && props != nil {
		for _, k := range keys {
			delete(props, k)
		}
		doc["properties"], _ = json.Marshal(props)
	}
	var required []string
	if json.Unmarshal(doc["required"], &required) == nil && required != nil {
		doc["required"], _ = json.Marshal(slices.DeleteFunc(required, func(k string) bool { return slices.Contains(keys, k) }))
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// HideData drops the keys from a record's data; data that is not an object
// is returned as it is.
func HideData(data []byte, keys []string) []byte {
	if len(keys) == 0 {
		return data
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil || doc == nil {
		return data
	}
	for _, k := range keys {
		delete(doc, k)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return data
	}
	return out
}

// HasAny answers whether the data object sets any of the keys.
func HasAny(data []byte, keys []string) bool {
	var doc map[string]json.RawMessage
	if len(keys) == 0 || json.Unmarshal(data, &doc) != nil {
		return false
	}
	for _, k := range keys {
		if _, ok := doc[k]; ok {
			return true
		}
	}
	return false
}
