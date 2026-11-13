package collection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.CollectionService
}

func New(svc input.CollectionService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router behind Bearer. Creating a collection is an RBAC
// permission; everything else follows the caller's role in the collection,
// which the service checks.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	r.Get("/", h.List)
	r.Get("/list", h.Options) // before /:id, which would swallow it
	r.Get("/:id", h.Get)
	r.Post("/", middleware.Require(authz, domain.ResourceCollections, domain.ActionCreate), h.Create)
	r.Patch("/:id", h.Update)
	r.Post("/delete", h.Delete)
	r.Get("/:id/members", h.Members)
	r.Post("/:id/members", h.AddMember)
	r.Patch("/:id/members/:userId", h.SetMemberRole)
	r.Delete("/:id/members/:userId", h.RemoveMember)
	r.Get("/:id/audit", h.AuditLog)
	r.Get("/:id/quality", h.Quality)
	r.Post("/:id/quality", h.StartQuality)
	r.Post("/:id/fix", h.Fix)
	r.Post("/:id/delete-invalid", h.DeleteInvalid)
	r.Post("/:id/duplicate", middleware.Require(authz, domain.ResourceCollections, domain.ActionCreate), h.Duplicate)
	r.Get("/:id/stats", h.Stats)
	r.Post("/:id/leave", h.Leave)
}

type collectionDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Schema      json.RawMessage `json:"schema,omitempty" swaggertype:"object"`
	RecordCount *int64          `json:"record_count,omitempty"`
	// Member is false for a platform admin who sees the collection without belonging to it; list only.
	Member *bool `json:"member,omitempty"`
	// Sent with details=true only.
	MemberCount  *int64     `json:"member_count,omitempty"`
	FieldCount   *int64     `json:"field_count,omitempty"`
	LastActivity *time.Time `json:"last_activity,omitempty"`
	// Role is the caller's role here: owner, editor or viewer.
	Role      string    `json:"role,omitempty" enums:"owner,editor,viewer"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type optionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createDTO struct {
	Name        string          `json:"name" example:"Wired"`
	Description string          `json:"description"`
	Icon        string          `json:"icon" example:"sparkles"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
}

type updateDTO struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Icon        *string         `json:"icon"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
	// Version is the version the client read; when sent, a newer row is a 409 conflict.
	Version *int `json:"version" example:"3"`
}

func toDTO(c domain.Collection) collectionDTO {
	return collectionDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, Icon: c.Icon, Schema: c.Schema, Role: string(c.Role),
		Version: c.Version, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// @Summary List collections, with their record counts and without schemas
// @Tags Collections
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Part of the name"
// @Param sort query string false "Sort field" Enums(name, created_at, updated_at) default(name)
// @Param order query string false "Sort direction" Enums(asc, desc) default(asc)
// @Param details query bool false "Add member_count, field_count and last_activity"
// @Security BearerAuth
// @Success 200 {object} object{items=[]collectionDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.CollectionFilter{
		Page:    domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search:  c.Query("search"),
		Sort:    c.Query("sort"),
		Order:   c.Query("order"),
		Details: fiber.Query[bool](c, "details"),
	}
	items, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]collectionDTO, 0, len(items))
	for _, item := range items {
		dto := toDTO(item)
		dto.RecordCount = &item.RecordCount
		dto.Member = &item.Member
		if filter.Details {
			dto.MemberCount, dto.FieldCount, dto.LastActivity = &item.MemberCount, &item.FieldCount, &item.LastActivity
		}
		out = append(out, dto)
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

// @Summary List collections as id and name, for pickers
// @Tags Collections
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]optionDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/list [get]
func (h *Handler) Options(c fiber.Ctx) error {
	items, err := h.svc.Options(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]optionDTO, 0, len(items))
	for _, item := range items {
		out = append(out, optionDTO{ID: item.ID, Name: item.Name})
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Get a collection with its schema
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} collectionDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	item, err := h.svc.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*item))
}

// @Summary Create a collection; schema is a JSON Schema (draft 2020-12) without external $ref
// @Tags Collections
// @Produce json
// @Accept json
// @Param body body createDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} collectionDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections [post]
func (h *Handler) Create(c fiber.Ctx) error {
	var body createDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	item, err := h.svc.Create(c.Context(), input.CreateCollectionCommand{
		Name: body.Name, Description: body.Description, Icon: body.Icon, Schema: body.Schema,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toDTO(*item))
}

// @Summary Update a collection; records are not checked again against a new schema
// @Tags Collections
// @Produce json
// @Accept json
// @Param body body updateDTO true "Request"
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} collectionDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id} [patch]
func (h *Handler) Update(c fiber.Ctx) error {
	var body updateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	item, err := h.svc.Update(c.Context(), input.UpdateCollectionCommand{
		ID: c.Params("id"), Name: body.Name, Description: body.Description, Icon: body.Icon, Schema: body.Schema, Version: body.Version,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*item))
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

// @Summary Delete collections; all or none, and only collections without records
// @Tags Collections
// @Accept json
// @Produce json
// @Param body body idsDTO true "Collection IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/delete [post]
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

type memberDTO struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status" enums:"pending,active,locked,disabled"`
	Role      string    `json:"role" enums:"owner,editor,viewer"`
	CreatedAt time.Time `json:"created_at"`
}

