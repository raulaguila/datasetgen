package integration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
)

type deliveryRow struct {
	ID        uuid.UUID `gorm:"column:id"`
	WebhookID uuid.UUID `gorm:"column:webhook_id"`
	Event     string    `gorm:"column:event"`
	Body      []byte    `gorm:"column:body"`
	Status    *int      `gorm:"column:status"`
	Error     string    `gorm:"column:error"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (m deliveryRow) toDomain() domain.WebhookDelivery {
	return domain.WebhookDelivery{ID: m.ID.String(), WebhookID: m.WebhookID.String(), Event: m.Event, Body: m.Body, Status: m.Status, Error: m.Error, CreatedAt: m.CreatedAt}
}

const keptDeliveries = 50

func (r *Repository) LogDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	var m deliveryRow
	err := r.db.WithContext(ctx).Raw(`
		WITH added AS (
			INSERT INTO app.webhook_delivery (webhook_id, event, body, status, error) VALUES (?, ?, ?, ?, ?)
			RETURNING id, created_at
		), pruned AS (
			DELETE FROM app.webhook_delivery WHERE id IN (
				SELECT id FROM app.webhook_delivery WHERE webhook_id = ? ORDER BY created_at DESC OFFSET ?)
		)
		SELECT id, created_at FROM added`,
		d.WebhookID, d.Event, string(d.Body), d.Status, d.Error, d.WebhookID, keptDeliveries-1).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "webhook delivery")
	}
	d.ID, d.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

// Deliveries lists the hook's calls without their bodies, newest first.
func (r *Repository) Deliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error) {
	var rows []deliveryRow
	err := r.db.WithContext(ctx).Table("app.webhook_delivery").Select("id, webhook_id, event, status, error, created_at").
		Where("webhook_id = ?", webhookID).Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "webhook delivery")
	}
	out := make([]domain.WebhookDelivery, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) Delivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	var m deliveryRow
	err := r.db.WithContext(ctx).Table("app.webhook_delivery").Select("id, webhook_id, event, body").Where("id = ?", id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "webhook delivery")
	}
	d := m.toDomain()
	return &d, nil
}

type automationRow struct {
	ID           uuid.UUID  `gorm:"column:id"`
	CollectionID uuid.UUID  `gorm:"column:collection_id"`
	Name         string     `gorm:"column:name"`
	Enabled      bool       `gorm:"column:enabled"`
	Trigger      string     `gorm:"column:trigger"`
	Hour         *int       `gorm:"column:hour"`
	Conditions   []byte     `gorm:"column:conditions"`
	Action       string     `gorm:"column:action"`
	Config       []byte     `gorm:"column:config"`
	CreatedBy    uuid.UUID  `gorm:"column:created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	LastRunAt    *time.Time `gorm:"column:last_run_at"`
	LastError    string     `gorm:"column:last_error"`
	Runs         int        `gorm:"column:runs"`
}

func (m automationRow) toDomain() domain.Automation {
	a := domain.Automation{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), Name: m.Name, Enabled: m.Enabled, Trigger: m.Trigger, Hour: m.Hour,
		Action: m.Action, Config: m.Config, CreatedBy: m.CreatedBy.String(), CreatedAt: m.CreatedAt, LastRunAt: m.LastRunAt, LastError: m.LastError, Runs: m.Runs,
	}
	_ = json.Unmarshal(m.Conditions, &a.Conditions)
	return a
}

const automationColumns = "id, collection_id, name, enabled, trigger, hour, conditions, action, config, created_by, created_at, last_run_at, last_error, runs"

// Automations lists at most 50: the use case caps a collection at 20.
func (r *Repository) Automations(ctx context.Context, collectionID string) ([]domain.Automation, error) {
	var rows []automationRow
	err := r.db.WithContext(ctx).Table("app.automation").Select(automationColumns).
		Where("collection_id = ?", collectionID).Order("created_at").Limit(50).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "automation")
	}
	out := make([]domain.Automation, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) Automation(ctx context.Context, id string) (*domain.Automation, error) {
	var m automationRow
	if err := r.db.WithContext(ctx).Table("app.automation").Select(automationColumns).Where("id = ?", id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "automation")
	}
	a := m.toDomain()
	return &a, nil
}

func (r *Repository) CreateAutomation(ctx context.Context, a *domain.Automation) error {
	cond, err := json.Marshal(a.Conditions)
	if err != nil {
		return err
	}
	var m automationRow
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO app.automation (collection_id, name, enabled, trigger, hour, conditions, action, config, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id, created_at`,
		a.CollectionID, a.Name, a.Enabled, a.Trigger, a.Hour, string(cond), a.Action, string(a.Config), a.CreatedBy).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "automation")
	}
	a.ID, a.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) UpdateAutomation(ctx context.Context, a *domain.Automation) error {
	cond, err := json.Marshal(a.Conditions)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Exec(`
		UPDATE app.automation SET name = ?, enabled = ?, trigger = ?, hour = ?, conditions = ?, action = ?, config = ?
		WHERE id = ? AND collection_id = ?`,
		a.Name, a.Enabled, a.Trigger, a.Hour, string(cond), a.Action, string(a.Config), a.ID, a.CollectionID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "automation")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "automation")
	}
	return nil
}

func (r *Repository) DeleteAutomation(ctx context.Context, collectionID, id string) (string, error) {
	var names []string
	err := r.db.WithContext(ctx).Raw(`DELETE FROM app.automation WHERE id = ? AND collection_id = ? RETURNING name`, id, collectionID).Find(&names).Error
	if err != nil {
		return "", shared.WrapError(err, "automation")
	}
	if len(names) == 0 {
		return "", shared.WrapError(gorm.ErrRecordNotFound, "automation")
	}
	return names[0], nil
}

// AutomationsFor reads at most 20 ids, the cap per collection.
func (r *Repository) AutomationsFor(ctx context.Context, collectionID, trigger string) ([]string, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Table("app.automation").Select("id").
		Where("collection_id = ? AND trigger = ? AND enabled", collectionID, trigger).Limit(20).Find(&ids).Error
	if err != nil {
		return nil, shared.WrapError(err, "automation")
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}

func (r *Repository) AutomationRan(ctx context.Context, id, errText string) error {
	err := r.db.WithContext(ctx).Exec(`UPDATE app.automation SET last_run_at = now(), last_error = ?, runs = runs + 1 WHERE id = ?`, errText, id).Error
	return shared.WrapError(err, "automation")
}

// shortcut: claims at most 100 daily automations per tick; the next tick takes the rest.
func (r *Repository) DueDaily(ctx context.Context) ([]string, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.automation SET last_run_at = now()
		WHERE id IN (
			SELECT id FROM app.automation
			WHERE trigger = 'daily' AND enabled AND hour <= extract(hour FROM now())
			  AND (last_run_at IS NULL OR last_run_at < date_trunc('day', now()))
			LIMIT 100 FOR UPDATE SKIP LOCKED)
		RETURNING id`).Find(&ids).Error
	if err != nil {
		return nil, shared.WrapError(err, "automation")
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}
