package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
// the list screen shows a one-line summary, so objects and long text stay behind.
// An object with an id and a name (an attached file) keeps just those two, for its link;
// a list of strings (tags, references) or of such objects (several files) keeps its first 10.
const preview = `(SELECT coalesce(jsonb_object_agg(key, CASE
	WHEN jsonb_typeof(value) = 'string' THEN to_jsonb(left(value #>> '{}', 120))
	WHEN jsonb_typeof(value) = 'object' THEN jsonb_build_object('id', value -> 'id', 'name', left(value ->> 'name', 120))
	WHEN jsonb_typeof(value) = 'array' AND jsonb_path_exists(value, '$[*] ? (@.type() == "object")') THEN (SELECT jsonb_agg(jsonb_build_object('id', e -> 'id', 'name', left(e ->> 'name', 120))) FROM (SELECT e FROM jsonb_array_elements(value) e LIMIT 10) x)
	WHEN jsonb_typeof(value) = 'array' THEN (SELECT coalesce(jsonb_agg(left(e, 120)), '[]') FROM (SELECT e FROM jsonb_array_elements_text(value) e LIMIT 10) x)
	ELSE value END), '{}')
	FROM jsonb_each(CASE WHEN jsonb_typeof(data) = 'object' THEN data ELSE '{}' END)
	WHERE jsonb_typeof(value) IN ('string', 'number', 'boolean')
		OR (jsonb_typeof(value) = 'object' AND jsonb_typeof(value -> 'id') = 'string' AND jsonb_typeof(value -> 'name') = 'string')
		OR (jsonb_typeof(value) = 'array' AND NOT jsonb_path_exists(value, '$[*] ? (@.type() != "string")'))
		OR (jsonb_typeof(value) = 'array' AND NOT jsonb_path_exists(value, '$[*] ? (@.type() != "object" || !(@.id.type() == "string") || !(@.name.type() == "string"))')))::text`

// shortcut: search is a trigram ILIKE over the whole JSON text, so keys match too;
// move to full-text or per-field filters when collections hold tens of thousands of records.
// matching selects the filter's records of app.record.
func (r *Repository) matching(ctx context.Context, filter domain.RecordFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Table("app.record").Where("collection_id = ?", filter.CollectionID)
	if filter.Search != "" {
		q = q.Where("data::text ILIKE ?", shared.Contains(filter.Search))
	}
	if len(filter.Match) > 0 {
		q = q.Where("data @> ?::jsonb", string(filter.Match)) // idx_app_record_data_path
	}
	if len(filter.IDs) > 0 {
		ids, _ := json.Marshal(filter.IDs)
		q = q.Where("id IN (SELECT jsonb_array_elements_text(?::jsonb)::uuid)", string(ids))
	}
	if sql, vars := whereSQL(filter.Where); sql != "" {
		q = q.Where(sql, vars...)
	}
	return q
}

// whereSQL turns the rules into one condition; field names and values are bind
// parameters, never SQL text (the service checked the operators).
func whereSQL(w *domain.Where) (string, []any) {
	if w.Empty() {
		return "", nil
	}
	parts := make([]string, 0, len(w.Rules)+len(w.Groups))
	var vars []any
	for i := range w.Groups {
		if sql, v := whereSQL(&w.Groups[i]); sql != "" {
			parts, vars = append(parts, sql), append(vars, v...)
		}
	}
	for _, c := range w.Rules {
		if sql, v := ruleSQL(c); sql != "" {
			parts, vars = append(parts, sql), append(vars, v...)
		}
	}
	join := " AND "
	if w.Any {
		join = " OR "
	}
	return "(" + strings.Join(parts, join) + ")", vars
}

// uuidText matches an id as text, so a cast never fails on an odd value.
const uuidText = `'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'`

