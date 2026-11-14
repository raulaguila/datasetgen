package collection

import (
	"context"
	"encoding/json"
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

var _ output.CollectionRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID           uuid.UUID `gorm:"column:id"`
	Name         string    `gorm:"column:name"`
	Description  string    `gorm:"column:description"`
	Icon         string    `gorm:"column:icon"`
	Member       bool      `gorm:"column:member"`
	Schema       []byte    `gorm:"column:schema"`
	RecordCount  int64     `gorm:"column:record_count"`
	MemberCount  int64     `gorm:"column:member_count"`
	FieldCount   int64     `gorm:"column:field_count"`
	LastActivity time.Time `gorm:"column:last_activity"`
	Role         string    `gorm:"column:role"`
	Version      int       `gorm:"column:version"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (m row) toDomain() domain.Collection {
	return domain.Collection{
		ID: m.ID.String(), Name: m.Name, Description: m.Description, Icon: m.Icon, Member: m.Member, Schema: json.RawMessage(m.Schema),
		RecordCount: m.RecordCount, MemberCount: m.MemberCount, FieldCount: m.FieldCount, LastActivity: m.LastActivity, Role: domain.MemberRole(m.Role), Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// sortColumns is the allowlist behind ORDER BY, which takes no bind parameters.
var sortColumns = map[string]string{"name": "name", "created_at": "created_at", "updated_at": "updated_at"}

// platformAdmin is true when the user (one bind) holds an enabled superuser or
// full-access ('*') role; such a user sees and owns every collection.
const platformAdmin = `EXISTS (
	SELECT 1 FROM usr.usr_user_role ur
	JOIN usr.usr_role ro ON ro.id = ur.role_id AND ro.enabled
	WHERE ur.user_id = ? AND (ro.is_superuser OR EXISTS (
		SELECT 1 FROM usr.usr_role_permission p WHERE p.role_id = ro.id AND p.resource = '*')))`

// visible joins the caller's membership and keeps the rows they may see.
func visible(q *gorm.DB, userID string) *gorm.DB {
	return q.Joins("LEFT JOIN app.collection_member m ON m.collection_id = c.id AND m.user_id = ?", userID).
		Where("c.deleted_at IS NULL AND (m.user_id IS NOT NULL OR "+platformAdmin+")", userID)
}

const columns = "id, name, description, icon, schema::text AS schema, version, created_at, updated_at"

// List leaves the schema out (the list screen does not show it) and counts records per row.
func (r *Repository) List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error) {
	matching := func() *gorm.DB {
		q := visible(r.db.WithContext(ctx).Table("app.collection c"), filter.UserID)
		if filter.Search != "" {
			q = q.Where("(c.name ILIKE ? OR c.description ILIKE ?)", shared.Contains(filter.Search), shared.Contains(filter.Search))
		}
		return q
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "collection")
	}
	column, ok := sortColumns[filter.Sort]
	if !ok {
		column = "name"
	}
	direction := "ASC"
	if filter.Order == "desc" {
		direction = "DESC"
	}
	var rows []row
	err := matching().
		Select(`c.id, c.name, c.description, c.icon, c.version, c.created_at, c.updated_at, m.user_id IS NOT NULL AS member,
			(SELECT count(*) FROM app.record rc WHERE rc.collection_id = c.id) AS record_count,
			CASE WHEN `+platformAdmin+` THEN 'owner' ELSE m.role END AS role`+listDetails(filter.Details), filter.UserID).
		Order("c." + column + " " + direction + ", c.id").
		Offset(filter.Page.Offset()).Limit(filter.Page.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "collection")
	}
	out := make([]domain.Collection, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, total, nil
}

// listDetails adds the aggregates behind CollectionFilter.Details.
func listDetails(on bool) string {
	if !on {
		return ""
	}
	return `,
		(SELECT count(*) FROM app.collection_member cm WHERE cm.collection_id = c.id) AS member_count,
		CASE WHEN json_typeof(c.schema->'properties') = 'object'
			THEN (SELECT count(*) FROM json_object_keys(c.schema->'properties')) ELSE 0 END AS field_count,
		GREATEST(c.updated_at, (SELECT max(rl.updated_at) FROM app.record rl WHERE rl.collection_id = c.id)) AS last_activity`
}

func (r *Repository) ListOptions(ctx context.Context, userID string, limit int) ([]domain.Collection, error) {
	var rows []row
	err := visible(r.db.WithContext(ctx).Table("app.collection c"), userID).
		Select("c.id, c.name").Order("c.name, c.id").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := make([]domain.Collection, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Collection, error) {
	var m row
	err := r.db.WithContext(ctx).Raw(`SELECT `+columns+` FROM app.collection WHERE id = ? AND deleted_at IS NULL`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	c := m.toDomain()
	return &c, nil
}

func (r *Repository) SchemaForWrite(ctx context.Context, id string) (json.RawMessage, error) {
	var m row
	err := r.db.WithContext(ctx).Raw(`SELECT schema::text AS schema FROM app.collection WHERE id = ? AND deleted_at IS NULL FOR SHARE`, id).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	return json.RawMessage(m.Schema), nil
}

func (r *Repository) Create(ctx context.Context, c *domain.Collection) error {
	var m row
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.collection (name, description, icon, schema) VALUES (?, ?, ?, ?::json)
		RETURNING `+columns, c.Name, c.Description, c.Icon, string(c.Schema)).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "collection")
	}
	*c = m.toDomain()
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, patch domain.CollectionPatch) (*domain.Collection, error) {
	var schema *string
	if patch.Schema != nil {
		s := string(patch.Schema)
		schema = &s
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		UPDATE app.collection
		SET name = COALESCE(?::text, name),
		    description = COALESCE(?::text, description),
		    icon = COALESCE(?::text, icon),
		    schema = COALESCE(?::json, schema),
		    version = version + 1
		WHERE id = ? AND (?::integer IS NULL OR version = ?)
		RETURNING `+columns, patch.Name, patch.Description, patch.Icon, schema, id, patch.Version, patch.Version).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	if len(rows) == 0 {
		return nil, missOrStale(ctx, r.db, "app.collection", "collection", id, patch.Version)
	}
	c := rows[0].toDomain()
	return &c, nil
}

// missOrStale tells a missing row from one changed since the caller read it.
func missOrStale(ctx context.Context, db *gorm.DB, table, resource, id string, version *int) error {
	if version == nil {
		return shared.WrapError(gorm.ErrRecordNotFound, resource)
	}
	var found []int
	if err := db.WithContext(ctx).Raw(`SELECT 1 FROM `+table+` WHERE id = ?`, id).Find(&found).Error; err != nil {
		return shared.WrapError(err, resource)
	}
	if len(found) == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, resource)
	}
	return apperror.New(apperror.CodeConflict, "the record was changed by someone else").WithField("version")
}

func (r *Repository) Access(ctx context.Context, id, userID string) (domain.MemberRole, error) {
	var m struct{ Role *string }
	err := r.db.WithContext(ctx).Raw(`
		SELECT CASE WHEN `+platformAdmin+` THEN 'owner'
		       ELSE (SELECT m.role FROM app.collection_member m WHERE m.collection_id = c.id AND m.user_id = ?) END AS role
		FROM app.collection c WHERE c.id = ? AND c.deleted_at IS NULL
	`, userID, userID, id).Take(&m).Error
	if err == nil && m.Role == nil {
		err = gorm.ErrRecordNotFound // not a member: the collection stays hidden
	}
	if err != nil {
		return "", shared.WrapError(err, "collection")
	}
	return domain.MemberRole(*m.Role), nil
}

// shortcut: at most 20 referring fields; a collection pointed at from more places shows the first 20.
func (r *Repository) Referrers(ctx context.Context, targetID, userID string) ([]domain.Referrer, error) {
	var rows []struct {
		ID     uuid.UUID `gorm:"column:id"`
		Name   string    `gorm:"column:name"`
		Field  string    `gorm:"column:field"`
		Sub    string    `gorm:"column:sub"`
		Schema []byte    `gorm:"column:schema"`
	}
	// A top-level reference field, or a reference subfield of a list of objects.
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.id, c.name, f.field, f.sub, c.schema
		FROM app.collection c
		JOIN app.collection_member m ON m.collection_id = c.id AND m.user_id = ?
		CROSS JOIN LATERAL (
			SELECT p.key AS field, '' AS sub FROM json_each(c.schema -> 'properties') p WHERE p.value ->> 'x-ref' = ?
			UNION ALL
			SELECT p.key, s.key FROM json_each(c.schema -> 'properties') p
			CROSS JOIN LATERAL json_each(CASE WHEN json_typeof(p.value -> 'items' -> 'properties') = 'object' THEN p.value -> 'items' -> 'properties' ELSE '{}'::json END) s
			WHERE s.value ->> 'x-ref' = ?
		) f
		WHERE c.deleted_at IS NULL
		ORDER BY c.name, f.field, f.sub LIMIT 20`, userID, targetID, targetID).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := make([]domain.Referrer, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.Referrer{CollectionID: m.ID.String(), Name: m.Name, Field: m.Field, Sub: m.Sub, Schema: m.Schema})
	}
	return out, nil
}

