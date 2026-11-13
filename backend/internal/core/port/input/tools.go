package input

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// NewKey is a key just made: its secret shows this once.
type NewKey struct {
	Key    domain.APIKey
	Secret string
}

// NewWebhook is a hook just made: its signing secret shows this once.
type NewWebhook struct {
	Webhook domain.Webhook
	Secret  string
}

// IntegrationService manages a collection's API keys and webhooks (owners) and
// authenticates requests that carry a key.
type IntegrationService interface {
	Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error)
	CreateKey(ctx context.Context, collectionID, name string, canWrite bool) (*NewKey, error)
	DeleteKey(ctx context.Context, collectionID, id string) error
	// AuthenticateKey answers what a dsk_ key allows; any failure is Unauthorized.
	AuthenticateKey(ctx context.Context, secret string) (*domain.KeyGrant, error)

	Webhooks(ctx context.Context, collectionID string) ([]domain.Webhook, error)
	CreateWebhook(ctx context.Context, collectionID, url string, events []string) (*NewWebhook, error)
	DeleteWebhook(ctx context.Context, collectionID, id string) error
	// TestWebhook sends a ping now and records the answer.
	TestWebhook(ctx context.Context, collectionID, id string) (*domain.Webhook, error)
}

// ToolsService runs the trash, field history, saved views and attached files.
type ToolsService interface {
	TrashedRecords(ctx context.Context, collectionID string) ([]domain.TrashedRecord, error)
	RestoreRecords(ctx context.Context, collectionID string, ids []string) (int, error)
	// PurgeRecords empties the given trashed records, or the whole trash with none (owners).
	PurgeRecords(ctx context.Context, collectionID string, ids []string) (int64, error)

	TrashedCollections(ctx context.Context) ([]domain.TrashedCollection, error)
	RestoreCollection(ctx context.Context, id string) error
	PurgeCollection(ctx context.Context, id string) error

	SchemaVersions(ctx context.Context, collectionID string) ([]domain.SchemaVersion, error)

	Views(ctx context.Context, collectionID string) ([]domain.SavedView, error)
	CreateView(ctx context.Context, collectionID, name, query string, shared bool) (*domain.SavedView, error)
	DeleteView(ctx context.Context, collectionID, id string) error

	UploadFile(ctx context.Context, collectionID, name, contentType string, data []byte) (*domain.File, error)
	// File returns a file and its bytes to a viewer of its collection.
	File(ctx context.Context, id string) (*domain.File, []byte, error)
}

// BulkUpdateCommand sets the same top-level values in several records.
type BulkUpdateCommand struct {
	IDs []string
	Set map[string]json.RawMessage
}
