package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	maxAutomations  = 20
	maxMessage      = 500
	shownDeliveries = 50
)

func (s *Service) hookOf(ctx context.Context, collectionID, hookID string) (*domain.Webhook, error) {
	if err := validate.ID(hookID); err != nil {
		return nil, err
	}
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	h, err := s.Store.Integrations().Webhook(ctx, hookID)
	if err != nil || h.CollectionID != collectionID {
		return nil, apperror.New(apperror.CodeNotFound, "webhook not found")
	}
	return h, nil
}

func (s *Service) Deliveries(ctx context.Context, collectionID, hookID string) ([]domain.WebhookDelivery, error) {
	if _, err := s.hookOf(ctx, collectionID, hookID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().Deliveries(ctx, hookID, shownDeliveries)
}

func (s *Service) Redeliver(ctx context.Context, collectionID, hookID, deliveryID string) (*domain.WebhookDelivery, error) {
	h, err := s.hookOf(ctx, collectionID, hookID)
	if err != nil {
		return nil, err
	}
	if err := validate.ID(deliveryID); err != nil {
		return nil, err
	}
	d, err := s.Store.Integrations().Delivery(ctx, deliveryID)
	if err != nil || d.WebhookID != hookID {
		return nil, apperror.New(apperror.CodeNotFound, "delivery not found")
	}
	var body map[string]any
	if err := json.Unmarshal(d.Body, &body); err != nil {
		return nil, err
	}
	_ = s.deliver(ctx, h, body) // the answer is logged as a new delivery
	list, err := s.Store.Integrations().Deliveries(ctx, hookID, 1)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (s *Service) Automations(ctx context.Context, collectionID string) ([]domain.Automation, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	return s.Store.Integrations().Automations(ctx, collectionID)
}

// checkAutomation validates the settings; the conditions are tried on the
// collection now, so a bad rule fails here and not on every run.
func (s *Service) checkAutomation(ctx context.Context, collectionID string, in input.AutomationInput) (*domain.Automation, error) {
	bad := func(field, msg string) error { return apperror.New(apperror.CodeInvalidInput, msg).WithField(field) }
	a := &domain.Automation{CollectionID: collectionID, Name: strings.TrimSpace(in.Name), Enabled: in.Enabled, Trigger: in.Trigger, Conditions: in.Conditions, Action: in.Action, Config: in.Config}
	if n := utf8.RuneCountInString(a.Name); n < 1 || n > maxName {
		return nil, bad("name", "name must be 1-80 characters")
	}
	switch a.Trigger {
	case domain.TriggerCreated, domain.TriggerUpdated:
	case domain.TriggerDaily:
		if in.Hour == nil || *in.Hour < 0 || *in.Hour > 23 {
			return nil, bad("hour", "a daily automation needs an hour from 0 to 23")
		}
		a.Hour = in.Hour
	default:
		return nil, bad("trigger", "trigger must be record.created, record.updated or daily")
	}
	if a.Conditions.Rules == nil {
		a.Conditions.Rules = []domain.Condition{}
	}
	switch a.Action {
	case domain.ActionSetField:
		var c struct {
			Field string          `json:"field"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(a.Config, &c) != nil || c.Field == "" || len(c.Field) > 200 || len(c.Value) == 0 || !json.Valid(c.Value) {
			return nil, bad("config", `set_field needs {"field": name, "value": JSON}`)
		}
		a.Config, _ = json.Marshal(c)
	case domain.ActionNotify:
		var c struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(a.Config, &c) != nil || strings.TrimSpace(c.Message) == "" || utf8.RuneCountInString(c.Message) > maxMessage {
			return nil, bad("config", `notify needs {"message": 1-500 characters}`)
		}
		c.Message = strings.TrimSpace(c.Message)
		a.Config, _ = json.Marshal(c)
	case domain.ActionEmail:
		var c emailConfig
		if json.Unmarshal(a.Config, &c) != nil || (c.To != emailToMe && c.To != emailToMembers) {
			return nil, bad("config", `email needs {"to": "me" or "members", "subject", "message"}`)
		}
		c.Subject, c.Message = strings.TrimSpace(c.Subject), strings.TrimSpace(c.Message)
		if n := utf8.RuneCountInString(c.Subject); n < 1 || n > maxName || strings.ContainsAny(c.Subject, "\r\n") {
			return nil, bad("config", "the email subject must be 1-80 characters on one line")
		}
		if c.Message == "" || utf8.RuneCountInString(c.Message) > maxMessage {
			return nil, bad("config", "the email message must be 1-500 characters")
		}
		a.Config, _ = json.Marshal(c)
	case domain.ActionWebhook:
		var c webhookConfig
		if json.Unmarshal(a.Config, &c) != nil || validate.ID(c.WebhookID) != nil {
			return nil, bad("config", `webhook needs {"webhook_id": one of the collection's webhooks}`)
		}
		if h, err := s.Store.Integrations().Webhook(ctx, c.WebhookID); err != nil || h.CollectionID != collectionID {
			return nil, bad("config", "the webhook is not one of this collection's")
		}
		a.Config, _ = json.Marshal(c)
	case domain.ActionCreateRecord:
		var c createConfig
		if json.Unmarshal(a.Config, &c) != nil || len(c.Data) == 0 {
			return nil, bad("config", `create_record needs {"collection_id", "data": a JSON object}`)
		}
		// The maker must be able to write there now; each run checks again.
		if _, err := policy.CollectionRole(ctx, s.Store.Collections(), c.CollectionID, domain.RoleEditor); err != nil {
			return nil, err
		}
		a.Config, _ = json.Marshal(c)
	default:
		return nil, bad("action", "action must be set_field, notify, email, webhook or create_record")
	}
	if _, _, err := s.Records.List(ctx, domain.RecordFilter{CollectionID: collectionID, Page: domain.NewPage(1, 1), Where: &a.Conditions}); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) CreateAutomation(ctx context.Context, collectionID string, in input.AutomationInput) (*domain.Automation, error) {
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	if _, byKey := reqctx.Key(ctx); byKey {
		return nil, apperror.New(apperror.CodeForbidden, "an API key cannot make automations")
	}
	a, err := s.checkAutomation(ctx, collectionID, in)
	if err != nil {
		return nil, err
	}
	a.CreatedBy = reqctx.Actor(ctx)
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		existing, err := uow.Integrations().Automations(ctx, collectionID)
		if err != nil {
			return err
		}
		if len(existing) >= maxAutomations {
			return apperror.New(apperror.CodeInvalidInput, "a collection holds up to 20 automations; delete one first")
		}
		if err := uow.Integrations().CreateAutomation(ctx, a); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.automation_create", collectionID, map[string]any{"collection_id": collectionID, "automation": a.Name})
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) UpdateAutomation(ctx context.Context, collectionID, id string, in input.AutomationInput) (*domain.Automation, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	a, err := s.checkAutomation(ctx, collectionID, in)
	if err != nil {
		return nil, err
	}
	a.ID = id
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Integrations().UpdateAutomation(ctx, a); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.automation_update", collectionID, map[string]any{"collection_id": collectionID, "automation": a.Name})
	})
	if err != nil {
		return nil, err
	}
	return s.Store.Integrations().Automation(ctx, id)
}

func (s *Service) DeleteAutomation(ctx context.Context, collectionID, id string) error {
	if err := validate.ID(id); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := s.owner(ctx, uow, collectionID); err != nil {
			return err
		}
		name, err := uow.Integrations().DeleteAutomation(ctx, collectionID, id)
		if err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.automation_delete", collectionID, map[string]any{"collection_id": collectionID, "automation": name})
	})
}