type memberRow struct {
	UserID    uuid.UUID `gorm:"column:user_id"`
	Name      string    `gorm:"column:name"`
	Email     string    `gorm:"column:email"`
	Status    string    `gorm:"column:status"`
	Role      string    `gorm:"column:role"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (r *Repository) Members(ctx context.Context, id string) ([]domain.CollectionMember, error) {
	var rows []memberRow
	// shortcut: no paging; a collection has a handful of helpers. Page it past ~200.
	err := r.db.WithContext(ctx).Raw(`
		SELECT m.user_id, u.name, u.email::text AS email, u.status, m.role, m.created_at
		FROM app.collection_member m JOIN usr.usr_user u ON u.id = m.user_id
		WHERE m.collection_id = ?
		ORDER BY m.role = 'owner' DESC, u.name, m.user_id
	`, id).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "member")
	}
	out := make([]domain.CollectionMember, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.CollectionMember{
			UserID: m.UserID.String(), Name: m.Name, Email: m.Email, Status: m.Status,
			Role: domain.MemberRole(m.Role), CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

func (r *Repository) AddMember(ctx context.Context, id, userID string, role domain.MemberRole) error {
	err := r.db.WithContext(ctx).Exec(`INSERT INTO app.collection_member (collection_id, user_id, role) VALUES (?, ?, ?)`, id, userID, string(role)).Error
	return shared.WrapError(err, "member")
}

func (r *Repository) SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error {
	res := r.db.WithContext(ctx).Exec(`UPDATE app.collection_member SET role = ? WHERE collection_id = ? AND user_id = ?`, string(role), id, userID)
	return memberChanged(res)
}

func (r *Repository) RemoveMember(ctx context.Context, id, userID string) error {
	return memberChanged(r.db.WithContext(ctx).Exec(`DELETE FROM app.collection_member WHERE collection_id = ? AND user_id = ?`, id, userID))
}

func (r *Repository) HasMembership(ctx context.Context, userID string) (bool, error) {
	var found bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM app.collection_member WHERE user_id = ?)`, userID).Take(&found).Error
	return found, shared.WrapError(err, "member")
}

