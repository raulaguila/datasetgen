package output

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// IntegrationRepository keeps a collection's API keys and webhooks.
type IntegrationRepository interface {
	Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error)
	CreateKey(ctx context.Context, k *domain.APIKey, hash []byte) error
	DeleteKey(ctx context.Context, collectionID, id string) error
	// Grant finds a key by its hash; its member must still be active. It marks
	// the key used (at most once a minute). NotFound when nothing matches.
	Grant(ctx context.Context, hash []byte) (*domain.KeyGrant, error)

	Webhooks(ctx context.Context, collectionID string) ([]domain.Webhook, error)
	// WebhooksFor lists the hooks of a collection that ask for event, with sealed secrets.
	WebhooksFor(ctx context.Context, collectionID, event string) ([]domain.Webhook, error)
	Webhook(ctx context.Context, id string) (*domain.Webhook, error)
	CreateWebhook(ctx context.Context, w *domain.Webhook, createdBy string) error
	DeleteWebhook(ctx context.Context, collectionID, id string) error
	WebhookResult(ctx context.Context, id string, status int, errText string) error
}

// ToolsRepository keeps the trash, schema history, saved views and files.
type ToolsRepository interface {
	// TrashRecords moves records to the trash; all or none (NotFound).
	TrashRecords(ctx context.Context, ids []string, by string) error
	TrashedRecords(ctx context.Context, collectionID string, limit int) ([]domain.TrashedRecord, error)
	// RestoreRecords moves trashed records of the collection back; returns their data by id.
	RestoreRecords(ctx context.Context, collectionID string, ids []string) (map[string]json.RawMessage, error)
	PurgeRecords(ctx context.Context, collectionID string, ids []string) (int64, error)

	TrashCollections(ctx context.Context, ids []string, by string) (map[string]string, error)
	// TrashedCollections lists the deleted collections the user owns (all for a platform admin).
	TrashedCollections(ctx context.Context, userID string) ([]domain.TrashedCollection, error)
	// TrashedCollectionOwner answers whether the user owns the deleted collection (NotFound otherwise).
	TrashedCollectionOwner(ctx context.Context, id, userID string) error
	RestoreCollection(ctx context.Context, id string) error
	PurgeCollection(ctx context.Context, id string) error

	SaveSchemaVersion(ctx context.Context, collectionID string, version int, schema json.RawMessage, by string) error
	SchemaVersions(ctx context.Context, collectionID string, limit int) ([]domain.SchemaVersion, error)

	Views(ctx context.Context, collectionID, userID string) ([]domain.SavedView, error)
	CreateView(ctx context.Context, v *domain.SavedView) error
	// DeleteView removes a view the user made (NotFound otherwise).
	DeleteView(ctx context.Context, collectionID, id, userID string) error

	CreateFile(ctx context.Context, f *domain.File, by string) error
	File(ctx context.Context, id string) (*domain.File, error)
	// RecordsExist counts which of ids are live records of the collection.
	RecordsExist(ctx context.Context, collectionID string, ids []string) (map[string]bool, error)
	// FilesExist counts which of ids are files of the collection.
	FilesExist(ctx context.Context, collectionID string, ids []string) (map[string]bool, error)
}

// WebhookSender POSTs one delivery; an error covers a refused address, a
// failed connection and an answer outside 2xx (status then holds it).
type WebhookSender interface {
	Send(ctx context.Context, url string, body []byte, headers map[string]string) (status int, err error)
}
