package collection

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
	svc input.CollectionService
}

func New(svc input.CollectionService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceCollections, domain.ActionRead)
	r.Get("/", read, h.List)
	r.Get("/list", read, h.Options) // before /:id, which would swallow it
	r.Get("/:id", read, h.Get)
	r.Post("/", middleware.Require(authz, domain.ResourceCollections, domain.ActionCreate), h.Create)
	r.Patch("/:id", middleware.Require(authz, domain.ResourceCollections, domain.ActionUpdate), h.Update)
	r.Post("/delete", middleware.Require(authz, domain.ResourceCollections, domain.ActionDelete), h.Delete)
}

type collectionDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema,omitempty" swaggertype:"object"`
	RecordCount *int64          `json:"record_count,omitempty"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type optionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createDTO struct {
	Name        string          `json:"name" example:"Wired"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
}

type updateDTO struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
	// Version is the version the client read; when sent, a newer row is a 409 conflict.
	Version *int `json:"version" example:"3"`
}

func toDTO(c domain.Collection) collectionDTO {
	return collectionDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, Schema: c.Schema,
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
// @Security BearerAuth
// @Success 200 {object} object{items=[]collectionDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/collections [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.CollectionFilter{
		Page:   domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search: c.Query("search"),
		Sort:   c.Query("sort"),
		Order:  c.Query("order"),
	}
	items, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]collectionDTO, 0, len(items))
	for _, item := range items {
		dto := toDTO(item)
		dto.RecordCount = &item.RecordCount
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
		Name: body.Name, Description: body.Description, Schema: body.Schema,
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
		ID: c.Params("id"), Name: body.Name, Description: body.Description, Schema: body.Schema, Version: body.Version,
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
