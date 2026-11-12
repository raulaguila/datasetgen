package domain

import (
	"encoding/json"
	"time"
)

// CollectionFilter selects one page of collections.
type CollectionFilter struct {
	Page   Page
	Search string
	Sort   string
	Order  string
}

// Collection groups records that follow one JSON Schema (app.collection).
type Collection struct {
	ID          string
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema, draft 2020-12
	RecordCount int64           // filled by List only
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CollectionPatch holds the fields of an update; nil keeps the stored value.
type CollectionPatch struct {
	Name        *string
	Description *string
	Schema      json.RawMessage
	Version     *int // the version the caller read; a newer row is a conflict
}