// fieldSQL is the rule's field as a jsonb expression: a top-level key, the
// record's own date, a lookup's value(s) on the named records, or a rollup.
// Subqueries name the outer row "record" (matching gives app.record no alias).
func fieldSQL(c domain.Condition) (string, []any) {
	switch {
	case c.Field == domain.FieldCreatedAt:
		return "to_jsonb(record.created_at)", nil
	case c.Field == domain.FieldUpdatedAt:
		return "to_jsonb(record.updated_at)", nil
	case c.Lookup != nil:
		l := c.Lookup
		// One id gives the value; a list of ids gives the list of values.
		return `(CASE jsonb_typeof(record.data -> ?::text)
			WHEN 'string' THEN (SELECT t.data -> ?::text FROM app.record t WHERE t.collection_id = ?::uuid
				AND t.id = (SELECT (record.data ->> ?::text)::uuid WHERE record.data ->> ?::text ~ ` + uuidText + `))
			WHEN 'array' THEN (SELECT jsonb_agg(t.data -> ?::text) FROM app.record t WHERE t.collection_id = ?::uuid
				AND t.id = ANY(ARRAY(SELECT x::uuid FROM jsonb_array_elements_text(record.data -> ?::text) x WHERE x ~ ` + uuidText + `))
				AND t.data -> ?::text IS NOT NULL) END)`,
			[]any{l.Ref, l.Field, l.Collection, l.Ref, l.Ref, l.Field, l.Collection, l.Ref, l.Field}
	case c.Rollup != nil:
		r := c.Rollup
		agg, args := "count(*)", []any{}
		if r.Fn != "count" {
			num := map[string]string{"sum": "sum(%s)", "avg": "round(avg(%s), 6)", "min": "min(%s)", "max": "max(%s)"}[r.Fn]
			agg = fmt.Sprintf(num, "(o.data ->> ?::text)::numeric") + " FILTER (WHERE jsonb_typeof(o.data -> ?::text) = 'number')"
			args = append(args, r.Field, r.Field)
		}
		// Both containment forms use idx_app_record_data_path, as Rollup does.
		return `(SELECT to_jsonb(` + agg + `) FROM app.record o WHERE o.collection_id = ?::uuid
			AND (o.data @> jsonb_build_object(?::text, record.id::text) OR o.data @> jsonb_build_object(?::text, jsonb_build_array(record.id::text))))`,
			append(args, r.Collection, r.Ref, r.Ref)
	}
	return "record.data -> ?::text", []any{c.Field}
}

// isDate is a YYYY-MM-DD value: it compares with the first 10 characters of
// the field, so a date-time matches by its day.
func isDate(v json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(v, &s) != nil || len(s) != 10 {
		return "", false
	}
	_, err := time.Parse(time.DateOnly, s)
	return s, err == nil
}

// prefilter is a necessary condition on the whole JSON text that the trigram
// index (idx_app_record_data_trgm) serves; text JSON escapes cannot have one.
func prefilter(c domain.Condition, text string) (string, []any) {
	if c.Lookup != nil || c.Rollup != nil || strings.HasPrefix(c.Field, "$") || len([]rune(text)) < 3 ||
		strings.ContainsFunc(text, func(r rune) bool { return r == '"' || r == '\\' || r < ' ' }) {
		return "", nil
	}
	return "record.data::text ILIKE ? AND ", []any{shared.Contains(text)}
}

