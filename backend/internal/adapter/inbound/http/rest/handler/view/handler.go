// Package view serves a collection's saved views of the record list and its dashboard.
package view

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.ToolsService
}

func New(svc input.ToolsService) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts on /v1/collections/:owner/:slug.
func (h *Handler) Routes(at fiber.Router) {
	at.Get("/dashboard", h.Dashboard)
	at.Put("/dashboard", h.SaveDashboard)
	at.Get("/views", h.Views)
	at.Post("/views", h.CreateView)
	at.Delete("/views/:viewId", h.DeleteView)
}

type viewDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	Shared    bool      `json:"shared"`
	Mine      bool      `json:"mine"`
	CreatedAt time.Time `json:"created_at"`
} // @name View

func toViewDTO(v domain.SavedView) viewDTO {
	return viewDTO{ID: v.ID, Name: v.Name, Query: v.Query, Shared: v.Shared, Mine: v.Mine, CreatedAt: v.CreatedAt}
}

// @Summary The collection's dashboard widgets; members
// @Tags Views & dashboards
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{widgets=[]object}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID viewDashboard
// @Router /v1/collections/{owner}/{slug}/dashboard [get]
func (h *Handler) Dashboard(c fiber.Ctx) error {
	widgets, err := h.svc.Dashboard(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(fiber.Map{"widgets": widgets})
}

type dashboardDTO struct {
	Widgets json.RawMessage `json:"widgets" swaggertype:"array,object"`
} // @name Dashboard

// @Summary Replace the collection's dashboard widgets (kinds count, sum, avg, min, max, bars, list); owners
// @Tags Views & dashboards
// @Accept json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body dashboardDTO true "Widgets"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID viewSaveDashboard
// @Router /v1/collections/{owner}/{slug}/dashboard [put]
func (h *Handler) SaveDashboard(c fiber.Ctx) error {
	var body dashboardDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	if err := h.svc.SaveDashboard(c.Context(), middleware.CollectionID(c), body.Widgets); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary The caller's saved views of the record list and the shared ones; viewers
// @Tags Views & dashboards
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]viewDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID viewList
// @Router /v1/collections/{owner}/{slug}/views [get]
func (h *Handler) Views(c fiber.Ctx) error {
	items, err := h.svc.Views(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]viewDTO, 0, len(items))
	for _, v := range items {
		out = append(out, toViewDTO(v))
	}
	return c.JSON(fiber.Map{"items": out})
}

type createViewDTO struct {
	Name   string `json:"name"`
	Query  string `json:"query" example:"sort=data.nome&order=asc"`
	Shared bool   `json:"shared"`
} // @name ViewCreateInput

// @Summary Save a view of the record list; shared views need an editor
// @Tags Views & dashboards
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body createViewDTO true "View"
// @Security BearerAuth
// @Success 201 {object} viewDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID viewCreate
// @Router /v1/collections/{owner}/{slug}/views [post]
func (h *Handler) CreateView(c fiber.Ctx) error {
	var body createViewDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	v, err := h.svc.CreateView(c.Context(), middleware.CollectionID(c), body.Name, body.Query, body.Shared)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toViewDTO(*v))
}

// @Summary Delete one of the caller's saved views
// @Tags Views & dashboards
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param viewId path string true "View ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID viewDelete
// @Router /v1/collections/{owner}/{slug}/views/{viewId} [delete]
func (h *Handler) DeleteView(c fiber.Ctx) error {
	if err := h.svc.DeleteView(c.Context(), middleware.CollectionID(c), c.Params("viewId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
