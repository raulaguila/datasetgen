package policy

import (
	"context"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

// JobWebhook delivers one webhook call.
const JobWebhook = "webhook.deliver"

// WebhookJob is the payload of JobWebhook: the hook and the body it receives.
type WebhookJob struct {
	HookID string         `json:"hook_id"`
	Body   map[string]any `json:"body"`
}

// JobAutomation runs one automation, on one record or (daily) on every match.
const JobAutomation = "automation.run"

// AutomationJob is the payload of JobAutomation; RecordID is empty for a daily run.
type AutomationJob struct {
	AutomationID string `json:"automation_id"`
	RecordID     string `json:"record_id,omitempty"`
}

type quietKey struct{}

// WithoutAutomations marks changes an automation makes: they do not start
// automations again, so two automations cannot feed each other forever.
func WithoutAutomations(ctx context.Context) context.Context {
	return context.WithValue(ctx, quietKey{}, true)
}

// Notify queues a delivery of event for each of the collection's hooks that ask
// for it, and a run of each automation it triggers, in the change's
// transaction: no change, no call; no call lost.
func Notify(ctx context.Context, uow output.UnitOfWork, collectionID, event string, data map[string]any) error {
	if id, _ := data["id"].(string); id != "" && (event == domain.TriggerCreated || event == domain.TriggerUpdated) && ctx.Value(quietKey{}) == nil {
		autos, err := uow.Integrations().AutomationsFor(ctx, collectionID, event)
		if err != nil {
			return err
		}
		for _, a := range autos {
			if err := EnqueueJob(ctx, uow, JobAutomation, AutomationJob{AutomationID: a, RecordID: id}); err != nil {
				return err
			}
		}
	}
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
