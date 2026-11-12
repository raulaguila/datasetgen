package product

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/pointer"
)

type Handler struct {
	svc input.ProductService
}

func New(svc input.ProductService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceProducts, domain.ActionRead)
	r.Get("/", read, h.List)
	r.Get("/list", read, h.Options) // before /:id, which would swallow it
	r.Get("/:id", read, h.Get)
	r.Post("/", middleware.Require(authz, domain.ResourceProducts, domain.ActionCreate), h.Create)
	r.Patch("/:id", middleware.Require(authz, domain.ResourceProducts, domain.ActionUpdate), h.Update)
	r.Post("/delete", middleware.Require(authz, domain.ResourceProducts, domain.ActionDelete), h.Delete)
}

type productDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Price     string    `json:"price"`
	Enabled   bool      `json:"enabled"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// productPaginationDTO documents both numbered and cursor pagination.
type productPaginationDTO struct {
	Page       int     `json:"page,omitempty"`
	Limit      int     `json:"limit"`
	TotalItems *int64  `json:"total_items,omitempty"`
	TotalPages *int64  `json:"total_pages,omitempty"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

type optionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createDTO struct {
	Name    string `json:"name"`
	Price   any    `json:"price" swaggertype:"string" example:"19.90"`
	Enabled *bool  `json:"enabled"`
}

type updateDTO struct {
	Name    *string `json:"name"`
	Price   any     `json:"price" swaggertype:"string" example:"19.90"`
	Enabled *bool   `json:"enabled"`
	// Version is the version the client read; when sent, a newer row is a 409 conflict.
	Version *int `json:"version" example:"3"`
}

func toDTO(p domain.Product) productDTO {
	return productDTO{
		ID: p.ID, Name: p.Name, Price: p.Price, Enabled: p.Enabled, Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func priceString(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", apperror.New(apperror.CodeInvalidInput, "price required").WithField("price")
	case string:
		return t, nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		return "", apperror.New(apperror.CodeInvalidInput, "price must be a number").WithField("price")
	}
}

// @Summary List products
// @Tags Products
// @Produce json
// @Param page query int false "Page" default(1)
// @Param cursor query string false "Use start for the first cursor page, then next_cursor; requires created_at desc and omits totals"
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param search query string false "Part of the name"
// @Param enabled query bool false "Only enabled (true) or disabled (false) products"
// @Param sort query string false "Sort field" Enums(name, price, created_at, updated_at) default(created_at)
// @Param order query string false "Sort direction" Enums(asc, desc) default(desc)
// @Security BearerAuth
// @Success 200 {object} object{items=[]productDTO,pagination=productPaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter := domain.ProductFilter{
		Page:   domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")),
		Search: c.Query("search"),
		Sort:   c.Query("sort"),
		Order:  c.Query("order"),
	}
	if raw := c.Query("enabled"); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "enabled must be true or false").WithField("enabled"))
		}
		filter.Enabled = &enabled
	}
	if raw := c.Query("cursor"); raw != "" {
		filter.Cursor = true
		if raw != "start" {
			cursor, err := decodeCursor(raw)
			if err != nil {
				return mapper.WriteError(c, err)
			}
			filter.After = cursor
		}
	}
	items, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]productDTO, 0, len(items))
	for _, p := range items {
		out = append(out, toDTO(p))
	}
	if filter.Cursor {
		pagination := productPaginationDTO{Limit: filter.Page.Limit}
		if len(items) == filter.Page.Limit {
			last := items[len(items)-1]
			raw, err := json.Marshal(domain.ProductCursor{CreatedAt: last.CreatedAt, ID: last.ID})
			if err != nil {
				return err
			}
			next := base64.RawURLEncoding.EncodeToString(raw)
			pagination.NextCursor = &next
		}
		return c.JSON(fiber.Map{"items": out, "pagination": pagination})
	}
	page := mapper.Pagination(filter.Page, total)
	return c.JSON(fiber.Map{"items": out, "pagination": productPaginationDTO{Page: page.Page, Limit: page.Limit, TotalItems: &page.TotalItems, TotalPages: &page.TotalPages}})
}

// @Summary List enabled products as id and name, for pickers
// @Tags Products
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]optionDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products/list [get]
func (h *Handler) Options(c fiber.Ctx) error {
	items, err := h.svc.Options(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]optionDTO, 0, len(items))
	for _, p := range items {
		out = append(out, optionDTO{ID: p.ID, Name: p.Name})
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Get a product
// @Tags Products
// @Produce json
// @Param id path string true "Product ID"
// @Security BearerAuth
// @Success 200 {object} productDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products/{id} [get]
func (h *Handler) Get(c fiber.Ctx) error {
	p, err := h.svc.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*p))
}

// @Summary Create a product
// @Tags Products
// @Produce json
// @Accept json
// @Param body body createDTO true "Request"
// @Security BearerAuth
// @Success 201 {object} productDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products [post]
func (h *Handler) Create(c fiber.Ctx) error {
	var body createDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	price, err := priceString(body.Price)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	p, err := h.svc.Create(c.Context(), input.CreateProductCommand{
		Name: body.Name, Price: price, Enabled: body.Enabled,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toDTO(*p))
}

// @Summary Update a product
// @Tags Products
// @Produce json
// @Accept json
// @Param body body updateDTO true "Request"
// @Param id path string true "Product ID"
// @Security BearerAuth
// @Success 200 {object} productDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 409 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products/{id} [patch]
func (h *Handler) Update(c fiber.Ctx) error {
	var body updateDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	cmd := input.UpdateProductCommand{ID: c.Params("id"), Name: body.Name, Enabled: body.Enabled, Version: body.Version}
	if body.Price != nil {
		price, err := priceString(body.Price)
		if err != nil {
			return mapper.WriteError(c, err)
		}
		cmd.Price = pointer.Of(price)
	}
	p, err := h.svc.Update(c.Context(), cmd)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(toDTO(*p))
}

type idsDTO struct {
	IDs []string `json:"ids"`
}

// @Summary Delete products; all or none
// @Tags Products
// @Accept json
// @Produce json
// @Param body body idsDTO true "Product IDs, 1 to 100"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/products/delete [post]
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

func decodeCursor(raw string) (*domain.ProductCursor, error) {
	invalid := apperror.New(apperror.CodeInvalidInput, "invalid cursor").WithField("cursor")
	if len(raw) > 512 {
		return nil, invalid
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalid
	}
	var cursor domain.ProductCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, invalid
	}
	return &cursor, nil
}
