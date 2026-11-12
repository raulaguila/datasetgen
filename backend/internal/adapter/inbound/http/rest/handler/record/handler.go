package record

import (
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
	svc input.RecordService
}

func New(svc input.RecordService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceRecords, domain.ActionRead)
	r.Get("/", read, h.List)
	r.Get("/:id", read, h.Get)
	r.Post("/", middleware.Require(authz, domain.ResourceRecords, domain.ActionCreate), h.Create)
	r.Patch("/:id", middleware.Require(authz, domain.ResourceRecords, domain.ActionUpdate), h.Update)
	r.Post("/delete", middleware.Require(authz, domain.ResourceRecords, domain.ActionDelete), h.Delete)
}

type recordDTO struct {
	ID           string          `json:"id"`
	CollectionID string          `json:"collection_id"`
	Data         json.RawMessage `json:"data" swaggertype:"object"`
	Version      int             `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
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

// @Summary List the records of one collection
// @Description Each item's data is a preview: only its top-level strings, numbers and booleans, strings cut to 120 characters. GET /v1/records/{id} returns the full data.
// @Tags Records
// @Produce json
// @Param collection_id query string true "Collection ID"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Text anywhere in the record JSON"
// @Param order query string false "Creation order" Enums(asc, desc) default(desc)
// @Security BearerAuth
// @Success 200 {object} object{items=[]recordDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/records [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.RecordFilter{
		CollectionID: c.Query("collection_id"),
		Page:         domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search:       c.Query("search"),
		Order:        c.Query("order"),
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