// ruleSQL is one rule's condition.
func ruleSQL(c domain.Condition) (string, []any) {
	x, xv := fieldSQL(c)
	text := "(" + x + " #>> '{}')"
	with := func(vals ...any) []any { return append(append([]any{}, xv...), vals...) }
	// byDay compares the field's first 10 characters (the expression appears twice).
	byDay := func(op, d string) (string, []any) {
		return "(jsonb_typeof(" + x + ") = 'string' AND left(" + text + ", 10) " + op + " ?)", append(with(xv...), d)
	}
	// compare is the field op one value: by day for a date, else same JSON type only
	// (a number never compares with a string).
	compare := func(op string, v json.RawMessage) (string, []any) {
		if d, ok := isDate(v); ok {
			return byDay(op, d)
		}
		return "(jsonb_typeof(" + x + ") = jsonb_typeof(?::jsonb) AND " + x + " " + op + " ?::jsonb)", append(append(with(string(v)), xv...), string(v))
	}
	v := string(c.Value)
	switch c.Op {
	case domain.OpEq:
		if d, ok := isDate(c.Value); ok {
			return byDay("=", d)
		}
		return x + " = ?::jsonb", with(v)
	case domain.OpNeq:
		if d, ok := isDate(c.Value); ok {
			sql, args := byDay("=", d)
			return "NOT coalesce(" + sql + ", false)", args
		}
		return x + " IS DISTINCT FROM ?::jsonb", with(v)
	case domain.OpContains, domain.OpStartsWith:
		var t string
		_ = json.Unmarshal(c.Value, &t)
		pattern := shared.Contains(t)
		if c.Op == domain.OpStartsWith {
			pattern = strings.TrimPrefix(pattern, "%")
		}
		pre, pv := prefilter(c, t)
		return "(" + pre + text + " ILIKE ?)", append(append(pv, xv...), pattern)
	case domain.OpNotContains:
		var t string
		_ = json.Unmarshal(c.Value, &t)
		return "coalesce(" + text + ", '') NOT ILIKE ?", with(shared.Contains(t))
	case domain.OpGt, domain.OpGte, domain.OpLt, domain.OpLte:
		op := map[string]string{domain.OpGt: ">", domain.OpGte: ">=", domain.OpLt: "<", domain.OpLte: "<="}[c.Op]
		return compare(op, c.Value)
	case domain.OpBetween:
		var ends []json.RawMessage
		_ = json.Unmarshal(c.Value, &ends)
		from, fv := compare(">=", ends[0])
		to, tv := compare("<=", ends[1])
		return "(" + from + " AND " + to + ")", append(fv, tv...)
	case domain.OpEmpty:
		return "coalesce(" + x + `, 'null') IN ('null'::jsonb, '""'::jsonb, '[]'::jsonb)`, xv
	case domain.OpNotEmpty:
		return "coalesce(" + x + `, 'null') NOT IN ('null'::jsonb, '""'::jsonb, '[]'::jsonb)`, xv
	case domain.OpIn:
		return "?::jsonb @> jsonb_build_array(" + x + ")", append([]any{v}, xv...)
	case domain.OpNotIn:
		return "NOT coalesce(?::jsonb @> jsonb_build_array(" + x + "), false)", append([]any{v}, xv...)
	case domain.OpHas:
		return x + " @> jsonb_build_array(?::jsonb)", with(v)
	case domain.OpHasAny:
		return "(jsonb_typeof(" + x + ") = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(?::jsonb) e WHERE " + x + " @> jsonb_build_array(e)))",
			append(append(append([]any{}, xv...), v), xv...)
	}
	return "", nil
}

