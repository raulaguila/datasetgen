package record

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Repository struct {
	db *gorm.DB
}

var _ output.RecordRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID           uuid.UUID `gorm:"column:id"`
	CollectionID uuid.UUID `gorm:"column:collection_id"`
	Data         []byte    `gorm:"column:data"`
	Version      int       `gorm:"column:version"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Record {
	return domain.Record{
		ID: m.ID.String(), CollectionID: m.CollectionID.String(), Data: json.RawMessage(m.Data),
		Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

const columns = "id, collection_id, data::text AS data, version, created_at, updated_at"

// preview keeps the top-level plain values of data, strings cut to 120 characters:
// the list screen shows a one-line summary, so arrays, objects and long text stay behind.
const preview = `(SELECT coalesce(jsonb_object_agg(key, CASE WHEN jsonb_typeof(value) = 'string'
	THEN to_jsonb(left(value #>> '{}', 120)) ELSE value END), '{}')
	FROM jsonb_each(CASE WHEN jsonb_typeof(data) = 'object' THEN data ELSE '{}' END)
	WHERE jsonb_typeof(value) IN ('string', 'number', 'boolean'))::text`

// shortcut: search is a trigram ILIKE over the whole JSON text, so keys match too;
// move to full-text or per-field filters when collections hold tens of thousands of records.
func (r *Repository) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	matching := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("app.record").Where("collection_id = ?", filter.CollectionID)
		if filter.Search != "" {
			q = q.Where("data::text ILIKE ?", shared.Contains(filter.Search))
		}
		if len(filter.Match) > 0 {
			q = q.Where("data @> ?::jsonb", string(filter.Match)) // idx_app_record_data_path
		}
		return q
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "record")
	}
	direction := "DESC"
	if filter.Order == "asc" {
		direction = "ASC"
	}
	sort := "created_at" // idx_app_record_collection
	if filter.Sort == "updated_at" {
		sort = "updated_at" // idx_app_record_collection_updated
	}
	var rows []row
	err := matching().Select("id, collection_id, " + preview + " AS data, version, created_at, updated_at").Order(sort + " " + direction + ", id " + direction).
		Offset(filter.Page.Offset()).Limit(filter.Page.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "record")
	}
	out := make([]domain.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, total, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Record, error) {
	var m row
	if err := r.db.WithContext(ctx).Raw(`SELECT `+columns+` FROM app.record WHERE id = ?`, id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "record")
	}
	rec := m.toDomain()
	return &rec, nil
}

func (r *Repository) Create(ctx context.Context, rec *domain.Record) error {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.record (collection_id, data) VALUES (?, ?::jsonb) RETURNING `+columns,
		rec.CollectionID, string(rec.Data)).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "record")
	}
	*rec = m.toDomain()
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, data json.RawMessage, version *int) (*domain.Record, error) {
	// Save the data being replaced first. FOR UPDATE waits for a concurrent change
	// and then reads its result, so the saved row is the one this update replaces.
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO app.record_version (record_id, version, data, saved_at)
		SELECT id, version, data, updated_at FROM app.record
		WHERE id = ? AND (?::integer IS NULL OR version = ?) FOR UPDATE
		ON CONFLICT DO NOTHING`, id, version, version).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	var rows []row
	err = r.db.WithContext(ctx).Raw(`
		UPDATE app.record SET data = ?::jsonb, version = version + 1
		WHERE id = ? AND (?::integer IS NULL OR version = ?)
		RETURNING `+columns, string(data), id, version, version).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	if len(rows) == 0 {
		if version == nil {
			return nil, shared.WrapError(gorm.ErrRecordNotFound, "record")
		}
		// The service read the row in this transaction, so a miss here is a newer version.
		return nil, apperror.New(apperror.CodeConflict, "the record was changed by someone else").WithField("version")
	}
	rec := rows[0].toDomain()
	return &rec, nil
}

func (r *Repository) DeleteMany(ctx context.Context, ids []string) error {
	var deleted []uuid.UUID
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM app.record WHERE id IN ?
		AND (SELECT count(*) FROM app.record WHERE id IN ?) = ?
		RETURNING id
	`, ids, ids, len(ids)).Find(&deleted).Error
	if err != nil {
		return shared.WrapError(err, "record")
	}
	if len(deleted) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "record")
	}
	return nil
}

