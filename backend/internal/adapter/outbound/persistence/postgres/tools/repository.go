package tools

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

var _ output.ToolsRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// jsonIDs passes a list of ids as one JSON parameter: GORM would expand a slice.
func jsonIDs(ids []string) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

const idList = `(SELECT jsonb_array_elements_text(?::jsonb)::uuid)`

func (r *Repository) TrashRecords(ctx context.Context, ids []string, by string) error {
	var moved []uuid.UUID
	// One statement: the count check, the delete and the copy see the same snapshot.
	err := r.db.WithContext(ctx).Raw(`
		WITH gone AS (
			DELETE FROM app.record WHERE id IN `+idList+`
			AND (SELECT count(*) FROM app.record WHERE id IN `+idList+`) = ?
			RETURNING id, collection_id, data, version, created_at, updated_at
		)
		INSERT INTO app.record_trash (id, collection_id, data, version, created_at, updated_at, deleted_by)
		SELECT id, collection_id, data, version, created_at, updated_at, NULLIF(?, '')::uuid FROM gone
		RETURNING id`, jsonIDs(ids), jsonIDs(ids), len(ids), by).Find(&moved).Error
	if err != nil {
		return shared.WrapError(err, "record")
	}
	if len(moved) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "record")
	}
	return nil
}

