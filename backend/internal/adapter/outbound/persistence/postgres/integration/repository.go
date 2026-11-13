package integration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.IntegrationRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type keyRow struct {
	ID           uuid.UUID  `gorm:"column:id"`
	CollectionID uuid.UUID  `gorm:"column:collection_id"`
	Name         string     `gorm:"column:name"`
	Prefix       string     `gorm:"column:prefix"`
	CanWrite     bool       `gorm:"column:can_write"`
	CreatedBy    uuid.UUID  `gorm:"column:created_by"`
	CreatorName  string     `gorm:"column:creator_name"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	LastUsedAt   *time.Time `gorm:"column:last_used_at"`
}

func (m keyRow) toDomain() domain.APIKey {
	return domain.APIKey{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), Name: m.Name, Prefix: m.Prefix, CanWrite: m.CanWrite,
		CreatedBy: m.CreatedBy.String(), CreatorName: m.CreatorName, CreatedAt: m.CreatedAt, LastUsedAt: m.LastUsedAt,
	}
}

// shortcut: no limit on keys or hooks per collection; the screen lists them all.
func (r *Repository) Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error) {
	var rows []keyRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT k.id, k.collection_id, k.name, k.prefix, k.can_write, k.created_by, u.name AS creator_name, k.created_at, k.last_used_at
		FROM app.api_key k JOIN usr.usr_user u ON u.id = k.created_by
		WHERE k.collection_id = ? AND k.share IS NULL ORDER BY k.created_at DESC`, collectionID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	out := make([]domain.APIKey, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) CreateKey(ctx context.Context, k *domain.APIKey, hash []byte) error {
	var m keyRow
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.api_key (collection_id, name, prefix, key_hash, can_write, created_by)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id, created_at`,
		k.CollectionID, k.Name, k.Prefix, hash, k.CanWrite, k.CreatedBy).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "api key")
	}
	k.ID, k.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) DeleteKey(ctx context.Context, collectionID, id string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM app.api_key WHERE id = ? AND collection_id = ?`, id, collectionID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "api key")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "api key")
	}
	return nil
}

func (r *Repository) Grant(ctx context.Context, hash []byte) (*domain.KeyGrant, error) {
	var m struct {
		ID           uuid.UUID `gorm:"column:id"`
		Name         string    `gorm:"column:name"`
		CollectionID uuid.UUID `gorm:"column:collection_id"`
		CanWrite     bool      `gorm:"column:can_write"`
		CreatedBy    uuid.UUID `gorm:"column:created_by"`
		Share        *string   `gorm:"column:share"`
	}
	// One statement: find the key of an active member and stamp its use, at most once a minute.
	err := r.db.WithContext(ctx).Raw(`
		WITH k AS (
			SELECT k.id, k.name, k.collection_id, k.can_write, k.created_by, k.share, k.last_used_at
			FROM app.api_key k JOIN usr.usr_user u ON u.id = k.created_by AND u.status = 'active'
			WHERE k.key_hash = ?
		), used AS (
			UPDATE app.api_key a SET last_used_at = now() FROM k
			WHERE a.id = k.id AND (k.last_used_at IS NULL OR k.last_used_at < now() - INTERVAL '1 minute')
		)
		SELECT id, name, collection_id, can_write, created_by, share FROM k`, hash).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	g := &domain.KeyGrant{KeyID: m.ID.String(), Name: m.Name, CollectionID: m.CollectionID.String(), CanWrite: m.CanWrite, UserID: m.CreatedBy.String()}
	if m.Share != nil {
		g.Share = *m.Share
	}
	return g, nil
}

type shareRow struct {
	ID         uuid.UUID  `gorm:"column:id"`
	Share      string     `gorm:"column:share"`
	Sealed     string     `gorm:"column:sealed"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"`
}

func (m shareRow) toDomain() domain.Share {
	return domain.Share{ID: m.ID.String(), Kind: m.Share, Token: m.Sealed, CreatedAt: m.CreatedAt, LastUsedAt: m.LastUsedAt}
}

