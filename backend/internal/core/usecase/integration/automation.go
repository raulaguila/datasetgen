package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	default:
		return nil, bad("action", "action must be set_field or notify")
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
		if err := uow.Integrations().DeleteAutomation(ctx, collectionID, id); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "collection.automation_delete", collectionID, map[string]any{"collection_id": collectionID})
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
	}
	return fmt.Errorf("unknown action %q", a.Action)
}
