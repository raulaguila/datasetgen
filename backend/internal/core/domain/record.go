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
	// Where keeps records that pass its rules (all, or any of them).
	Where *Where
	// TZ is the caller's IANA time zone, for {"today": n} dates; UTC when empty.
	TZ string
}

// RecordMaxBytes caps one record's data as Postgres stores it
// (chk_app_record_data_size, uncompressed jsonb): about 250 000 tokens of text.
const RecordMaxBytes = 1 << 20

// Where operators. Values compare as JSON: numbers as numbers, strings
// (ISO dates too) as text, so gt and lt work on both. A date value
// (YYYY-MM-DD, or {"today": n}, the caller's today plus n days) compares
// with the first 10 characters of the field, so a date-time matches by its day.
const (
	OpEq          = "eq"
	OpNeq         = "neq"
	OpContains    = "contains"     // text holds the value, ignoring case
	OpNotContains = "not_contains" // text does not hold the value (empty counts)
	OpStartsWith  = "starts_with"  // text begins with the value, ignoring case
	OpGt          = "gt"
	OpGte         = "gte"
	OpLt          = "lt"
	OpLte         = "lte"
	OpBetween     = "between" // value is [from, to], both ends included
	OpEmpty       = "empty"   // absent, null, "" or []
	OpNotEmpty    = "not_empty"
	OpIn          = "in"      // value is an array; the field equals one of its items
	OpNotIn       = "not_in"  // the field equals none of the items (empty counts)
	OpHas         = "has"     // the field is an array holding the value
	OpHasAny      = "has_any" // the field is an array holding one of the items
)

// WhereOps lists the operators a rule may use.
var WhereOps = []string{OpEq, OpNeq, OpContains, OpNotContains, OpStartsWith, OpGt, OpGte, OpLt, OpLte, OpBetween, OpEmpty, OpNotEmpty, OpIn, OpNotIn, OpHas, OpHasAny}

// The record's own dates, usable as a rule's field.
const (
	FieldCreatedAt = "$created_at"
	FieldUpdatedAt = "$updated_at"
)

// Condition compares one top-level field with a JSON value.
type Condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
	// Set by the record service for a lookup or rollup field the caller may
	// read; never read from a request.
	Lookup *LookupRule `json:"-"`
	Rollup *RollupRule `json:"-"`
}

// LookupRule reads Field of the records of Collection that this record's Ref names.
type LookupRule struct{ Collection, Ref, Field string }

// RollupRule aggregates (Fn) Field over the records of Collection whose Ref names this record.
type RollupRule struct{ Collection, Ref, Fn, Field string }

// Where is a set of rules joined by AND, or by OR when Any is set.
type Where struct {
	Any   bool        `json:"any"`
	Rules []Condition `json:"rules"`
	// Groups are bracketed sets of rules, each joined by its own Any, then
	// joined with Rules by this Any: (a AND b) OR (c AND d). One level only.
	Groups []Where `json:"groups,omitempty"`
}

// Empty says the filter has no rule at all.
func (w *Where) Empty() bool {
	if w == nil {
		return true
	}
	for i := range w.Groups {
		if !w.Groups[i].Empty() {
			return false
		}
	}
	return len(w.Rules) == 0
}

// RecordGroup is the count of records sharing one value of a field, with
// the sum and average of numeric fields over them. Key is null for the
// totals of the whole selection.
type RecordGroup struct {
	Key   json.RawMessage
	Count int64
	Sums  map[string]float64
	Avgs  map[string]float64
	Mins  map[string]float64
	Maxs  map[string]float64
}

// GroupQuery says how Aggregate groups and what it sums.
type GroupQuery struct {
	Group   string   // a top-level field; empty for one total row
	Numeric []string // up to five number fields to sum, average, min and max
	// Bucket (day, week, month or year) groups a date field by span, keyed by the span's first day.
	Bucket string
	// Elements groups a list field by each element it holds instead of the whole list.
	Elements bool
}

// StatsQuery names the fields a profile reads, by what it shows of each.
type StatsQuery struct {
	Filled  []string // how many records fill each
	Numbers []string // min, max, average and sum
	Dates   []string // first, last and a count per Bucket
	Values  []string // the commonest values and how many differ
	Bucket  string   // day, week, month or year
	// Top and Skip page the values, commonest first: Top of them after the
	// first Skip. Zero Top lists the default few.
	Top, Skip int
	// Elements counts each element of a list field as a value of its own.
	Elements bool
}

// StatsBuckets are the spans a date series counts by.
var StatsBuckets = []string{"day", "week", "month", "year"}

// DateCount is how many records fall in the bucket starting on At (YYYY-MM-DD).
type DateCount struct {
	At    string
	Count int64
}

// FieldStats is one field's profile; only the parts its query asked for are set.
type FieldStats struct {
	Filled             int64
	Min, Max, Avg, Sum *float64
	First, Last        *string
	Distinct           int64
	Top                []RecordGroup // Key and Count only
	Series             []DateCount
}

// RecordStats profiles the selected records field by field.
type RecordStats struct {
	Total  int64
	Fields map[string]*FieldStats
}

// SortField is the data field a Sort of "data.<field>" orders by, "" otherwise.
func (f RecordFilter) SortField() string {
	if len(f.Sort) > 5 && f.Sort[:5] == "data." {
		return f.Sort[5:]
	}
	return ""
}

// KeepFields cuts a record's data to the named top-level fields; no fields,
// or data that is not an object, comes back as it was.
func KeepFields(data json.RawMessage, fields []string) json.RawMessage {
	var all map[string]json.RawMessage
	if len(fields) == 0 || json.Unmarshal(data, &all) != nil || all == nil {
		return data
	}
	kept := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		if v, ok := all[f]; ok {
			kept[f] = v
		}
	}
	out, _ := json.Marshal(kept)
	return out
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

// Referrer is a collection with a reference field pointing at another collection.
type Referrer struct {
	CollectionID string
	Name         string
	Field        string
	// Sub, when set, is the subfield of each item of the list Field that holds the reference.
	Sub    string
	Schema json.RawMessage
}

// RecordLinks are the records of one collection whose reference field names a record.
type RecordLinks struct {
	Referrer
	Total   int64
	Records []Record // the newest few, previews
}

// RecordComment is a note left on a record.
type RecordComment struct {
	ID         string
	RecordID   string
	UserID     string // empty once the author's account is gone
	AuthorName string
	Body       string
	CreatedAt  time.Time
}