func (r *Repository) Owners(ctx context.Context, id string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM app.collection_member WHERE collection_id = ? AND role = 'owner'`, id).Take(&n).Error
	return n, shared.WrapError(err, "member")
}

func memberChanged(res *gorm.DB) error {
	if res.Error == nil && res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "member")
	}
	return shared.WrapError(res.Error, "member")
}

type dayRow struct {
	Day   string `gorm:"column:day"`
	Count int64  `gorm:"column:count"`
}

type contributorRow struct {
	UserID  uuid.UUID `gorm:"column:user_id"`
	Name    string    `gorm:"column:name"`
	Created int64     `gorm:"column:created"`
	Edited  int64     `gorm:"column:edited"`
}

func (r *Repository) Stats(ctx context.Context, id, tz string, days int) (*domain.CollectionStats, error) {
	db := r.db.WithContext(ctx)
	var head struct {
		Records    int64      `gorm:"column:records"`
		Members    int64      `gorm:"column:members"`
		LastUpdate *time.Time `gorm:"column:last_update"`
	}
	err := db.Raw(`
		SELECT (SELECT count(*) FROM app.record WHERE collection_id = ?) AS records,
		       (SELECT count(*) FROM app.collection_member WHERE collection_id = ?) AS members,
		       (SELECT max(updated_at) FROM app.record WHERE collection_id = ?) AS last_update
	`, id, id, id).Take(&head).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	since := time.Now().AddDate(0, 0, -days)
	var perDay []dayRow
	// idx_app_record_collection (collection_id, created_at) serves the window.
	err = db.Raw(`
		SELECT to_char(created_at AT TIME ZONE ?, 'YYYY-MM-DD') AS day, count(*) AS count
		FROM app.record WHERE collection_id = ? AND created_at >= ?
		GROUP BY 1 ORDER BY 1
	`, tz, id, since).Find(&perDay).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	// Records carry no author; the audit log does (idx_usr_audit_log_collection).
	var people []contributorRow
	err = db.Raw(`
		SELECT a.actor_id AS user_id, u.name,
		       count(*) FILTER (WHERE a.action = 'record.create') AS created,
		       count(*) FILTER (WHERE a.action = 'record.update') AS edited
		FROM usr.usr_audit_log a JOIN usr.usr_user u ON u.id = a.actor_id
		WHERE a.details->>'collection_id' = ? AND a.created_at >= ? AND a.action IN ('record.create', 'record.update')
		GROUP BY a.actor_id, u.name ORDER BY count(*) DESC, u.name LIMIT 10
	`, id, since).Find(&people).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	out := &domain.CollectionStats{Records: head.Records, Members: head.Members, LastUpdate: head.LastUpdate,
		PerDay: make([]domain.DayCount, 0, len(perDay)), Contributors: make([]domain.Contributor, 0, len(people))}
	for _, d := range perDay {
		out.PerDay = append(out.PerDay, domain.DayCount{Day: d.Day, Count: d.Count})
	}
	for _, p := range people {
		out.Contributors = append(out.Contributors, domain.Contributor{UserID: p.UserID.String(), Name: p.Name, Created: p.Created, Edited: p.Edited})
	}
	return out, nil
}

type qualityRow struct {
	SchemaVersion int        `gorm:"column:schema_version"`
	Status        string     `gorm:"column:status"`
	Checked       int        `gorm:"column:checked"`
	InvalidCount  int        `gorm:"column:invalid_count"`
	Truncated     bool       `gorm:"column:truncated"`
	Invalid       []byte     `gorm:"column:invalid"`
	InvalidIDs    []byte     `gorm:"column:invalid_ids"`
	Error         string     `gorm:"column:error"`
	StartedAt     time.Time  `gorm:"column:started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at"`
}

func (r *Repository) Quality(ctx context.Context, id string, withIDs bool) (*domain.QualityRun, error) {
	ids := "'[]'"
	if withIDs { // up to the scan cap of uuids: only the delete reads them
		ids = "array_to_json(invalid_ids)::text"
	}
	var rows []qualityRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT schema_version, status, checked, invalid_count, truncated, invalid::text AS invalid, `+ids+` AS invalid_ids,
			error, started_at, finished_at FROM app.collection_quality WHERE collection_id = ?`, id).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "collection")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	m := rows[0]
	var invalidIDs []string
	if err := json.Unmarshal(m.InvalidIDs, &invalidIDs); err != nil {
		return nil, err
	}
	return &domain.QualityRun{
		SchemaVersion: m.SchemaVersion, Status: m.Status, Checked: m.Checked, InvalidCount: m.InvalidCount, Truncated: m.Truncated,
		Invalid: json.RawMessage(m.Invalid), InvalidIDs: invalidIDs, Error: m.Error, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt,
	}, nil
}