func toMemberDTO(m domain.CollectionMember) memberDTO {
	return memberDTO{UserID: m.UserID, Name: m.Name, Email: m.Email, Status: m.Status, Role: string(m.Role), CreatedAt: m.CreatedAt}
}

type addMemberDTO struct {
	Email string `json:"email" example:"ana@example.com"`
	// Name is needed only when the email has no account: it is then invited.
	Name string `json:"name" example:"Ana"`
	Role string `json:"role" enums:"owner,editor,viewer" example:"editor"`
}

type addedMemberDTO struct {
	Member  memberDTO `json:"member"`
	Invited bool      `json:"invited"`
	// ActivationToken is set only for an invite with manual delivery.
	ActivationToken string `json:"activation_token,omitempty"`
}

type roleDTO struct {
	Role string `json:"role" enums:"owner,editor,viewer" example:"viewer"`
}

// @Summary List a collection's members; owners only
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]memberDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/members [get]
func (h *Handler) Members(c fiber.Ctx) error {
	items, err := h.svc.Members(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]memberDTO, 0, len(items))
	for _, m := range items {
		out = append(out, toMemberDTO(m))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Add a member by email; an email without an account is invited and joins at once
// @Tags Collections
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body addMemberDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} addedMemberDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/members [post]
func (h *Handler) AddMember(c fiber.Ctx) error {
	var body addMemberDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	res, err := h.svc.AddMember(c.Context(), input.AddMemberCommand{
		CollectionID: c.Params("id"), Email: body.Email, Name: body.Name, Role: domain.MemberRole(body.Role),
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(addedMemberDTO{Member: toMemberDTO(res.Member), Invited: res.Invited, ActivationToken: res.ActivationToken})
}

// @Summary Change a member's role; a collection keeps at least one owner
// @Tags Collections
// @Accept json
// @Param id path string true "Collection ID"
// @Param userId path string true "User ID"
// @Param body body roleDTO true "Request"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/members/{userId} [patch]
func (h *Handler) SetMemberRole(c fiber.Ctx) error {
	var body roleDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	if err := h.svc.SetMemberRole(c.Context(), c.Params("id"), c.Params("userId"), domain.MemberRole(body.Role)); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Remove a member; a collection keeps at least one owner
// @Tags Collections
// @Param id path string true "Collection ID"
// @Param userId path string true "User ID"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/members/{userId} [delete]
func (h *Handler) RemoveMember(c fiber.Ctx) error {
	if err := h.svc.RemoveMember(c.Context(), c.Params("id"), c.Params("userId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// auditDTO mirrors the audit log's records (handler/audit).
type auditDTO struct {
	ID          string         `json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	ActorID     string         `json:"actor_id,omitempty"`
	ActorName   string         `json:"actor_name,omitempty"`
	ActorEmail  string         `json:"actor_email,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	Action      string         `json:"action" example:"record.update"`
	TargetID    string         `json:"target_id,omitempty"`
	TargetLabel string         `json:"target_label,omitempty"`
	TargetGone  bool           `json:"target_gone,omitempty"`
	Details     map[string]any `json:"details"`
}

// @Summary A collection's audit log: its changes, its records' and its members'; owners only
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param action query string false "Action prefix, such as record. or collection.member"
// @Param user_id query string false "The actor or the target"
// @Param from query string false "From this time on (RFC 3339)"
// @Param to query string false "Before this time (RFC 3339)"
// @Security BearerAuth
// @Success 200 {object} object{items=[]auditDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/audit [get]
func (h *Handler) AuditLog(c fiber.Ctx) error {
	filter, err := mapper.AuditFilter(c, domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	items, total, err := h.svc.AuditLog(c.Context(), c.Params("id"), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]auditDTO, 0, len(items))
	for _, r := range items {
		out = append(out, auditDTO(r))
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

type invalidRecordDTO struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data" swaggertype:"object"`
	UpdatedAt time.Time       `json:"updated_at"`
	Issues    []issueDTO      `json:"issues"`
}

// issueDTO mirrors schema.Issue, which swag does not read.
type issueDTO struct {
	Path    string `json:"path"`
	Keyword string `json:"keyword" example:"required"`
	Message string `json:"message"`
}

type qualityDTO struct {
	// Status is none (never ran), running, done or failed.
	Status string `json:"status" enums:"none,running,done,failed"`
	// Version is the collection version checked; a lower one than current_version is stale.
	Version        int                `json:"version"`
	CurrentVersion int                `json:"current_version"`
	Checked        int                `json:"checked"`
	InvalidCount   int                `json:"invalid_count"`
	Invalid        []invalidRecordDTO `json:"invalid"`
	// Truncated means the check stopped at 500 000 records.
	Truncated  bool       `json:"truncated"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func toQualityDTO(st *input.QualityState) (qualityDTO, error) {
	out := qualityDTO{Status: "none", CurrentVersion: st.CurrentVersion, Invalid: []invalidRecordDTO{}}
	if r := st.Run; r != nil {
		out.Status, out.Version, out.Checked, out.InvalidCount = r.Status, r.SchemaVersion, r.Checked, r.InvalidCount
		out.Truncated, out.Error, out.StartedAt, out.FinishedAt = r.Truncated, r.Error, &r.StartedAt, r.FinishedAt
		if len(r.Invalid) > 0 {
			if err := json.Unmarshal(r.Invalid, &out.Invalid); err != nil { // stored with the same keys
				return out, err
			}
		}
	}
	return out, nil
}

// @Summary The latest check of the records against the current schema; editors and owners
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} qualityDTO "invalid holds the first 100, with their issues"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/quality [get]
func (h *Handler) Quality(c fiber.Ctx) error {
	return h.writeQuality(c, h.svc.Quality)
}

// @Summary Start a check of the records in the background, unless one is running; editors and owners
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} qualityDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/quality [post]
func (h *Handler) StartQuality(c fiber.Ctx) error {
	return h.writeQuality(c, h.svc.StartQuality)
}

func (h *Handler) writeQuality(c fiber.Ctx, get func(context.Context, string) (*input.QualityState, error)) error {
	st, err := get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out, err := toQualityDTO(st)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(out)
}

type fixDTO struct {
	Op    string          `json:"op" enums:"set_default,rename,remove"`
	Field string          `json:"field"`
	To    string          `json:"to"`
	Value json.RawMessage `json:"value" swaggertype:"object"`
	// DryRun answers how many records would change, changing none.
	DryRun bool `json:"dry_run"`
}

type countDTO struct {
	Count int64 `json:"count"`
}

// @Summary Change the data of every record at once; editors and owners
// @Description set_default gives field the value in records without it; rename moves field to "to" where "to" is free; remove drops field. Records are not checked against the schema; each changed record keeps its previous version.
// @Tags Collections
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body fixDTO true "The change"
// @Security BearerAuth
// @Success 200 {object} countDTO "records changed, or that would change with dry_run"
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/fix [post]
func (h *Handler) Fix(c fiber.Ctx) error {
	var body fixDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	n, err := h.svc.Fix(c.Context(), c.Params("id"), domain.RecordFix{Op: body.Op, Field: body.Field, To: body.To, Value: body.Value, DryRun: body.DryRun})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(countDTO{Count: n})
}

// @Summary Delete the records the latest quality check found invalid; editors and owners
// @Description Records changed after that check started are kept. 409 when the check is missing, unfinished or of older fields.
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} countDTO "records deleted"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/delete-invalid [post]
func (h *Handler) DeleteInvalid(c fiber.Ctx) error {
	n, err := h.svc.DeleteInvalid(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(countDTO{Count: int64(n)})
}

type duplicateDTO struct {
	Name    string `json:"name" example:"Wired (copy)"`
	Records bool   `json:"records"`
}

// @Summary Copy a collection into a new one the caller owns
// @Description Copies name (or the one given), description, icon and fields, and the records when asked. Members are not copied.
// @Tags Collections
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body duplicateDTO true "Copy"
// @Security BearerAuth
// @Success 201 {object} collectionDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/duplicate [post]
func (h *Handler) Duplicate(c fiber.Ctx) error {
	var body duplicateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	created, err := h.svc.Duplicate(c.Context(), input.DuplicateCollectionCommand{ID: c.Params("id"), Name: body.Name, Records: body.Records})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := toDTO(*created)
	out.RecordCount = &created.RecordCount
	return c.Status(fiber.StatusCreated).JSON(out)
}

type statsDTO struct {
	Records    int64      `json:"records"`
	Members    int64      `json:"members"`
	LastUpdate *time.Time `json:"last_update,omitempty"`
	// PerDay holds the days with records created in the last 30, oldest first.
	PerDay       []dayDTO         `json:"per_day"`
	Contributors []contributorDTO `json:"contributors"`
}

type dayDTO struct {
	Day   string `json:"day" example:"2026-11-12"`
	Count int64  `json:"count"`
}

type contributorDTO struct {
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
	Edited  int64  `json:"edited"`
}

// @Summary A collection's overview: totals, records per day and top contributors of the last 30 days
// @Tags Collections
// @Produce json
// @Param id path string true "Collection ID"
// @Param tz query string false "IANA time zone the days are counted in" default(UTC)
// @Security BearerAuth
// @Success 200 {object} statsDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/stats [get]
func (h *Handler) Stats(c fiber.Ctx) error {
	st, err := h.svc.Stats(c.Context(), c.Params("id"), c.Query("tz"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := statsDTO{Records: st.Records, Members: st.Members, LastUpdate: st.LastUpdate, PerDay: make([]dayDTO, 0, len(st.PerDay)), Contributors: make([]contributorDTO, 0, len(st.Contributors))}
	for _, d := range st.PerDay {
		out.PerDay = append(out.PerDay, dayDTO(d))
	}
	for _, p := range st.Contributors {
		out.Contributors = append(out.Contributors, contributorDTO(p))
	}
	return c.JSON(out)
}

// @Summary Leave a collection; the last owner cannot
// @Tags Collections
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections/{id}/leave [post]
func (h *Handler) Leave(c fiber.Ctx) error {
	if err := h.svc.Leave(c.Context(), c.Params("id")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
