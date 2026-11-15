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
	ID          uuid.UUID       `gorm:"column:id"`
	Name        string          `gorm:"column:name"`
	Prefix      string          `gorm:"column:prefix"`
	CanWrite    bool            `gorm:"column:can_write"`
	Scopes      json.RawMessage `gorm:"column:scopes"`
	CreatedBy   uuid.UUID       `gorm:"column:created_by"`
	CreatorName string          `gorm:"column:creator_name"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	LastUsedAt  *time.Time      `gorm:"column:last_used_at"`
}

func (m keyRow) toDomain() (domain.APIKey, error) {
	k := domain.APIKey{
		ID: m.ID.String(), Name: m.Name, Prefix: m.Prefix, CanWrite: m.CanWrite,
		CreatedBy: m.CreatedBy.String(), CreatorName: m.CreatorName, CreatedAt: m.CreatedAt, LastUsedAt: m.LastUsedAt,
	}
	if len(m.Scopes) > 0 {
		if err := json.Unmarshal(m.Scopes, &k.Scopes); err != nil {
			return k, err
		}
	}
	return k, nil
}

func keysOf(rows []keyRow) ([]domain.APIKey, error) {
	out := make([]domain.APIKey, 0, len(rows))
	for _, m := range rows {
		k, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// Keys lists the keys that reach a collection, with their access to it.
// shortcut: no limit on keys per collection (each member holds up to 20); page it past a few hundred.
func (r *Repository) Keys(ctx context.Context, collectionID string) ([]domain.APIKey, error) {
	var rows []keyRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT k.id, k.name, k.prefix, s.can_write, k.created_by, u.name AS creator_name, k.created_at, k.last_used_at
		FROM app.api_key_collection s
		JOIN app.api_key k ON k.id = s.key_id
		JOIN usr.usr_user u ON u.id = k.created_by
		WHERE s.collection_id = ? ORDER BY k.created_at DESC`, collectionID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	return keysOf(rows)
}

// scopesOf is the JSON list of a key's collections, live ones only, by name.
const scopesOf = `COALESCE((
	SELECT json_agg(json_build_object('collection_id', s.collection_id, 'collection_name', c.name, 'can_write', s.can_write) ORDER BY c.name)
	FROM app.api_key_collection s JOIN app.collection c ON c.id = s.collection_id AND c.deleted_at IS NULL
	WHERE s.key_id = k.id), '[]')`

func (r *Repository) UserKeys(ctx context.Context, userID string) ([]domain.APIKey, error) {
	var rows []keyRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT k.id, k.name, k.prefix, k.created_by, k.created_at, k.last_used_at, `+scopesOf+` AS scopes
		FROM app.api_key k WHERE k.created_by = ? AND k.share IS NULL ORDER BY k.created_at DESC`, userID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	return keysOf(rows)
}

func (r *Repository) UserKey(ctx context.Context, id, userID string) (*domain.APIKey, error) {
	var m keyRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT k.id, k.name, k.prefix, k.created_by, k.created_at, k.last_used_at, `+scopesOf+` AS scopes
		FROM app.api_key k WHERE k.id = ? AND k.created_by = ? AND k.share IS NULL`, id, userID).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	k, err := m.toDomain()
	return &k, err
}

func scopesJSON(scopes []domain.KeyScope) (string, error) {
	b, err := json.Marshal(scopes)
	return string(b), err
}

func (r *Repository) CreateKey(ctx context.Context, k *domain.APIKey, hash []byte) error {
	scopes, err := scopesJSON(k.Scopes)
	if err != nil {
		return err
	}
	var m keyRow
	err = r.db.WithContext(ctx).Raw(`
		WITH k AS (
			INSERT INTO app.api_key (name, prefix, key_hash, created_by) VALUES (?, ?, ?, ?) RETURNING id, created_at
		), s AS (
			INSERT INTO app.api_key_collection (key_id, collection_id, can_write)
			SELECT k.id, x.collection_id, x.can_write FROM k, json_to_recordset(?::json) AS x(collection_id uuid, can_write boolean)
		)
		SELECT id, created_at FROM k`,
		k.Name, k.Prefix, hash, k.CreatedBy, scopes).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "api key")
	}
	k.ID, k.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) UpdateKey(ctx context.Context, id, name string, scopes []domain.KeyScope) error {
	list, err := scopesJSON(scopes)
	if err != nil {
		return err
	}
	// Rows the new list drops go; the others are added or get their new access.
	return shared.WrapError(r.db.WithContext(ctx).Exec(`
		WITH x AS (
			SELECT * FROM json_to_recordset(?::json) AS x(collection_id uuid, can_write boolean)
		), k AS (
			UPDATE app.api_key SET name = ? WHERE id = ?
		), d AS (
			DELETE FROM app.api_key_collection s WHERE s.key_id = ? AND s.collection_id NOT IN (SELECT collection_id FROM x)
		)
		INSERT INTO app.api_key_collection (key_id, collection_id, can_write)
		SELECT ?, collection_id, can_write FROM x
		ON CONFLICT (key_id, collection_id) DO UPDATE SET can_write = EXCLUDED.can_write`,
		list, name, id, id, id).Error, "api key")
}