func (s *Service) RunAutomation(ctx context.Context, collectionID, id string) (*domain.Automation, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	if err := s.owner(ctx, s.Store, collectionID); err != nil {
		return nil, err
	}
	a, err := s.Store.Integrations().Automation(ctx, id)
	if err != nil || a.CollectionID != collectionID {
		return nil, apperror.New(apperror.CodeNotFound, "automation not found")
	}
	if err := s.run(ctx, a, ""); err != nil && !isAppError(err) {
		return nil, err
	}
	return s.Store.Integrations().Automation(ctx, id)
}

func (s *Service) Schedule(ctx context.Context) error {
	ids, err := s.Store.Integrations().DueDaily(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
			return policy.EnqueueJob(ctx, uow, policy.JobAutomation, policy.AutomationJob{AutomationID: id})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runJob(ctx context.Context, payload []byte) error {
	var job policy.AutomationJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return policy.PermanentJobError(err)
	}
	a, err := s.Store.Integrations().Automation(ctx, job.AutomationID)
	if apperror.IsCode(err, apperror.CodeNotFound) {
		return nil // deleted since
	}
	if err != nil {
		return err
	}
	if !a.Enabled {
		return nil
	}
	err = s.run(ctx, a, job.RecordID)
	if isAppError(err) {
		return policy.PermanentJobError(err) // a rule or a value the records refuse: retrying cannot help
	}
	return err
}

func isAppError(err error) bool {
	var ae *apperror.Error
	return err != nil && errors.As(err, &ae) && ae.Code != apperror.CodeInternal
}

// run acts as the automation's maker on the matching records: the one that
// changed, or (recordID empty) up to AutomationBatch of the collection.
// Its own changes start no automations (policy.WithoutAutomations).
func (s *Service) run(ctx context.Context, a *domain.Automation, recordID string) error {
	ctx = policy.WithoutAutomations(reqctx.WithActor(ctx, a.CreatedBy))
	filter := domain.RecordFilter{CollectionID: a.CollectionID, Page: domain.NewPage(1, domain.AutomationBatch), Where: &a.Conditions, Sort: "updated_at", Order: "desc"}
	if recordID != "" {
		filter.IDs, filter.Page = []string{recordID}, domain.NewPage(1, 1)
	}
	items, _, err := s.Records.List(ctx, filter)
	if err == nil && len(items) == 0 {
		if recordID == "" {
			return s.Store.Integrations().AutomationRan(ctx, a.ID, "")
		}
		return nil // the record does not pass the conditions
	}
	if err == nil {
		err = s.act(ctx, a, items)
	}
	errText := ""
	if err != nil {
		errText = validate.Text(err.Error(), 300)
	}
	if markErr := s.Store.Integrations().AutomationRan(ctx, a.ID, errText); markErr != nil {
		return markErr
	}
	return err
}

func (s *Service) act(ctx context.Context, a *domain.Automation, items []domain.Record) error {
	ids := make([]string, 0, len(items))
	for _, r := range items {
		ids = append(ids, r.ID)
	}
	switch a.Action {
	case domain.ActionSetField:
		var c struct {
			Field string          `json:"field"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(a.Config, &c); err != nil {
			return err
		}
		_, err := s.Records.BulkUpdate(ctx, input.BulkUpdateCommand{IDs: ids, Set: map[string]json.RawMessage{c.Field: c.Value}})
		return err
	case domain.ActionNotify:
		var c struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(a.Config, &c); err != nil {
			return err
		}
		params := map[string]string{"collection_id": a.CollectionID, "count": fmt.Sprint(len(ids))}
		if len(ids) == 1 {
			params["record_id"] = ids[0]
		}
		return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
			return uow.Notifications().Create(ctx, &domain.Notification{UserID: a.CreatedBy, Level: domain.NotificationInfo, Params: params, Title: a.Name, Message: c.Message})
		})
	case domain.ActionEmail:
		return s.email(ctx, a, ids)
	case domain.ActionWebhook:
		var c webhookConfig
		if err := json.Unmarshal(a.Config, &c); err != nil {
			return err
		}
		records := make([]map[string]any, len(items))
		for i, r := range items {
			records[i] = map[string]any{"id": r.ID, "data": r.Data, "version": r.Version, "updated_at": r.UpdatedAt}
		}
		body := map[string]any{"event": "automation.run", "collection_id": a.CollectionID, "automation": a.Name,
			"occurred_at": time.Now().UTC().Format(time.RFC3339), "records": records}
		// Queued, so the call gets the hook's retries and lands in its delivery log.
		return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
			return policy.EnqueueJob(ctx, uow, policy.JobWebhook, policy.WebhookJob{HookID: c.WebhookID, Body: body})
		})
	case domain.ActionCreateRecord:
		var c createConfig
		if err := json.Unmarshal(a.Config, &c); err != nil {
			return err
		}
		for _, r := range items {
			data, err := fillTemplate(c.Data, r)
			if err != nil {
				return err
			}
			if _, err := s.Records.Create(ctx, input.CreateRecordCommand{CollectionID: c.CollectionID, Data: data}); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown action %q", a.Action)
}

const (
	emailToMe      = "me"
	emailToMembers = "members"
)

type emailConfig struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type webhookConfig struct {
	WebhookID string `json:"webhook_id"`
}

type createConfig struct {
	CollectionID string                     `json:"collection_id"`
	Data         map[string]json.RawMessage `json:"data"`
}

// email queues one message per recipient for the run: the maker, or every
// active member. The mailer adds the link to the collection (or the record).
func (s *Service) email(ctx context.Context, a *domain.Automation, ids []string) error {
	var c emailConfig
	if err := json.Unmarshal(a.Config, &c); err != nil {
		return err
	}
	members, err := s.Store.Collections().Members(ctx, a.CollectionID)
	if err != nil {
		return err
	}
	path := "/collections/" + a.CollectionID
	if len(ids) == 1 {
		path = "/records/" + ids[0]
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		for _, m := range members {
			if m.Status != domain.UserStatusActive || (c.To == emailToMe && m.UserID != a.CreatedBy) {
				continue
			}
			payload, err := s.Mail.Encode(output.EmailToken{Email: m.Email, Token: c.Message, Subject: c.Subject, Path: path})
			if err != nil {
				return err
			}
			if err := uow.Outbox().Enqueue(ctx, domain.OutboxPurposeAutomation, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

// fillTemplate builds a record from an automation's data: a string value that
// is exactly "{{field}}" takes that top-level field of the matching record
// (left out when the record has none), "{{id}}" the record's id.
func fillTemplate(tpl map[string]json.RawMessage, r domain.Record) (json.RawMessage, error) {
	var src map[string]json.RawMessage
	if err := json.Unmarshal(r.Data, &src); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(tpl))
	for k, v := range tpl {
		var ref string
		if json.Unmarshal(v, &ref) != nil || !strings.HasPrefix(ref, "{{") || !strings.HasSuffix(ref, "}}") {
			out[k] = v
			continue
		}
		name := strings.TrimSpace(ref[2 : len(ref)-2])
		if name == "id" {
			out[k], _ = json.Marshal(r.ID)
		} else if val, ok := src[name]; ok {
			out[k] = val
		}
	}
	return json.Marshal(out)
}