func (r *Repository) StartQuality(ctx context.Context, id string, schemaVersion int, stale time.Duration, force bool) (string, error) {
	var started []string
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.collection_quality (collection_id, schema_version, status) VALUES (?, ?, 'running')
		ON CONFLICT (collection_id) DO UPDATE SET schema_version = EXCLUDED.schema_version, status = 'running',
			error = '', started_at = now(), finished_at = NULL
		WHERE ? OR app.collection_quality.status <> 'running' OR app.collection_quality.started_at < now() - ?::interval
		RETURNING started_at::text`, id, schemaVersion, force, stale.String()).Find(&started).Error
	if err != nil || len(started) == 0 {
		return "", shared.WrapError(err, "collection")
	}
	return started[0], nil
}

func (r *Repository) FinishQuality(ctx context.Context, id, token string, run domain.QualityRun) error {
	invalid := string(run.Invalid)
	if invalid == "" {
		invalid = "[]"
	}
	ids, err := json.Marshal(append([]string{}, run.InvalidIDs...))
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Exec(`
		UPDATE app.collection_quality SET status = ?, checked = ?, invalid_count = ?, truncated = ?, invalid = ?::jsonb,
			invalid_ids = ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid), error = ?, finished_at = now()
		WHERE collection_id = ? AND (? = '' OR started_at::text = ?)`,
		run.Status, run.Checked, run.InvalidCount, run.Truncated, invalid, string(ids), run.Error, id, token, token).Error
	return shared.WrapError(err, "collection")
}
