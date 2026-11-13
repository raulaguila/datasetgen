package policy

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

// JobWebhook delivers one webhook call.
const JobWebhook = "webhook.deliver"

// WebhookJob is the payload of JobWebhook: the hook and the body it receives.
type WebhookJob struct {
	HookID string         `json:"hook_id"`
	Body   map[string]any `json:"body"`
}

// Notify queues a delivery of event for each of the collection's hooks that ask
// for it, in the change's transaction: no change, no call; no call lost.
func Notify(ctx context.Context, uow output.UnitOfWork, collectionID, event string, data map[string]any) error {
	hooks, err := uow.Integrations().WebhooksFor(ctx, collectionID, event)
	if err != nil || len(hooks) == 0 {
		return err
	}
	body := map[string]any{"event": event, "collection_id": collectionID, "occurred_at": time.Now().UTC().Format(time.RFC3339), "data": data}
	for _, h := range hooks {
		if err := EnqueueJob(ctx, uow, JobWebhook, WebhookJob{HookID: h.ID, Body: body}); err != nil {
			return err
		}
	}
	return nil
}
