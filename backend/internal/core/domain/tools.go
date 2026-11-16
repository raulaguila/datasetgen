package domain

import (
	"encoding/json"
	"time"
)

// APIKeyPrefix starts every API key, so a key reads as one in logs and headers.
const APIKeyPrefix = "dsk_"

// DeletedKey says which key a delete removed.
type DeletedKey struct {
	Name   string `gorm:"column:name"`
	Prefix string `gorm:"column:prefix"`
}

// APIKey lets a program use the records of the collections it reaches as the
// member who made it. Listed for one collection, CanWrite is that collection's.
type APIKey struct {
	ID          string
	Name        string
	Prefix      string // the first characters of the key, to tell keys apart
	CanWrite    bool   // false: read only (viewer), true: editor
	Scopes      []KeyScope
	CreatedBy   string
	CreatorName string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time // the key stops working then; nil never expires
}

// KeyScope is one collection a key reaches, read only or read and write.
type KeyScope struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name,omitempty"`
	CanWrite       bool   `json:"can_write"`
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
	ExpiresAt  *time.Time // nil: the link never ends
}

// KeyGrant is what an API key allows: the collections it reaches (id -> can
// write) and the member it acts as. A share link reaches its one CollectionID.
type KeyGrant struct {
	KeyID        string
	Name         string
	CollectionID string // a share link's collection; empty for an API key
	Scopes       map[string]bool
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
	Failures     int        // failed calls in a row; HookPauseAfter pauses the hook
	PausedAt     *time.Time // set: no calls go out until the owner resumes it
	CreatedBy    string     // read by Webhook only: who hears of a pause
}

// HookPauseAfter failed calls in a row pause a webhook.
const HookPauseAfter = 10

// Automation triggers and actions.
const (
	TriggerCreated = "record.created"
	TriggerUpdated = "record.updated"
	TriggerDaily   = "daily"
	ActionSetField = "set_field" // config {"field": key, "value": any JSON, null clears}
	ActionNotify   = "notify"    // config {"message": text}: a notice to the automation's maker
	// config {"to": "me" or "members", "subject", "message"}: one email per run
	ActionEmail = "email"
	// config {"webhook_id"}: one call of that collection webhook per run, with the records
	ActionWebhook = "webhook"
	// config {"collection_id", "data"}: a record per match; a string value "{{field}}"
	// takes the matching record's field, "{{id}}" its id
	ActionCreateRecord = "create_record"
	AutomationBatch    = 100 // records one daily run acts on
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

// MCPTool is a read-only search an owner publishes on the MCP server: keys that
// reach the collection see it as a tool of its own.
type MCPTool struct {
	ID             string
	CollectionID   string
	CollectionName string // filled for the MCP listing
	Name           string
	Description    string
	Enabled        bool
	Conditions     Where // always applied
	Params         []MCPToolParam
	Fields         []string // the data fields returned; empty returns all
	Sort, Order    string
	Limit          int
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MCPToolParam is an argument of a tool, compared with Field by Op. Type comes
// from the field's schema when the tool is saved.
type MCPToolParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Field       string `json:"field"`
	Op          string `json:"op"`
	Type        string `json:"type"` // string, number, integer or boolean; a list of it for op in
	Required    bool   `json:"required"`
}

// MCPToolOps are the comparisons a parameter may use.
var MCPToolOps = []string{"eq", "neq", "contains", "gt", "gte", "lt", "lte", "in", "has"}

// MCPBuiltinTools are the server's own tool names, which an owner's tool cannot take.
var MCPBuiltinTools = []string{"list_collections", "get_collection", "search_records", "get_record", "related_records",
	"collection_stats", "create_record", "update_record", "delete_records"}

// MCPCall is one tool call an AI assistant made in a collection through MCP.
type MCPCall struct {
	CollectionID   string
	CollectionName string // read by KeyMCPCalls only
	KeyID          string
	KeyName        string
	UserID         string
	UserName       string
	Tool           string
	OK             bool
	Error          string
	CreatedAt      time.Time
}
