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
}

// Collection groups records that follow one JSON Schema (app.collection).
type Collection struct {
	ID          string
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema, draft 2020-12
	RecordCount int64           // filled by List only
	Role        MemberRole      // the caller's role; filled by List and Get
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