func (r *Repository) CollectionsOf(ctx context.Context, ids []string) (map[string]string, error) {
	var rows []row
	if err := r.db.WithContext(ctx).Raw(`SELECT id, collection_id FROM app.record WHERE id IN ?`, ids).Find(&rows).Error; err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make(map[string]string, len(rows))
	for _, m := range rows {
		out[m.ID.String()] = m.CollectionID.String()
	}
	return out, nil
}

func (r *Repository) Scan(ctx context.Context, collectionID, afterID string, limit int) ([]domain.Record, error) {
	var rows []row
	// The zero uuid starts the scan; idx_app_record_collection_key serves the keyset.
	if afterID == "" {
		afterID = uuid.Nil.String()
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, data::text AS data, updated_at FROM app.record
		WHERE collection_id = ? AND id > ? ORDER BY id LIMIT ?`, collectionID, afterID, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make([]domain.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.Record{ID: m.ID.String(), Data: json.RawMessage(m.Data), UpdatedAt: m.UpdatedAt})
	}
	return out, nil
}

func (r *Repository) CreateMany(ctx context.Context, collectionID string, data []string) (int64, error) {
	// One JSON array parameter: GORM would expand a Go slice into a list.
	res := r.db.WithContext(ctx).Exec(`
		INSERT INTO app.record (collection_id, data)
		SELECT ?, d FROM jsonb_array_elements(?::jsonb) WITH ORDINALITY AS t(d, n) ORDER BY n`,
		collectionID, "["+strings.Join(data, ",")+"]")
	if res.Error != nil {
		return 0, shared.WrapError(res.Error, "record")
	}
	return res.RowsAffected, nil
}

func (r *Repository) CopyAll(ctx context.Context, fromCollection, toCollection string) (int64, error) {
	res := r.db.WithContext(ctx).Exec(`
		INSERT INTO app.record (collection_id, data) SELECT ?, data FROM app.record WHERE collection_id = ? ORDER BY created_at, id`,
		toCollection, fromCollection)
	if res.Error != nil {
		return 0, shared.WrapError(res.Error, "record")
	}
	return res.RowsAffected, nil
}

func (r *Repository) Versions(ctx context.Context, id string, limit int) ([]domain.RecordVersion, error) {
	var rows []versionRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT version, data::text AS data, saved_at FROM app.record_version
		WHERE record_id = ? ORDER BY version DESC LIMIT ?`, id, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make([]domain.RecordVersion, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.RecordVersion{Version: m.Version, Data: json.RawMessage(m.Data), SavedAt: m.SavedAt})
	}
	return out, nil
}

func (r *Repository) Version(ctx context.Context, id string, version int) (json.RawMessage, error) {
	var m versionRow
	err := r.db.WithContext(ctx).Raw(`SELECT data::text AS data FROM app.record_version WHERE record_id = ? AND version = ?`, id, version).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "record version")
	}
	return json.RawMessage(m.Data), nil
}

type versionRow struct {
	Version int       `gorm:"column:version"`
	Data    []byte    `gorm:"column:data"`
	SavedAt time.Time `gorm:"column:saved_at"`
}

// Fix applies one bulk change. Each op picks only the records it changes and
// saves their current data as a version first. jsonb_exists stands in for the
// ? operator, which GORM would read as a placeholder.
// shortcut: one transaction over every matching record; batch it when
// collections reach hundreds of thousands of records.
// fixSQL is the records a fix touches (where) and the data it writes (set).
func fixSQL(fix domain.RecordFix) (where string, whereArgs []any, set string, setArgs []any, err error) {
	switch fix.Op {
	case domain.FixSetDefault:
		return "jsonb_typeof(data) = 'object' AND NOT jsonb_exists(data, ?)", []any{fix.Field},
			"data || jsonb_build_object(?::text, ?::jsonb)", []any{fix.Field, string(fix.Value)}, nil
	case domain.FixRename:
		return "jsonb_exists(data, ?) AND NOT jsonb_exists(data, ?)", []any{fix.Field, fix.To},
			"(data - ?::text) || jsonb_build_object(?::text, data -> ?::text)", []any{fix.Field, fix.To, fix.Field}, nil
	case domain.FixRemove:
		return "jsonb_exists(data, ?)", []any{fix.Field}, "data - ?::text", []any{fix.Field}, nil
	}
	return "", nil, "", nil, apperror.New(apperror.CodeInvalidInput, "unknown fix").WithField("op")
}