func (r *Repository) TrashedRecords(ctx context.Context, collectionID string, limit int) ([]domain.TrashedRecord, error) {
	var rows []struct {
		ID            uuid.UUID `gorm:"column:id"`
		Data          []byte    `gorm:"column:data"`
		DeletedAt     time.Time `gorm:"column:deleted_at"`
		DeletedByName *string   `gorm:"column:deleted_by_name"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT t.id, t.data::text AS data, t.deleted_at, u.name AS deleted_by_name
		FROM app.record_trash t LEFT JOIN usr.usr_user u ON u.id = t.deleted_by
		WHERE t.collection_id = ? ORDER BY t.deleted_at DESC, t.id LIMIT ?`, collectionID, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make([]domain.TrashedRecord, 0, len(rows))
	for _, m := range rows {
		name := ""
		if m.DeletedByName != nil {
			name = *m.DeletedByName
		}
		out = append(out, domain.TrashedRecord{ID: m.ID.String(), CollectionID: collectionID, Data: m.Data, DeletedAt: m.DeletedAt, DeletedByName: name})
	}
	return out, nil
}

func (r *Repository) RestoreRecords(ctx context.Context, collectionID string, ids []string) (map[string]json.RawMessage, error) {
	var rows []struct {
		ID   uuid.UUID `gorm:"column:id"`
		Data []byte    `gorm:"column:data"`
	}
	err := r.db.WithContext(ctx).Raw(`
		WITH back AS (
			DELETE FROM app.record_trash WHERE collection_id = ? AND id IN `+idList+`
			AND (SELECT count(*) FROM app.record_trash WHERE collection_id = ? AND id IN `+idList+`) = ?
			RETURNING id, collection_id, data, version, created_at, updated_at
		)
		INSERT INTO app.record (id, collection_id, data, version, created_at, updated_at)
		SELECT id, collection_id, data, version, created_at, updated_at FROM back
		RETURNING id, data::text AS data`, collectionID, jsonIDs(ids), collectionID, jsonIDs(ids), len(ids)).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	if len(rows) == 0 {
		return nil, shared.WrapError(gorm.ErrRecordNotFound, "record")
	}
	out := make(map[string]json.RawMessage, len(rows))
	for _, m := range rows {
		out[m.ID.String()] = m.Data
	}
	return out, nil
}

func (r *Repository) PurgeRecords(ctx context.Context, collectionID string, ids []string) (int64, error) {
	q := `DELETE FROM app.record_trash WHERE collection_id = ?`
	args := []any{collectionID}
	if len(ids) > 0 {
		q += ` AND id IN ` + idList
		args = append(args, jsonIDs(ids))
	}
	var gone []uuid.UUID
	if err := r.db.WithContext(ctx).Raw(q+` RETURNING id`, args...).Find(&gone).Error; err != nil {
		return 0, shared.WrapError(err, "record")
	}
	if len(gone) > 0 {
		list := make([]string, len(gone))
		for i, id := range gone {
			list[i] = id.String()
		}
		if err := r.db.WithContext(ctx).Exec(`DELETE FROM app.record_version WHERE record_id IN `+idList, jsonIDs(list)).Error; err != nil {
			return 0, shared.WrapError(err, "record")
		}
	}
	return int64(len(gone)), nil
}

func (r *Repository) TrashCollections(ctx context.Context, ids []string, by string) (map[string]string, error) {
	var rows []struct {
		ID   uuid.UUID `gorm:"column:id"`
		Name string    `gorm:"column:name"`
	}
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.collection SET deleted_at = now(), deleted_by = NULLIF(?, '')::uuid
		WHERE id IN `+idList+` AND deleted_at IS NULL
		AND (SELECT count(*) FROM app.collection WHERE id IN `+idList+` AND deleted_at IS NULL) = ?
		RETURNING id, name`, by, jsonIDs(ids), jsonIDs(ids), len(ids)).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	if len(rows) == 0 {
		return nil, shared.WrapError(gorm.ErrRecordNotFound, "collection")
	}
	names := make(map[string]string, len(rows))
	for _, m := range rows {
		names[m.ID.String()] = m.Name
	}
	return names, nil
}

// ownsOrAdmin keeps deleted collections the user owns, or all for a platform admin (two binds).
const ownsOrAdmin = `(EXISTS (SELECT 1 FROM app.collection_member m WHERE m.collection_id = c.id AND m.user_id = ? AND m.role = 'owner')
	OR EXISTS (SELECT 1 FROM usr.usr_user_role ur JOIN usr.usr_role ro ON ro.id = ur.role_id AND ro.enabled
		WHERE ur.user_id = ? AND (ro.is_superuser OR EXISTS (SELECT 1 FROM usr.usr_role_permission p WHERE p.role_id = ro.id AND p.resource = '*'))))`

func (r *Repository) TrashedCollections(ctx context.Context, userID string) ([]domain.TrashedCollection, error) {
	var rows []struct {
		ID          uuid.UUID `gorm:"column:id"`
		Name        string    `gorm:"column:name"`
		Icon        string    `gorm:"column:icon"`
		RecordCount int64     `gorm:"column:record_count"`
		DeletedAt   time.Time `gorm:"column:deleted_at"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.id, c.name, c.icon, c.deleted_at, (SELECT count(*) FROM app.record r WHERE r.collection_id = c.id) AS record_count
		FROM app.collection c WHERE c.deleted_at IS NOT NULL AND `+ownsOrAdmin+`
		ORDER BY c.deleted_at DESC LIMIT 100`, userID, userID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := make([]domain.TrashedCollection, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.TrashedCollection{ID: m.ID.String(), Name: m.Name, Icon: m.Icon, RecordCount: m.RecordCount, DeletedAt: m.DeletedAt})
	}
	return out, nil
}

func (r *Repository) TrashedCollectionOwner(ctx context.Context, id, userID string) error {
	var found []int
	err := r.db.WithContext(ctx).Raw(`SELECT 1 FROM app.collection c WHERE c.id = ? AND c.deleted_at IS NOT NULL AND `+ownsOrAdmin+` FOR UPDATE`,
		id, userID, userID).Find(&found).Error
	if err == nil && len(found) == 0 {
		err = gorm.ErrRecordNotFound
	}
	return shared.WrapError(err, "collection")
}

func (r *Repository) RestoreCollection(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Exec(`UPDATE app.collection SET deleted_at = NULL, deleted_by = NULL WHERE id = ?`, id).Error
	return shared.WrapError(err, "collection")
}

func (r *Repository) PurgeCollection(ctx context.Context, id string) error {
	db := r.db.WithContext(ctx)
	for _, q := range []string{
		`DELETE FROM app.record_version v USING app.record r WHERE v.record_id = r.id AND r.collection_id = ?`,
		`DELETE FROM app.record_version v USING app.record_trash t WHERE v.record_id = t.id AND t.collection_id = ?`,
		`DELETE FROM app.record WHERE collection_id = ?`,
		`DELETE FROM app.collection WHERE id = ? AND deleted_at IS NOT NULL`,
	} {
		if err := db.Exec(q, id).Error; err != nil {
			return shared.WrapError(err, "collection")
		}
	}
	return nil
}

func (r *Repository) SaveSchemaVersion(ctx context.Context, collectionID string, version int, schema json.RawMessage, by string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO app.collection_schema_version (collection_id, version, schema, saved_by)
		VALUES (?, ?, ?::json, NULLIF(?, '')::uuid) ON CONFLICT DO NOTHING`, collectionID, version, string(schema), by).Error
	return shared.WrapError(err, "collection")
}

