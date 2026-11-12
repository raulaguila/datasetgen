package domain

import (
	"encoding/json"
	"time"
)

// RecordFilter selects one page of records of one collection.
type RecordFilter struct {
	CollectionID string
	Page         Page
	Search       string // matched against the JSON text of data
	Order        string // created_at asc or desc
}

// Record is one entry of a collection (app.record).
type Record struct {
	ID           string
	CollectionID string
	Data         json.RawMessage
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
