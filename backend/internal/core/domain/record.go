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
	Sort         string // created_at, updated_at or data.<field> (a top-level field)
	Order        string // asc or desc
	// Match keeps records whose data contains this JSON object (data @> Match),
	// so {"category": "trigger"} filters on one field's exact value.
	Match json.RawMessage
	// IDs keeps only these records (up to 100), to look up several by id.
	IDs []string
}

// SortField is the data field a Sort of "data.<field>" orders by, "" otherwise.
func (f RecordFilter) SortField() string {
	if len(f.Sort) > 5 && f.Sort[:5] == "data." {
		return f.Sort[5:]
	}
	return ""
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

// RecordVersion is a record's data as it was before a change replaced it.
type RecordVersion struct {
	Version int
	Data    json.RawMessage
	SavedAt time.Time // when this version was written
}

// RecordFix is one bulk change to the data of a collection's records.
type RecordFix struct {
	Op    string          // set_default, rename or remove
	Field string          // the field changed; the old name for rename
	To    string          // rename only: the new name
	Value json.RawMessage // set_default only: the value given to records without the field
	// DryRun counts the records the change would touch and changes nothing.
	DryRun bool
}

const (
	FixSetDefault = "set_default"
	FixRename     = "rename"
	FixRemove     = "remove"
)