func (r *Repository) SchemaVersions(ctx context.Context, collectionID string, limit int) ([]domain.SchemaVersion, error) {
	var rows []struct {
		Version     int       `gorm:"column:version"`
		Schema      []byte    `gorm:"column:schema"`
		SavedAt     time.Time `gorm:"column:saved_at"`
		SavedByName *string   `gorm:"column:saved_by_name"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT v.version, v.schema::text AS schema, v.saved_at, u.name AS saved_by_name
		FROM app.collection_schema_version v LEFT JOIN usr.usr_user u ON u.id = v.saved_by
		WHERE v.collection_id = ? ORDER BY v.version DESC LIMIT ?`, collectionID, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := make([]domain.SchemaVersion, 0, len(rows))
	for _, m := range rows {
		name := ""
		if m.SavedByName != nil {
			name = *m.SavedByName
		}
		out = append(out, domain.SchemaVersion{Version: m.Version, Schema: m.Schema, SavedAt: m.SavedAt, SavedByName: name})
	}
	return out, nil
}

type viewRow struct {
	ID        uuid.UUID `gorm:"column:id"`
	UserID    uuid.UUID `gorm:"column:user_id"`
	Name      string    `gorm:"column:name"`
	Query     string    `gorm:"column:query"`
	Shared    bool      `gorm:"column:shared"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// shortcut: no limit per collection; add one when views reach the hundreds.
func (r *Repository) Views(ctx context.Context, collectionID, userID string) ([]domain.SavedView, error) {
	var rows []viewRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, user_id, name, query, shared, created_at FROM app.saved_view
		WHERE collection_id = ? AND (user_id = ? OR shared) ORDER BY name, id`, collectionID, userID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "view")
	}
	out := make([]domain.SavedView, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.SavedView{
			ID: m.ID.String(), CollectionID: collectionID, UserID: m.UserID.String(), Name: m.Name, Query: m.Query,
			Shared: m.Shared, Mine: m.UserID.String() == userID, CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

func (r *Repository) CreateView(ctx context.Context, v *domain.SavedView) error {
	var m viewRow
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.saved_view (collection_id, user_id, name, query, shared) VALUES (?, ?, ?, ?, ?)
		RETURNING id, created_at`, v.CollectionID, v.UserID, v.Name, v.Query, v.Shared).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "view")
	}
	v.ID, v.CreatedAt, v.Mine = m.ID.String(), m.CreatedAt, true
	return nil
}

func (r *Repository) DeleteView(ctx context.Context, collectionID, id, userID string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM app.saved_view WHERE id = ? AND collection_id = ? AND user_id = ?`, id, collectionID, userID)
	if res.Error != nil {
		return shared.WrapError(res.Error, "view")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "view")
	}
	return nil
}

type fileRow struct {
	ID           uuid.UUID `gorm:"column:id"`
	CollectionID uuid.UUID `gorm:"column:collection_id"`
	Name         string    `gorm:"column:name"`
	ContentType  string    `gorm:"column:content_type"`
	Size         int       `gorm:"column:size"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (r *Repository) CreateFile(ctx context.Context, f *domain.File, by string) error {
	var m fileRow
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.file (collection_id, name, content_type, size, created_by) VALUES (?, ?, ?, ?, NULLIF(?, '')::uuid)
		RETURNING id, created_at`, f.CollectionID, f.Name, f.ContentType, f.Size, by).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "file")
	}
	f.ID, f.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) File(ctx context.Context, id string) (*domain.File, error) {
	var m fileRow
	err := r.db.WithContext(ctx).Raw(`SELECT id, collection_id, name, content_type, size, created_at FROM app.file WHERE id = ?`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "file")
	}
	return &domain.File{ID: m.ID.String(), CollectionID: m.CollectionID.String(), Name: m.Name, ContentType: m.ContentType, Size: m.Size, CreatedAt: m.CreatedAt}, nil
}

func (r *Repository) existing(ctx context.Context, table, collectionID string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	var found []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`SELECT id FROM `+table+` WHERE collection_id = ? AND id IN `+idList, collectionID, jsonIDs(ids)).Find(&found).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	for _, id := range found {
		out[id.String()] = true
	}
	return out, nil
}

func (r *Repository) RecordsExist(ctx context.Context, collectionID string, ids []string) (map[string]bool, error) {
	return r.existing(ctx, "app.record", collectionID, ids)
}

func (r *Repository) FilesExist(ctx context.Context, collectionID string, ids []string) (map[string]bool, error) {
	return r.existing(ctx, "app.file", collectionID, ids)
}