func (r *Repository) CountFix(ctx context.Context, collectionID string, fix domain.RecordFix) (int64, error) {
	where, whereArgs, _, _, err := fixSQL(fix)
	if err != nil {
		return 0, err
	}
	var n int64
	err = r.db.WithContext(ctx).Raw(`SELECT count(*) FROM app.record WHERE collection_id = ? AND `+where,
		append([]any{collectionID}, whereArgs...)...).Scan(&n).Error
	return n, shared.WrapError(err, "record")
}

func (r *Repository) Fix(ctx context.Context, collectionID string, fix domain.RecordFix) (int64, error) {
	where, whereArgs, set, setArgs, err := fixSQL(fix)
	if err != nil {
		return 0, err
	}
	var ids []string
	err = r.db.WithContext(ctx).Raw(`
		INSERT INTO app.record_version (record_id, version, data, saved_at)
		SELECT id, version, data, updated_at FROM app.record WHERE collection_id = ? AND `+where+` FOR UPDATE
		ON CONFLICT DO NOTHING RETURNING record_id::text`, append([]any{collectionID}, whereArgs...)...).Find(&ids).Error
	if err != nil {
		return 0, shared.WrapError(err, "record")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Exec(`UPDATE app.record SET data = `+set+`, version = version + 1
		WHERE id IN (SELECT jsonb_array_elements_text(?::jsonb)::uuid)`, append(setArgs, jsonList(ids))...)
	if res.Error != nil {
		return 0, shared.WrapError(res.Error, "record")
	}
	return res.RowsAffected, nil
}

func (r *Repository) DeleteListed(ctx context.Context, collectionID string, ids []string, unchangedSince time.Time) ([]string, error) {
	var deleted []string
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM app.record WHERE collection_id = ? AND updated_at <= ?
			AND id IN (SELECT jsonb_array_elements_text(?::jsonb)::uuid)
		RETURNING id::text`, collectionID, unchangedSince, jsonList(ids)).Find(&deleted).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	return deleted, nil
}

// jsonList passes ids as one JSON array parameter; GORM would expand a slice.
func jsonList(ids []string) string {
	b, _ := json.Marshal(ids) // strings always marshal
	return string(b)
}

func (r *Repository) LockUnique(ctx context.Context, collectionID string) error {
	err := r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended('record-unique:' || ?, 0))", collectionID).Error
	return shared.WrapError(err, "record")
}

func (r *Repository) TakenValues(ctx context.Context, collectionID, excludeID, key string, values []json.RawMessage) ([]int, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = string(v)
	}
	// One containment per value: idx_app_record_data_path answers each.
	var out []int
	err := r.db.WithContext(ctx).Raw(`
		SELECT (v.i - 1)::int FROM jsonb_array_elements(?::jsonb) WITH ORDINALITY AS v(val, i)
		WHERE EXISTS (SELECT 1 FROM app.record rec
			WHERE rec.collection_id = ? AND rec.id::text <> ? AND rec.data @> jsonb_build_object(?::text, v.val))
		ORDER BY v.i`,
		"["+strings.Join(parts, ",")+"]", collectionID, excludeID, key).Scan(&out).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	return out, nil
}

func (r *Repository) DuplicatedKeys(ctx context.Context, collectionID string, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	list, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	// Runs on schema saves that turn a field unique: one pass per key over the collection.
	var out []string
	err = r.db.WithContext(ctx).Raw(`
		SELECT k FROM jsonb_array_elements_text(?::jsonb) AS k
		WHERE EXISTS (SELECT 1 FROM app.record rec
			WHERE rec.collection_id = ? AND rec.data -> k IS NOT NULL AND rec.data -> k <> 'null'::jsonb
			GROUP BY rec.data -> k HAVING count(*) > 1)`,
		string(list), collectionID).Scan(&out).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	return out, nil
}
