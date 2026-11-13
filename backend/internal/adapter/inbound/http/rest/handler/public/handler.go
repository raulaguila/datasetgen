// Package public serves share links: a collection's form or read-only list to
// anyone holding the link, without a session.
package public

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/schema"
)

type Handler struct {
	shares      input.IntegrationService
	collections input.CollectionService
	records     input.RecordService
}

func New(shares input.IntegrationService, collections input.CollectionService, records input.RecordService) *Handler {
	return &Handler{shares: shares, collections: collections, records: records}
}

const (
	localShare      = "share.kind"
	localCollection = "share.collection"
)

// Routes mounts on /v1/public; submit limits form posts (they write records).
func (h *Handler) Routes(r fiber.Router, submit fiber.Handler) {
	g := r.Group("/:token", h.link)
	g.Get("/", h.Get)
	g.Get("/records", h.Records)
	g.Post("/records", submit, h.Submit)
}

// link acts as the link's maker, capped like an API key to the one collection
// (read only for a list, editor for a form); the handlers below expose only
// what the link's kind allows.
func (h *Handler) link(c fiber.Ctx) error {
	g, err := h.shares.AuthenticateShare(c.Context(), c.Params("token"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	ctx := reqctx.WithActor(c.Context(), g.UserID)
	c.SetContext(reqctx.WithAPIKey(ctx, reqctx.APIKey{ID: g.KeyID, Name: g.Name, CollectionID: g.CollectionID, CanWrite: g.CanWrite}))
	c.Locals(localShare, g.Share)
	c.Locals(localCollection, g.CollectionID)
	return c.Next()
}

func kind(c fiber.Ctx) string       { s, _ := c.Locals(localShare).(string); return s }
func collection(c fiber.Ctx) string { s, _ := c.Locals(localCollection).(string); return s }

var notFound = apperror.New(apperror.CodeNotFound, "link not found")

type linkDTO struct {
	Kind        string          `json:"kind" example:"form"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Schema      json.RawMessage `json:"schema" swaggertype:"object"`
	// Searchable is false when the collection has private fields: search would reveal their values.
	Searchable bool `json:"searchable"`
}

// @Summary What a share link opens: a form or a read-only list
// @Tags Public
// @Produce json
// @Param token path string true "Link token"
// @Success 200 {object} linkDTO
// @Failure 404 {object} mapper.errorBody
// @Router /v1/public/{token} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	col, err := h.collections.Get(c.Context(), collection(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	hidden := schema.PrivateKeys(col.Schema)
	return c.JSON(linkDTO{Kind: kind(c), Name: col.Name, Description: col.Description, Icon: col.Icon, Schema: schema.HideSchema(col.Schema, hidden), Searchable: len(hidden) == 0})
}

// private lists the collection's fields kept off share links.
func (h *Handler) private(c fiber.Ctx) ([]string, error) {
	col, err := h.collections.Get(c.Context(), collection(c))
	if err != nil {
		return nil, err
	}
	return schema.PrivateKeys(col.Schema), nil
}

type recordDTO struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data" swaggertype:"object"`
	CreatedAt time.Time       `json:"created_at"`
}

// @Summary The records behind a read-only link, newest first
// @Description Each item's data is a preview, as in GET /v1/records.
// @Tags Public
// @Produce json
// @Param token path string true "Link token"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Text anywhere in the record JSON"
// @Success 200 {object} object{items=[]recordDTO,pagination=mapper.PaginationDTO}
// @Failure 404 {object} mapper.errorBody
// @Router /v1/public/{token}/records [get]
func (h *Handler) Records(c fiber.Ctx) error {
	if kind(c) != domain.ShareView {
		return mapper.WriteError(c, notFound)
	}
	hidden, err := h.private(c)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	// Search reads the whole record: with private fields it would tell which records hold a hidden value.
	if len(hidden) > 0 && c.Query("search") != "" {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "this link does not search: the collection has private fields").WithField("search"))
	}
	filter := domain.RecordFilter{
		CollectionID: collection(c),
		Page:         domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search:       c.Query("search"),
		Sort:         "created_at",
		Order:        "desc",
	}
	items, total, err := h.records.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]recordDTO, 0, len(items))
	for _, r := range items {
		out = append(out, recordDTO{ID: r.ID, Data: schema.HideData(r.Data, hidden), CreatedAt: r.CreatedAt})
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

type submitDTO struct {
	Data json.RawMessage `json:"data" swaggertype:"object"`
}

// @Summary Send a form: adds one record, checked against the collection's fields
// @Tags Public
// @Accept json
// @Produce json
// @Param token path string true "Link token"
// @Param body body submitDTO true "Record"
// @Success 201 {object} object{id=string}
// @Failure 400 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/public/{token}/records [post]
func (h *Handler) Submit(c fiber.Ctx) error {
	if kind(c) != domain.ShareForm {
		return mapper.WriteError(c, notFound)
	}
	var body submitDTO
	if err := c.Bind().Body(&body); err != nil || len(body.Data) == 0 {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	hidden, err := h.private(c)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	if schema.HasAny(body.Data, hidden) {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "the form does not take private fields").WithField("data"))
	}
	rec, err := h.records.Create(c.Context(), input.CreateRecordCommand{CollectionID: collection(c), Data: body.Data})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	// Only the id: the form's sender reads nothing back.
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": rec.ID})
}
