package domain

import (
	"encoding/json"
	"time"
)

// APIKeyPrefix starts every API key, so a key reads as one in logs and headers.
const APIKeyPrefix = "dsk_"

// APIKey lets a program use one collection's records as the member who made it.
type APIKey struct {
	ID           string
	CollectionID string
	Name         string
	Prefix       string // the first characters of the key, to tell keys apart
	CanWrite     bool   // false: read only (viewer), true: editor
	CreatedBy    string
	CreatorName  string
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// SharePrefix starts every share link token.
const SharePrefix = "dss_"

// Share link kinds.
const (
	ShareForm = "form" // anyone with the link adds records
	ShareView = "view" // anyone with the link reads the records
)

// Share is a public link to a collection: a form or a read-only list.
type Share struct {
	ID         string
	Kind       string
	Token      string // sealed at rest; plain only inside the use case
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// KeyGrant is what an API key allows: the collection, the role cap and the member it acts as.
type KeyGrant struct {
	KeyID        string
	Name         string
	CollectionID string
	CanWrite     bool
	UserID       string
	Share        string // empty for an API key
}

// Webhook events.
const (
	EventRecordCreated   = "record.created"
	EventRecordUpdated   = "record.updated"
	EventRecordDeleted   = "record.deleted"
	EventRecordsImported = "records.imported"
)

// WebhookEvents lists the events a webhook may ask for.
var WebhookEvents = []string{EventRecordCreated, EventRecordUpdated, EventRecordDeleted, EventRecordsImported}

// Webhook POSTs a collection's record changes to a URL.
type Webhook struct {
	ID           string
	CollectionID string
	URL          string
	Secret       string // sealed at rest; plain only inside the use case
	Events       []string
	CreatedAt    time.Time
	LastStatus   *int
	LastError    string
	LastAt       *time.Time
}

// Automation triggers and actions.
const (
	TriggerCreated  = "record.created"
	TriggerUpdated  = "record.updated"
	TriggerDaily    = "daily"
	ActionSetField  = "set_field" // config {"field": key, "value": any JSON, null clears}
	ActionNotify    = "notify"    // config {"message": text}: a notice to the automation's maker
	AutomationBatch = 100         // records one daily run acts on
)

// Automation acts on the records that pass Conditions when Trigger happens.
type Automation struct {
	ID           string
	CollectionID string
	Name         string
	Enabled      bool
	Trigger      string
	Hour         *int // daily only: the hour (server time) it runs at
	Conditions   Where
	Action       string
	Config       json.RawMessage
	CreatedBy    string
	CreatedAt    time.Time
	LastRunAt    *time.Time
	LastError    string
	Runs         int
}

// WebhookDelivery is one call a webhook made and the answer it got.
type WebhookDelivery struct {
	ID        string
	WebhookID string
	Event     string
	Body      json.RawMessage
	Status    *int
	Error     string
	CreatedAt time.Time
}

// TrashedRecord is a deleted record waiting in the trash.
type TrashedRecord struct {
	ID            string
	CollectionID  string
	Data          json.RawMessage
	DeletedAt     time.Time
	DeletedByName string
}

// TrashedCollection is a deleted collection its owner may still restore.
type TrashedCollection struct {
	ID          string
	Name        string
	Icon        string
	RecordCount int64
	DeletedAt   time.Time
}

// TrashRetention is how long the trash keeps things (app.fn_purge_trash).
const TrashRetention = 30 * 24 * time.Hour

// SchemaVersion is a schema a collection had before a change replaced it.
type SchemaVersion struct {
	Version     int
	Schema      json.RawMessage
	SavedAt     time.Time
	SavedByName string
}

// SavedView is a named record list query.
type SavedView struct {
	ID           string
	CollectionID string
	UserID       string
	Name         string
	Query        string
	Shared       bool
	Mine         bool
	CreatedAt    time.Time
}

// FileMaxBytes caps an attached file: the request body limit is sized for it,
// and it stays under the frontend proxy's 10 MB body limit.
const FileMaxBytes = 8 * 1024 * 1024

// File is a file attached to a collection's records.
type File struct {
	ID           string
	CollectionID string
	Name         string
	ContentType  string
	Size         int
	CreatedAt    time.Time
}