// shortcut: groups and sums scan the filter's rows each time, fine to ~100 000 records per collection.
func (r *Repository) Aggregate(ctx context.Context, filter domain.RecordFilter, q domain.GroupQuery) ([]domain.RecordGroup, error) {
	group, numeric := q.Group, q.Numeric
	cols := []string{"count(*) AS n"}
	var vars []any
	key := "NULL::text"
	switch {
	case group == "":
	case q.Bucket != "":
		// The span's first day as a JSON string; values that are not dates group as null.
		key = `CASE WHEN jsonb_typeof(data -> ?) = 'string' AND pg_input_is_valid(left(data ->> ?, 10), 'date')
			THEN to_json(to_char(date_trunc(?, left(data ->> ?, 10)::date), 'YYYY-MM-DD'))::text END`
		vars = append(vars, group, group, q.Bucket, group)
	case q.Elements:
		key = "g.v::text"
	default:
		key = "(data -> ?)::text"
		vars = append(vars, group)
	}
	for i, f := range numeric {
		num := "(data ->> ?)::numeric) FILTER (WHERE jsonb_typeof(data -> ?) = 'number')"
		cols = append(cols, fmt.Sprintf("sum("+num+" AS s%d, avg("+num+" AS a%d, min("+num+" AS m%d, max("+num+" AS x%d", i, i, i, i))
		vars = append(vars, f, f, f, f, f, f, f, f)
	}
	db := r.matching(ctx, filter)
	if group != "" && q.Elements && q.Bucket == "" {
		db = db.Joins("CROSS JOIN LATERAL jsonb_array_elements("+elementsOf+") AS g(v)", group, group, group)
	}
	db = db.Select(key+" AS k, "+strings.Join(cols, ", "), vars...)
	if group != "" {
		db = db.Group("k").Order("n DESC").Limit(100)
	}
	rows, err := db.Rows()
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	var out []domain.RecordGroup
	err = scanRows(rows, func() error {
		var k *string
		var n int64
		nums := make([]*float64, 4*len(numeric))
		dest := []any{&k, &n}
		for i := range nums {
			dest = append(dest, &nums[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		g := domain.RecordGroup{Count: n, Sums: map[string]float64{}, Avgs: map[string]float64{}, Mins: map[string]float64{}, Maxs: map[string]float64{}}
		if k != nil {
			g.Key = json.RawMessage(*k)
		}
		for i, f := range numeric {
			if nums[4*i] != nil {
				g.Sums[f], g.Avgs[f], g.Mins[f], g.Maxs[f] = *nums[4*i], *nums[4*i+1], *nums[4*i+2], *nums[4*i+3]
			}
		}
		out = append(out, g)
		return nil
	})
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	return out, nil
}

// scanRows owns the cursor, including when scanning a row fails.
func scanRows(rows *sql.Rows, scan func() error) (err error) {
	defer func() {
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
	}()
	for rows.Next() {
		if err := scan(); err != nil {
			return err
		}
	}
	return rows.Err()
}

// elementsOf is a field's list, or the field alone as a list of one, for grouping by each element;
// an empty list leaves the record out. Takes the field three times.
const elementsOf = "CASE WHEN jsonb_typeof(data -> ?::text) = 'array' THEN data -> ?::text ELSE jsonb_build_array(data -> ?::text) END"

// filledSQL is true when a field holds a value: not missing, null, "" or [].
const filledSQL = `coalesce(data -> ?::text, 'null') NOT IN ('null'::jsonb, '""'::jsonb, '[]'::jsonb)`

// Stats reads the profile in up to three queries: one row of counts and
// ranges, the commonest values of every value field, and the date series.
// shortcut: each scans the filter's rows, fine to ~100 000 records per collection, like Aggregate.
func (r *Repository) Stats(ctx context.Context, filter domain.RecordFilter, q domain.StatsQuery) (*domain.RecordStats, error) {
	out := &domain.RecordStats{Fields: map[string]*domain.FieldStats{}}
	field := func(f string) *domain.FieldStats {
		if out.Fields[f] == nil {
			out.Fields[f] = &domain.FieldStats{}
		}
		return out.Fields[f]
	}
	cols := []string{"count(*)"}
	var vars []any
	for _, f := range q.Filled {
		cols, vars = append(cols, "count(*) FILTER (WHERE "+filledSQL+")"), append(vars, f)
	}
	for _, f := range q.Numbers {
		num := "(data ->> ?::text)::numeric) FILTER (WHERE jsonb_typeof(data -> ?::text) = 'number')"
		cols = append(cols, "min("+num, "max("+num, "avg("+num, "sum("+num)
		vars = append(vars, f, f, f, f, f, f, f, f)
	}
	for _, f := range q.Dates {
		// Text order is date order for ISO dates; the check skips anything else.
		d := "(data ->> ?::text) FILTER (WHERE jsonb_typeof(data -> ?::text) = 'string' AND pg_input_is_valid(left(data ->> ?::text, 10), 'date'))"
		cols = append(cols, "left(min"+d+", 10)", "left(max"+d+", 10)")
		vars = append(vars, f, f, f, f, f, f)
	}
	nums := make([]*float64, 4*len(q.Numbers))
	dates := make([]*string, 2*len(q.Dates))
	filled := make([]int64, len(q.Filled))
	dest := []any{&out.Total}
	for i := range filled {
		dest = append(dest, &filled[i])
	}
	for i := range nums {
		dest = append(dest, &nums[i])
	}
	for i := range dates {
		dest = append(dest, &dates[i])
	}
	if err := r.matching(ctx, filter).Select(strings.Join(cols, ", "), vars...).Row().Scan(dest...); err != nil {
		return nil, shared.WrapError(err, "record")
	}
	for i, f := range q.Filled {
		field(f).Filled = filled[i]
	}
	for i, f := range q.Numbers {
		s := field(f)
		s.Min, s.Max, s.Avg, s.Sum = nums[4*i], nums[4*i+1], nums[4*i+2], nums[4*i+3]
	}
	for i, f := range q.Dates {
		field(f).First, field(f).Last = dates[2*i], dates[2*i+1]
	}

	if len(q.Values) > 0 {
		value := "(data -> f)"
		inner := r.matching(ctx, filter).
			Joins("CROSS JOIN jsonb_array_elements_text(?::jsonb) AS k(f)", jsonList(q.Values))
		if q.Elements {
			value = "e.v"
			inner = inner.Joins("CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(data -> f) = 'array' THEN data -> f ELSE jsonb_build_array(data -> f) END) AS e(v)")
		}
		inner = inner.
			Select("f, " + value + "::text AS k, count(*) AS n, row_number() OVER (PARTITION BY f ORDER BY count(*) DESC, " + value + "::text) AS rn, count(*) OVER (PARTITION BY f) AS d").
			Group("f, k")
		rows, err := r.db.WithContext(ctx).Table("(?) AS t", inner).Select("f, k, n, d, rn").Where("rn = 1 OR rn > ? AND rn <= ?", q.Skip, q.Skip+q.Top).Order("f, rn").Rows()
		if err != nil {
			return nil, shared.WrapError(err, "record")
		}
		err = scanRows(rows, func() error {
			var f string
			var k *string
			var n, d, rn int64
			if err := rows.Scan(&f, &k, &n, &d, &rn); err != nil {
				return err
			}
			s := field(f)
			s.Distinct = d
			if rn <= int64(q.Skip) { // the first value only tells the distinct count of a later page
				return nil
			}
			g := domain.RecordGroup{Count: n}
			if k != nil {
				g.Key = json.RawMessage(*k)
			}
			s.Top = append(s.Top, g)
			return nil
		})
		if err != nil {
			return nil, shared.WrapError(err, "record")
		}
	}

	if len(q.Dates) > 0 {
		// shortcut: at most 2 000 buckets across the date fields; a daily series of years is cut, pick a wider bucket.
		rows, err := r.matching(ctx, filter).
			Joins("CROSS JOIN jsonb_array_elements_text(?::jsonb) AS k(f)", jsonList(q.Dates)).
			Where("jsonb_typeof(data -> f) = 'string' AND pg_input_is_valid(left(data ->> f, 10), 'date')").
			Select("f, to_char(date_trunc(?::text, left(data ->> f, 10)::date::timestamp), 'YYYY-MM-DD') AS b, count(*)", q.Bucket).
			Group("f, b").Order("f, b").Limit(2000).Rows()
		if err != nil {
			return nil, shared.WrapError(err, "record")
		}
		err = scanRows(rows, func() error {
			var f string
			var c domain.DateCount
			if err := rows.Scan(&f, &c.At, &c.Count); err != nil {
				return err
			}
			field(f).Series = append(field(f).Series, c)
			return nil
		})
		if err != nil {
			return nil, shared.WrapError(err, "record")
		}
	}
	return out, nil
}

func (r *Repository) List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error) {
	matching := func() *gorm.DB { return r.matching(ctx, filter) }
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "record")
	}
	direction := "DESC"
	if filter.Order == "asc" {
		direction = "ASC"
	}
	sort := "created_at" // idx_app_record_collection_created
	if filter.Sort == "updated_at" {
		sort = "updated_at" // idx_app_record_collection_updated
	}
	order := clause.OrderBy{Expression: clause.Expr{SQL: sort + " " + direction + ", id " + direction}}
	if field := filter.SortField(); field != "" {
		// jsonb order: null, strings, numbers, booleans; numbers compare as numbers.
		// shortcut: no index behind it, so each page sorts the collection's matching rows
		// (43 ms at 100 000 records, measured 2026-11); add an expression index per sorted field past ~1 000 000.
		order = clause.OrderBy{Expression: clause.Expr{SQL: "data -> ? " + direction + " NULLS LAST, id " + direction, Vars: []any{field}}}
	}
	var rows []row
	// shortcut: offset paging; page 3 600 of 100 000 records takes 56 ms (measured 2026-11). Move to keyset paging past ~1 000 000.
	err := matching().Select("id, collection_id, " + preview + " AS data, version, created_at, updated_at").Order(order).
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
	if err := r.prune(ctx, `SELECT ?::uuid`, id); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateMany takes the expected versions from Matching. A conflict rolls back
// the surrounding import, including history and jobs for earlier segments.
func (r *Repository) UpdateMany(ctx context.Context, changes []output.RecordChange) ([]domain.Record, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	batch := string(payload)
	db := r.db.WithContext(ctx)
	const incoming = `jsonb_to_recordset(?::jsonb) AS x(id uuid, version int, data jsonb)`
	err = db.Exec(`INSERT INTO app.record_version (record_id,version,data,saved_at)
 SELECT r.id,r.version,r.data,r.updated_at FROM app.record r JOIN `+incoming+`
 ON r.id=x.id AND r.version=x.version ORDER BY r.id FOR UPDATE OF r
 ON CONFLICT DO NOTHING`, batch).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	var rows []row
	err = db.Raw(`UPDATE app.record r SET data=x.data, version=r.version+1 FROM `+incoming+`
 WHERE r.id=x.id AND r.version=x.version
 RETURNING r.id,r.collection_id,r.data::text AS data,r.version,r.created_at,r.updated_at`, batch).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	if len(rows) != len(changes) {
		return nil, apperror.New(apperror.CodeConflict, "a record was changed by someone else").WithField("version")
	}
	ids := make([]string, len(changes))
	positions := make(map[string]int, len(changes))
	for i, c := range changes {
		ids[i] = c.ID
		positions[c.ID] = i
	}
	if err := r.prune(ctx, `SELECT jsonb_array_elements_text(?::jsonb)::uuid`, jsonList(ids)); err != nil {
		return nil, err
	}
	out := make([]domain.Record, len(rows))
	for _, m := range rows {
		rec := m.toDomain()
		out[positions[rec.ID]] = rec
	}
	return out, nil
}

// keepVersions is how many earlier versions a record keeps; older ones go.
const keepVersions = 100

// prune drops the versions past the latest keepVersions of the records the
// ids query selects; the primary key (record_id, version) serves it.
func (r *Repository) prune(ctx context.Context, ids string, args ...any) error {
	err := r.db.WithContext(ctx).Exec(`
		DELETE FROM app.record_version v USING app.record r
		WHERE r.id IN (`+ids+`) AND v.record_id = r.id AND v.version <= r.version - ?`,
		append(args, keepVersions+1)...).Error
	return shared.WrapError(err, "record")
}

func (r *Repository) LookupValues(ctx context.Context, collectionID string, ids []string, field string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID    string
		Value string
	}
	// ids come from one page of records (up to 100, each list ref up to its schema's size).
	err := r.db.WithContext(ctx).Raw(`
		SELECT id::text AS id, (data->?::text)::text AS value FROM app.record
		WHERE collection_id = ? AND id IN ? AND data->?::text IS NOT NULL`, field, collectionID, ids, field).Scan(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	for _, row := range rows {
		out[row.ID] = json.RawMessage(row.Value)
	}
	return out, nil
}

// rollupSQL holds each function's aggregate; only numbers count toward sum,
// avg, min and max, so a text value in the field is skipped, not an error.
var rollupSQL = map[string]string{
	"count": "count(*)",
	"sum":   "sum((r.data->>?::text)::numeric) FILTER (WHERE jsonb_typeof(r.data->?::text) = 'number')",
	"avg":   "round(avg((r.data->>?::text)::numeric) FILTER (WHERE jsonb_typeof(r.data->?::text) = 'number'), 6)",
	"min":   "min((r.data->>?::text)::numeric) FILTER (WHERE jsonb_typeof(r.data->?::text) = 'number')",
	"max":   "max((r.data->>?::text)::numeric) FILTER (WHERE jsonb_typeof(r.data->?::text) = 'number')",
}

func (r *Repository) Rollup(ctx context.Context, collectionID, ref string, parents []string, fn, field string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	agg, ok := rollupSQL[fn]
	if !ok {
		return nil, fmt.Errorf("unknown rollup %q", fn)
	}
	if len(parents) == 0 {
		return out, nil
	}
	args := []any{}
	if fn != "count" {
		args = append(args, field, field)
	}
	args = append(args, strings.Join(parents, ","), collectionID, ref, ref) // ids checked by the caller: no commas
	var rows []struct {
		ID    string
		Value *string
	}
	// Both containment forms use idx_app_record_data_path (GIN jsonb_path_ops).
	err := r.db.WithContext(ctx).Raw(`
		SELECT p.id::text AS id, to_jsonb(`+agg+`)::text AS value
		FROM unnest(string_to_array(?, ',')::uuid[]) AS p(id)
		JOIN app.record r ON r.collection_id = ?
			AND (r.data @> jsonb_build_object(?::text, p.id::text) OR r.data @> jsonb_build_object(?::text, jsonb_build_array(p.id::text)))
		GROUP BY p.id`, args...).Scan(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	for _, row := range rows {
		if row.Value != nil && *row.Value != "null" {
			out[row.ID] = json.RawMessage(*row.Value)
		}
	}
	return out, nil
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
	// The zero uuid starts the scan; idx_app_record_collection_id serves the keyset.
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

func (r *Repository) WithoutNumber(ctx context.Context, collectionID, key string, limit int) ([]domain.Record, error) {
	var rows []row
	// idx_app_record_collection_created serves the order; the filter reads the rows it walks.
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, data::text AS data FROM app.record
		WHERE collection_id = ? AND jsonb_typeof(data) = 'object' AND jsonb_typeof(data -> ?) IS DISTINCT FROM 'number'
		ORDER BY created_at, id LIMIT ?`, collectionID, key, limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make([]domain.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.Record{ID: m.ID.String(), Data: json.RawMessage(m.Data)})
	}
	return out, nil
}

func (r *Repository) Keys(ctx context.Context, collectionID string, limit int) ([]string, error) {
	var keys []string
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT k FROM app.record, jsonb_object_keys(CASE WHEN jsonb_typeof(data) = 'object' THEN data ELSE '{}' END) AS k
		WHERE collection_id = ? ORDER BY k LIMIT ?`, collectionID, limit).Scan(&keys).Error
	return keys, shared.WrapError(err, "record")
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
	if err := r.prune(ctx, `SELECT jsonb_array_elements_text(?::jsonb)::uuid`, jsonList(ids)); err != nil {
		return 0, err
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

func (r *Repository) Oversized(ctx context.Context, docs []json.RawMessage) ([]int, error) {
	// pg_column_size of each element is what chk_app_record_data_size measures on insert.
	var out []int
	err := r.db.WithContext(ctx).Raw(`
		SELECT (n - 1)::int FROM jsonb_array_elements(?::jsonb) WITH ORDINALITY AS t(d, n)
		WHERE pg_column_size(d) > ? ORDER BY n`,
		rawArray(docs), domain.RecordMaxBytes).Scan(&out).Error
	return out, shared.WrapError(err, "record")
}

func (r *Repository) ClaimImportBatch(ctx context.Context, collectionID, key string) (*output.ImportBatch, error) {
	res := r.db.WithContext(ctx).Exec(`INSERT INTO app.import_batch (collection_id, batch_key) VALUES (?, ?) ON CONFLICT DO NOTHING`, collectionID, key)
	if res.Error != nil {
		return nil, shared.WrapError(res.Error, "import batch")
	}
	if res.RowsAffected == 1 {
		return nil, nil
	}
	var prev output.ImportBatch
	err := r.db.WithContext(ctx).Raw(`SELECT created, updated FROM app.import_batch WHERE collection_id = ? AND batch_key = ?`, collectionID, key).Row().Scan(&prev.Created, &prev.Updated)
	if err != nil {
		return nil, shared.WrapError(err, "import batch")
	}
	return &prev, nil
}

func (r *Repository) FinishImportBatch(ctx context.Context, collectionID, key string, created, updated int64) error {
	q := `UPDATE app.import_batch SET created = ?, updated = ?, saved_at = now() WHERE collection_id = ? AND batch_key = ?`
	args := []any{created, updated, collectionID, key}
	if created == 0 && updated == 0 { // nothing saved: a later try of the same key must run
		q, args = `DELETE FROM app.import_batch WHERE collection_id = ? AND batch_key = ?`, []any{collectionID, key}
	}
	return shared.WrapError(r.db.WithContext(ctx).Exec(q, args...).Error, "import batch")
}

func (r *Repository) PruneImportBatches(ctx context.Context, before time.Time) error {
	return shared.WrapError(r.db.WithContext(ctx).Exec(`DELETE FROM app.import_batch WHERE saved_at < ?`, before).Error, "import batch")
}

func (r *Repository) LockUnique(ctx context.Context, collectionID string) error {
	err := r.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended('record-unique:' || ?, 0))", collectionID).Error
	return shared.WrapError(err, "record")
}

func (r *Repository) MaxNumber(ctx context.Context, collectionID, key string) (int64, error) {
	var n struct{ N int64 }
	// Trashed records count: a restore must not meet its number taken by a newer record.
	err := r.db.WithContext(ctx).Raw(`
		SELECT coalesce(max(floor((data ->> ?)::numeric)), 0)::bigint AS n FROM (
			SELECT data FROM app.record WHERE collection_id = ? AND jsonb_typeof(data -> ?) = 'number'
			UNION ALL
			SELECT data FROM app.record_trash WHERE collection_id = ? AND jsonb_typeof(data -> ?) = 'number'
		) held`, key, collectionID, key, collectionID, key).Take(&n).Error
	if err != nil {
		return 0, shared.WrapError(err, "record")
	}
	return n.N, nil
}

// Both operations use PostgreSQL's exact JSONB equality. Containment supplies
// an index condition for selective probes; equality also distinguishes nested
// subsets and different array orders.
const candidateValues = `WITH vals AS (
 SELECT (n-1)::int AS index, val, min((n-1)::int) OVER (PARTITION BY val) AS grp
 FROM jsonb_array_elements(?::jsonb) WITH ORDINALITY AS v(val,n)
 WHERE val <> 'null'::jsonb
) `

const valueOwners = ` FROM vals v LEFT JOIN app.record r ON r.collection_id = ?
 AND r.data->?::text = v.val AND r.data @> jsonb_build_object(?::text,v.val) `

func (r *Repository) Matching(ctx context.Context, collectionID, key string, values []json.RawMessage) ([]output.RecordMatch, error) {
	var rows []struct {
		Index, Grp, Version int
		ID                  string
		Data                []byte
	}
	err := r.db.WithContext(ctx).Raw(candidateValues+
		`SELECT DISTINCT ON (v.index) v.index, v.grp, coalesce(r.id::text,'') AS id, r.data, coalesce(r.version,0) AS version`+
		valueOwners+` AND v.index=v.grp ORDER BY v.index,r.created_at,r.id`, rawArray(values), collectionID, key, key).Scan(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "record")
	}
	out := make([]output.RecordMatch, len(rows))
	for i, v := range rows {
		out[i] = output.RecordMatch{Index: v.Index, Group: v.Grp, Record: domain.Record{ID: v.ID, Data: v.Data, Version: v.Version, CollectionID: collectionID}}
	}
	return out, nil
}

func (r *Repository) UniqueValues(ctx context.Context, collectionID, key string, values []json.RawMessage) ([]output.UniqueValue, error) {
	var out []output.UniqueValue
	err := r.db.WithContext(ctx).Raw(candidateValues+
		`SELECT v.index, v.grp AS "group", coalesce(r.id::text,'') AS owner`+
		valueOwners, rawArray(values), collectionID, key, key).Scan(&out).Error
	return out, shared.WrapError(err, "record")
}

// rawArray builds one driver parameter without copying every document to a string first.
// Empty entries stand for missing fields in a positional array.
func rawArray(values []json.RawMessage) string {
	size := 2 + len(values)
	for _, v := range values {
		size += max(len(v), 4)
	}
	var buf strings.Builder
	buf.Grow(size)
	buf.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			buf.WriteByte(',')
		}
		if len(v) == 0 {
			buf.WriteString("null")
		} else {
			buf.Write(v)
		}
	}
	buf.WriteByte(']')
	return buf.String()
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
