package output

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// IntegrationRepository keeps a collection's API keys and webhooks.
type IntegrationRepository interface {
	Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error)
	CreateKey(ctx context.Context, k *domain.APIKey, hash []byte) error
	DeleteKey(ctx context.Context, collectionID, id string) error
	// Shares lists the collection's share links, tokens sealed.
	Shares(ctx context.Context, collectionID string) ([]domain.Share, error)
	CreateShare(ctx context.Context, collectionID, kind, sealed string, hash []byte, by string) (*domain.Share, error)
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
	// LogDelivery keeps a call and its answer, dropping all but the newest 50 of the hook.
	LogDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	Deliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error)
	Delivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)

	Automations(ctx context.Context, collectionID string) ([]domain.Automation, error)
	Automation(ctx context.Context, id string) (*domain.Automation, error)
	CreateAutomation(ctx context.Context, a *domain.Automation) error
	// UpdateAutomation saves a's settings (not its run counters); NotFound when it is not the collection's.
	UpdateAutomation(ctx context.Context, a *domain.Automation) error
	DeleteAutomation(ctx context.Context, collectionID, id string) error
	// AutomationsFor lists the enabled automations of a collection with this trigger.
	AutomationsFor(ctx context.Context, collectionID, trigger string) ([]string, error)
	AutomationRan(ctx context.Context, id, errText string) error
	// DueDaily claims the enabled daily automations whose hour has come and that
	// have not run today, stamping them run, and returns their ids.
	DueDaily(ctx context.Context) ([]string, error)
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
	// DropOrphanFiles deletes up to limit file rows made before the time that
	// no record, trashed record or record version names; their ids go to the
	// gone list.
	DropOrphanFiles(ctx context.Context, before time.Time, limit int) (int64, error)
	// GoneFiles lists up to limit deleted files whose bytes are still stored.
	GoneFiles(ctx context.Context, limit int) ([]string, error)
	ForgetGoneFiles(ctx context.Context, ids []string) error
}

// WebhookSender POSTs one delivery; an error covers a refused address, a
// failed connection and an answer outside 2xx (status then holds it).
type WebhookSender interface {
	Send(ctx context.Context, url string, body []byte, headers map[string]string) (status int, err error)
}
