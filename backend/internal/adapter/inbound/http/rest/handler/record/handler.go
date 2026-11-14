package record

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.RecordService
}

func New(svc input.RecordService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
// Routes expects a router behind Bearer; the service checks the caller's role
// in the record's collection (viewer reads, editor writes).
func (h *Handler) Routes(r fiber.Router) {
	r.Get("/", h.List)
	r.Get("/export", h.Export) // before /:id, which would swallow it
	r.Get("/aggregate", h.Aggregate)
	r.Get("/stats", h.Stats)
	r.Post("/import", h.Import)
	r.Get("/:id", h.Get)
	r.Get("/:id/versions", h.Versions)
	r.Get("/:id/links", h.Links)
	r.Get("/:id/comments", h.Comments)
	r.Post("/:id/comments", h.AddComment)
	r.Delete("/:id/comments/:commentId", h.DeleteComment)
	r.Post("/:id/restore", h.Restore)
	r.Post("/", h.Create)
	r.Patch("/:id", h.Update)
	r.Post("/delete", h.Delete)
}

type recordDTO struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Data         json.RawMessage `json:"data" swaggertype:"object"`
	Version      int             `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type linksDTO struct {
	CollectionID   string          `json:"collection_id"`
	CollectionName string          `json:"collection_name"`
	Field          string          `json:"field"`
	Sub            string          `json:"sub,omitempty"` // the reference subfield of each item when Field is a list of objects
	Schema         json.RawMessage `json:"schema" swaggertype:"object"`
	Total          int64           `json:"total"`
	Records        []recordDTO     `json:"records"`
}

// @Summary The records pointing at this one, per collection and reference field
// @Description Up to 10 newest records (previews) per field, in collections the caller can read.
// @Tags Records
// @Produce json
// @Param id path string true "Record ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]linksDTO}
// @Failure 404 {object} mapper.errorBody
// @Router /v1/records/{id}/links [get]
func (h *Handler) Links(c fiber.Ctx) error {
	links, err := h.svc.Links(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]linksDTO, 0, len(links))
	for _, l := range links {
		recs := make([]recordDTO, 0, len(l.Records))
		for _, r := range l.Records {
			recs = append(recs, toDTO(r))
		}
		out = append(out, linksDTO{CollectionID: l.CollectionID, CollectionName: l.Name, Field: l.Field, Sub: l.Sub, Schema: l.Schema, Total: l.Total, Records: recs})
	}
	return c.JSON(fiber.Map{"items": out})
}

type commentDTO struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id,omitempty"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

func toCommentDTO(c domain.RecordComment) commentDTO {
	return commentDTO{ID: c.ID, AuthorID: c.UserID, AuthorName: c.AuthorName, Body: c.Body, CreatedAt: c.CreatedAt}
}

// @Summary A record's comments, oldest first (up to 200)
// @Tags Records
// @Produce json
// @Param id path string true "Record ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]commentDTO}
// @Failure 404 {object} mapper.errorBody
// @Router /v1/records/{id}/comments [get]
func (h *Handler) Comments(c fiber.Ctx) error {
	list, err := h.svc.Comments(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]commentDTO, 0, len(list))
	for _, x := range list {
		out = append(out, toCommentDTO(x))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Comment on a record; any member
// @Tags Records
// @Accept json
// @Produce json
// @Param id path string true "Record ID"
// @Param body body object{body=string} true "Comment"
// @Security BearerAuth
// @Success 201 {object} commentDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/records/{id}/comments [post]
func (h *Handler) AddComment(c fiber.Ctx) error {
	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	x, err := h.svc.AddComment(c.Context(), c.Params("id"), body.Body)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toCommentDTO(*x))
}

// @Summary Delete a comment; its author or an owner
// @Tags Records
// @Param id path string true "Record ID"
// @Param commentId path string true "Comment ID"
// @Security BearerAuth
// @Success 204
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/records/{id}/comments/{commentId} [delete]
func (h *Handler) DeleteComment(c fiber.Ctx) error {
	if err := h.svc.DeleteComment(c.Context(), c.Params("id"), c.Params("commentId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type createDTO struct {
	CollectionID string          `json:"collection_id"`
	Data         json.RawMessage `json:"data" swaggertype:"object"`
}

type updateDTO struct {
	Data json.RawMessage `json:"data" swaggertype:"object"`
	// Version is the version the client read; when sent, a newer row is a 409 conflict.
	Version *int `json:"version" example:"3"`
}

func toDTO(r domain.Record) recordDTO {
	return recordDTO{
		ID: r.ID, CollectionID: r.CollectionID, Data: r.Data,
		Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// listFilter reads what selects records from the query string.
func listFilter(c fiber.Ctx) (domain.RecordFilter, error) {
	filter := domain.RecordFilter{
		CollectionID: c.Query("collection_id"),
		Page:         domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search:       c.Query("search"),
		Sort:         c.Query("sort"),
		Order:        c.Query("order"),
	}
	if f := c.Query("filter"); f != "" {
		filter.Match = json.RawMessage(f) // the service checks its shape
	}
	if ids := c.Query("ids"); ids != "" {
		filter.IDs = strings.Split(ids, ",")
	}
	if w := c.Query("where"); w != "" {
		filter.Where = &domain.Where{}
		if err := json.Unmarshal([]byte(w), filter.Where); err != nil {
			return filter, apperror.New(apperror.CodeInvalidInput, `where must be {"any": bool, "rules": [{"field", "op", "value"}]}`).WithField("where")
		}
	}
	return filter, nil
}

// @Summary List the records of one collection
// @Description Each item's data is a preview: only its top-level strings, numbers, booleans and lists of strings (first 10), strings cut to 120 characters. GET /v1/records/{id} returns the full data.
// @Tags Records
// @Produce json
// @Param collection_id query string true "Collection ID"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Text anywhere in the record JSON"
// @Param sort query string false "Sort field" default(created_at)
// @Param order query string false "Sort order" Enums(asc, desc) default(desc)
// @Param where query string false "Rules as JSON: {\"any\": false, \"rules\": [{\"field\": \"price\", \"op\": \"gt\", \"value\": 10}]}; ops eq, neq, contains, gt, gte, lt, lte, empty, not_empty, in, has; any joins them by OR; groups (one level) bracket rules: {\"any\": true, \"groups\": [{\"rules\": [...]}, {\"rules\": [...]}]}"
// @Param filter query string false "JSON object of top-level fields and exact values, up to 10, such as {\"category\":\"trigger\"}"
// @Security BearerAuth
// @Success 200 {object} object{items=[]recordDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter, err := listFilter(c)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	items, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]recordDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toDTO(item))
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

// @Summary Get a record
// @Tags Records
// @Produce json
// @Param id path string true "Record ID"
// @Security BearerAuth
// @Success 200 {object} recordDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	item, err := h.svc.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*item))
}

// @Summary Create a record; data must match the collection schema (VALIDATION_FAILED lists the issues in details)
// @Tags Records
// @Produce json
// @Accept json
// @Param body body createDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} recordDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records [post]
func (h *Handler) Create(c fiber.Ctx) error {
	var body createDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	item, err := h.svc.Create(c.Context(), input.CreateRecordCommand{CollectionID: body.CollectionID, Data: body.Data})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toDTO(*item))
}

// @Summary Replace the data of a record; it must match the collection schema
// @Tags Records
// @Produce json
// @Accept json
// @Param body body updateDTO true "Request"
// @Param id path string true "Record ID"
// @Security BearerAuth
// @Success 200 {object} recordDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/{id} [patch]
func (h *Handler) Update(c fiber.Ctx) error {
	var body updateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	item, err := h.svc.Update(c.Context(), input.UpdateRecordCommand{ID: c.Params("id"), Data: body.Data, Version: body.Version})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*item))
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

// @Summary Delete records; all or none
// @Tags Records
// @Accept json
// @Produce json
// @Param body body idsDTO true "Record IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records/delete [post]
func (h *Handler) Delete(c fiber.Ctx) error {
	var body idsDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.DeleteMany(c.Context(), body.IDs); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type groupDTO struct {
	Key   json.RawMessage    `json:"key" swaggertype:"object"`
	Count int64              `json:"count"`
	Sums  map[string]float64 `json:"sums"`
	Avgs  map[string]float64 `json:"avgs"`
	Mins  map[string]float64 `json:"mins"`
	Maxs  map[string]float64 `json:"maxs"`
}

// @Summary Count the selected records, by a field (group) or all together, with the sum, average, least and greatest of numeric fields (sum, comma separated, up to 5)
// @Tags Records
// @Produce json
// @Param collection_id query string true "Collection ID"
// @Param group query string false "Field to group by (up to 100 groups, largest first)"
// @Param sum query string false "Numeric fields, comma separated"
// @Param where query string false "Rules as JSON, as in the list"
// @Security BearerAuth
// @Success 200 {object} object{items=[]groupDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Router /v1/records/aggregate [get]
func (h *Handler) Aggregate(c fiber.Ctx) error {
	filter, err := listFilter(c)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	var numeric []string
	if f := c.Query("sum"); f != "" {
		numeric = strings.Split(f, ",")
	}
	groups, err := h.svc.Aggregate(c.Context(), filter, c.Query("group"), numeric)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]groupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, groupDTO{Key: g.Key, Count: g.Count, Sums: g.Sums, Avgs: g.Avgs, Mins: g.Mins, Maxs: g.Maxs})
	}
	return c.JSON(fiber.Map{"items": out})
}

type dateCountDTO struct {
	At    string `json:"at"`
	Count int64  `json:"count"`
}

type valueCountDTO struct {
	Key   json.RawMessage `json:"key" swaggertype:"object"`
	Count int64           `json:"count"`
}

type fieldStatsDTO struct {
	Filled   *int64          `json:"filled,omitempty"`
	Min      *float64        `json:"min,omitempty"`
	Max      *float64        `json:"max,omitempty"`
	Avg      *float64        `json:"avg,omitempty"`
	Sum      *float64        `json:"sum,omitempty"`
	First    *string         `json:"first,omitempty"`
	Last     *string         `json:"last,omitempty"`
	Distinct *int64          `json:"distinct,omitempty"`
	Top      []valueCountDTO `json:"top,omitempty"`
	Series   []dateCountDTO  `json:"series,omitempty"`
}

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// Stats godoc
// @Summary Profile records field by field
// @Description How many of the selected records fill each field, the range of numbers and dates, a count per date bucket and the commonest values. Viewer or above.
// @Tags records
// @Produce json
// @Param collection_id query string true "Collection ID"
// @Param where query string false "Rules as JSON, as in the list"
// @Param search query string false "Text search, as in the list"
// @Param filled query string false "Fields to count filled, comma separated"
// @Param numbers query string false "Numeric fields: min, max, avg, sum"
// @Param dates query string false "Date fields: first, last and a series"
// @Param values query string false "Fields to list the commonest values of (8 each)"
// @Param bucket query string false "Series bucket: day, week, month (default) or year"
// @Security BearerAuth
// @Success 200 {object} object{total=int,fields=map[string]fieldStatsDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Router /v1/records/stats [get]
func (h *Handler) Stats(c fiber.Ctx) error {
	filter, err := listFilter(c)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	q := domain.StatsQuery{Filled: splitList(c.Query("filled")), Numbers: splitList(c.Query("numbers")), Dates: splitList(c.Query("dates")), Values: splitList(c.Query("values")), Bucket: c.Query("bucket")}
	st, err := h.svc.Stats(c.Context(), filter, q)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	fields := make(map[string]fieldStatsDTO, len(st.Fields))
	for k, f := range st.Fields {
		d := fieldStatsDTO{Min: f.Min, Max: f.Max, Avg: f.Avg, Sum: f.Sum, First: f.First, Last: f.Last}
		if slices.Contains(q.Filled, k) {
			d.Filled = &f.Filled
		}
		if slices.Contains(q.Values, k) {
			d.Distinct, d.Top = &f.Distinct, make([]valueCountDTO, 0, len(f.Top))
			for _, g := range f.Top {
				d.Top = append(d.Top, valueCountDTO{Key: g.Key, Count: g.Count})
			}
		}
		for _, p := range f.Series {
			d.Series = append(d.Series, dateCountDTO{At: p.At, Count: p.Count})
		}
		fields[k] = d
	}
	return c.JSON(fiber.Map{"total": st.Total, "fields": fields})
}
