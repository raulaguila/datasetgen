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
	// UserID is the caller: a member sees the collections they belong to,
	// a platform admin sees all (as owner). Role follows from it.
	UserID string
	// Details adds MemberCount, FieldCount and LastActivity, which visit
	// every member and record: only the collections screen asks for them.
	Details bool
}

// Collection groups records that follow one JSON Schema (app.collection).
type Collection struct {
	ID          string
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema, draft 2020-12
	Icon        string          // a curated icon slug, "" for initials
	RecordCount int64           // filled by List only
	// Filled by List with Details only.
	MemberCount  int64
	FieldCount   int64
	LastActivity time.Time  // the newest change to the collection or one of its records
	Role         MemberRole // the caller's role; filled by List and Get
	Member       bool       // the caller is a member (a platform admin may not be); filled by List
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CollectionPatch holds the fields of an update; nil keeps the stored value.
type CollectionPatch struct {
	Name        *string
	Description *string
	Icon        *string
	Schema      json.RawMessage
	Version     *int // the version the caller read; a newer row is a conflict
}

// ValidCollectionIcon accepts "" (initials) or a slug of the frontend's icon set
// (frontend/src/lib/collection-icons.ts); both lists change together.
func ValidCollectionIcon(icon string) bool {
	switch icon {
	case "", "anchor", "award", "bell", "book-open", "bookmark", "box", "brain", "bug", "building", "calendar", "camera", "chart-column", "cloud", "code", "coffee", "compass", "cpu", "database", "file-text", "flag", "flame", "gamepad-2", "gem", "globe", "graduation-cap", "hammer", "heart", "image", "key", "languages", "leaf", "lightbulb", "map-pin", "message-square", "mic", "music", "package", "palette", "puzzle", "rocket", "scale", "shield", "shopping-bag", "sparkles", "star", "store", "target", "trophy", "users", "wand-sparkles", "wrench", "zap":
		return true
	}
	return false
}

// CollectionStats is what the collection's overview shows.
type CollectionStats struct {
	Records      int64
	Members      int64
	LastUpdate   *time.Time    // the newest record change; nil without records
	PerDay       []DayCount    // records created per local day, oldest first, days without any left out
	Contributors []Contributor // who created or edited records in the window, most active first
}

type DayCount struct {
	Day   string // YYYY-MM-DD in the caller's time zone
	Count int64
}

type Contributor struct {
	UserID  string
	Name    string
	Created int64
	Edited  int64
}

// MemberRole is what a person may do in one collection (app.collection_member).
type MemberRole string

const (
	RoleViewer MemberRole = "viewer" // reads the collection and its records
	RoleEditor MemberRole = "editor" // also writes records
	RoleOwner  MemberRole = "owner"  // also changes fields, settings and members
)

var memberRank = map[MemberRole]int{RoleViewer: 1, RoleEditor: 2, RoleOwner: 3}

// Valid reports whether r is one of the three roles.
func (r MemberRole) Valid() bool { return memberRank[r] > 0 }

// Allows reports whether r covers need: owner covers editor, editor covers viewer.
func (r MemberRole) Allows(need MemberRole) bool {
	return memberRank[r] >= memberRank[need] && memberRank[need] > 0
}

// CollectionMember is one person in a collection, with what the members screen shows.
type CollectionMember struct {
	UserID    string
	Name      string
	Email     string
	Status    string // UserStatus*
	Role      MemberRole
	CreatedAt time.Time
}

// QualityRun is the latest background check of a collection's records against
// its schema (app.collection_quality).
type QualityRun struct {
	SchemaVersion int // the collection version checked against
	Status        string
	Checked       int
	InvalidCount  int
	Truncated     bool            // stopped at the cap before the end of the collection
	Invalid       json.RawMessage // the first invalid records with their issues
	InvalidIDs    []string        // every invalid record found
	Error         string
	StartedAt     time.Time
	FinishedAt    *time.Time
}

const (
	QualityRunning = "running"
	QualityDone    = "done"
	QualityFailed  = "failed"
)