// Shares lists at most maxShares links: CreateShare caps them.
func (r *Repository) Shares(ctx context.Context, collectionID string) ([]domain.Share, error) {
	var rows []shareRow
	err := r.db.WithContext(ctx).Table("app.api_key").Select("id, share, sealed, created_at, last_used_at").
		Where("collection_id = ? AND share IS NOT NULL", collectionID).Order("created_at DESC").Limit(50).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "share link")
	}
	out := make([]domain.Share, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) CreateShare(ctx context.Context, collectionID, kind, sealed string, hash []byte, by string) (*domain.Share, error) {
	var m shareRow
	// A form adds records (editor cap); a list only reads them.
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.api_key (collection_id, name, prefix, key_hash, can_write, created_by, share, sealed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id, share, created_at`,
		collectionID, "link:"+kind, domain.SharePrefix, hash, kind == domain.ShareForm, by, kind, sealed).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "share link")
	}
	s := m.toDomain()
	return &s, nil
}

type hookRow struct {
	ID           uuid.UUID  `gorm:"column:id"`
	CollectionID uuid.UUID  `gorm:"column:collection_id"`
	URL          string     `gorm:"column:url"`
	Secret       string     `gorm:"column:secret"`
	Events       []byte     `gorm:"column:events"` // a JSON array: text[] read as JSON
	CreatedAt    time.Time  `gorm:"column:created_at"`
	LastStatus   *int       `gorm:"column:last_status"`
	LastError    string     `gorm:"column:last_error"`
	LastAt       *time.Time `gorm:"column:last_at"`
}

func (m hookRow) toDomain() domain.Webhook {
	var events []string
	_ = json.Unmarshal(m.Events, &events) // absent when the query does not read it
	return domain.Webhook{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), URL: m.URL, Secret: m.Secret, Events: events,
		CreatedAt: m.CreatedAt, LastStatus: m.LastStatus, LastError: m.LastError, LastAt: m.LastAt,
	}
}

func (r *Repository) hooks(q *gorm.DB) ([]domain.Webhook, error) {
	var rows []hookRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, shared.WrapError(err, "webhook")
	}
	out := make([]domain.Webhook, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) Webhooks(ctx context.Context, collectionID string) ([]domain.Webhook, error) {
	return r.hooks(r.db.WithContext(ctx).Raw(`
		SELECT id, collection_id, url, array_to_json(events)::text AS events, created_at, last_status, last_error, last_at
		FROM app.webhook WHERE collection_id = ? ORDER BY created_at`, collectionID))
}

func (r *Repository) WebhooksFor(ctx context.Context, collectionID, event string) ([]domain.Webhook, error) {
	return r.hooks(r.db.WithContext(ctx).Raw(`
		SELECT id, url FROM app.webhook WHERE collection_id = ? AND ? = ANY (events)`, collectionID, event))
}

func (r *Repository) Webhook(ctx context.Context, id string) (*domain.Webhook, error) {
	var m hookRow
	err := r.db.WithContext(ctx).Raw(`SELECT id, collection_id, url, secret, array_to_json(events)::text AS events FROM app.webhook WHERE id = ?`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "webhook")
	}
	w := m.toDomain()
	return &w, nil
}

func (r *Repository) CreateWebhook(ctx context.Context, w *domain.Webhook, createdBy string) error {
	events, err := json.Marshal(w.Events)
	if err != nil {
		return err
	}
	var m hookRow
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO app.webhook (collection_id, url, secret, events, created_by)
		VALUES (?, ?, ?, ARRAY(SELECT jsonb_array_elements_text(?::jsonb)), ?)
		RETURNING id, created_at`, w.CollectionID, w.URL, w.Secret, string(events), createdBy).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "webhook")
	}
	w.ID, w.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) DeleteWebhook(ctx context.Context, collectionID, id string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM app.webhook WHERE id = ? AND collection_id = ?`, id, collectionID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "webhook")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "webhook")
	}
	return nil
}

func (r *Repository) WebhookResult(ctx context.Context, id string, status int, errText string) error {
	var s *int
	if status > 0 {
		s = &status
	}
	err := r.db.WithContext(ctx).Exec(`UPDATE app.webhook SET last_status = ?, last_error = ?, last_at = now() WHERE id = ?`, s, errText, id).Error
	return shared.WrapError(err, "webhook")
}