func (r *Repository) DeleteKey(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM app.api_key WHERE id = ? AND share IS NULL`, id)
	if res.Error == nil && res.RowsAffected == 0 {
		res.Error = gorm.ErrRecordNotFound
	}
	return shared.WrapError(res.Error, "api key")
}

func (r *Repository) DropScope(ctx context.Context, collectionID, keyID string) (domain.DeletedKey, error) {
	var rows []domain.DeletedKey
	err := r.db.WithContext(ctx).Raw(`
		WITH s AS (DELETE FROM app.api_key_collection WHERE key_id = ? AND collection_id = ? RETURNING key_id)
		SELECT k.name, k.prefix FROM s JOIN app.api_key k ON k.id = s.key_id`, keyID, collectionID).Find(&rows).Error
	if err != nil {
		return domain.DeletedKey{}, shared.WrapError(err, "api key")
	}
	if len(rows) == 0 {
		return domain.DeletedKey{}, shared.WrapError(gorm.ErrRecordNotFound, "api key")
	}
	return rows[0], nil
}

func (r *Repository) DeleteShare(ctx context.Context, collectionID, id string) (string, error) {
	var kinds []string
	err := r.db.WithContext(ctx).Raw(`DELETE FROM app.api_key WHERE id = ? AND collection_id = ? AND share IS NOT NULL RETURNING share`, id, collectionID).Find(&kinds).Error
	if err == nil && len(kinds) == 0 {
		err = gorm.ErrRecordNotFound
	}
	if err != nil {
		return "", shared.WrapError(err, "share link")
	}
	return kinds[0], nil
}

func (r *Repository) Grant(ctx context.Context, hash []byte) (*domain.KeyGrant, error) {
	var m struct {
		ID           uuid.UUID       `gorm:"column:id"`
		Name         string          `gorm:"column:name"`
		CollectionID *uuid.UUID      `gorm:"column:collection_id"`
		CanWrite     bool            `gorm:"column:can_write"`
		CreatedBy    uuid.UUID       `gorm:"column:created_by"`
		Share        *string         `gorm:"column:share"`
		Scopes       json.RawMessage `gorm:"column:scopes"`
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
		SELECT id, name, collection_id, can_write, created_by, share,
			COALESCE((SELECT json_object_agg(s.collection_id, s.can_write) FROM app.api_key_collection s WHERE s.key_id = k.id), '{}') AS scopes
		FROM k`, hash).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "api key")
	}
	g := &domain.KeyGrant{KeyID: m.ID.String(), Name: m.Name, UserID: m.CreatedBy.String(), Scopes: map[string]bool{}}
	if err := json.Unmarshal(m.Scopes, &g.Scopes); err != nil {
		return nil, err
	}
	if m.Share != nil && m.CollectionID != nil { // a share link reaches its one collection
		g.Share, g.CollectionID = *m.Share, m.CollectionID.String()
		g.Scopes = map[string]bool{g.CollectionID: m.CanWrite}
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

func (r *Repository) DeleteWebhook(ctx context.Context, collectionID, id string) (string, error) {
	var urls []string
	err := r.db.WithContext(ctx).Raw(`DELETE FROM app.webhook WHERE id = ? AND collection_id = ? RETURNING url`, id, collectionID).Find(&urls).Error
	if err != nil {
		return "", shared.WrapError(err, "webhook")
	}
	if len(urls) == 0 {
		return "", shared.WrapError(gorm.ErrRecordNotFound, "webhook")
	}
	return urls[0], nil
}

func (r *Repository) WebhookResult(ctx context.Context, id string, status int, errText string) error {
	var s *int
	if status > 0 {
		s = &status
	}
	err := r.db.WithContext(ctx).Exec(`UPDATE app.webhook SET last_status = ?, last_error = ?, last_at = now() WHERE id = ?`, s, errText, id).Error
	return shared.WrapError(err, "webhook")
}
