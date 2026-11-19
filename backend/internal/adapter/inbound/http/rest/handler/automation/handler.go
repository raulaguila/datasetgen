// Package automation serves a collection's automations.
package automation

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
	svc input.IntegrationService
}

func New(svc input.IntegrationService) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts on /v1/collections/:owner/:slug.
func (h *Handler) Routes(at fiber.Router) {
	at.Get("/automations", h.Automations)
	at.Post("/automations", h.CreateAutomation)
	at.Put("/automations/:autoId", h.UpdateAutomation)
	at.Delete("/automations/:autoId", h.DeleteAutomation)
	at.Post("/automations/:autoId/run", h.RunAutomation)
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

func badBody(c fiber.Ctx) error {
	return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
}

type automationDTO struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Enabled    bool            `json:"enabled"`
	Trigger    string          `json:"trigger" example:"record.created"`
	Hour       *int            `json:"hour,omitempty"`
	Conditions domain.Where    `json:"conditions" swaggertype:"object"`
	Action     string          `json:"action" example:"notify"`
	Config     json.RawMessage `json:"config" swaggertype:"object"`
	CreatedAt  time.Time       `json:"created_at"`
	LastRunAt  *time.Time      `json:"last_run_at,omitempty"`
	LastError  string          `json:"last_error,omitempty"`
	Runs       int             `json:"runs"`
} // @name Automation

func toAutomationDTO(a domain.Automation) automationDTO {
	return automationDTO{ID: a.ID, Name: a.Name, Enabled: a.Enabled, Trigger: a.Trigger, Hour: a.Hour, Conditions: a.Conditions,
		Action: a.Action, Config: a.Config, CreatedAt: a.CreatedAt, LastRunAt: a.LastRunAt, LastError: a.LastError, Runs: a.Runs}
}

type automationBody struct {
	Name       string          `json:"name" example:"Avisar urgentes"`
	Enabled    bool            `json:"enabled"`
	Trigger    string          `json:"trigger" example:"record.created" enums:"record.created,record.updated,daily"`
	Hour       *int            `json:"hour,omitempty" example:"8"`
	Conditions domain.Where    `json:"conditions" swaggertype:"object"`
	Action     string          `json:"action" example:"notify" enums:"set_field,notify,email,webhook,create_record"`
	Config     json.RawMessage `json:"config" swaggertype:"object"`
} // @name AutomationInput

func (b automationBody) input() input.AutomationInput {
	return input.AutomationInput{Name: b.Name, Enabled: b.Enabled, Trigger: b.Trigger, Hour: b.Hour, Conditions: b.Conditions, Action: b.Action, Config: b.Config}
}

// @Summary A collection's automations; owners
// @Tags Automations
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]automationDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID automationList
// @Router /v1/collections/{owner}/{slug}/automations [get]
func (h *Handler) Automations(c fiber.Ctx) error {
	list, err := h.svc.Automations(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]automationDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAutomationDTO(a))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Make an automation; owners
// @Description When a record is created or changed (or daily at hour), the records passing conditions get the action: set_field {"field","value"}, notify {"message"}, email {"to":"me"|"members","subject","message"}, webhook {"webhook_id"} or create_record {"collection_id","data"} (a text value naming a field, or id, in double curly braces copies it from the matching record). It acts as its maker.
// @Tags Automations
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body automationBody true "Automation"
// @Security BearerAuth
// @Success 201 {object} automationDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID automationCreate
// @Router /v1/collections/{owner}/{slug}/automations [post]
func (h *Handler) CreateAutomation(c fiber.Ctx) error {
	var body automationBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	a, err := h.svc.CreateAutomation(c.Context(), middleware.CollectionID(c), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toAutomationDTO(*a))
}

// @Summary Change an automation; owners
// @Tags Automations
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param autoId path string true "Automation ID"
// @Param body body automationBody true "Automation"
// @Security BearerAuth
// @Success 200 {object} automationDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID automationUpdate
// @Router /v1/collections/{owner}/{slug}/automations/{autoId} [put]
func (h *Handler) UpdateAutomation(c fiber.Ctx) error {
	var body automationBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	a, err := h.svc.UpdateAutomation(c.Context(), middleware.CollectionID(c), c.Params("autoId"), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toAutomationDTO(*a))
}

// @Summary Delete an automation; owners
// @Tags Automations
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param autoId path string true "Automation ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID automationDelete
// @Router /v1/collections/{owner}/{slug}/automations/{autoId} [delete]
func (h *Handler) DeleteAutomation(c fiber.Ctx) error {
	if err := h.svc.DeleteAutomation(c.Context(), middleware.CollectionID(c), c.Params("autoId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Run an automation now on every matching record (up to 100); owners
// @Tags Automations
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param autoId path string true "Automation ID"
// @Security BearerAuth
// @Success 200 {object} automationDTO "with the run's result"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID automationRun
// @Router /v1/collections/{owner}/{slug}/automations/{autoId}/run [post]
func (h *Handler) RunAutomation(c fiber.Ctx) error {
	a, err := h.svc.RunAutomation(c.Context(), middleware.CollectionID(c), c.Params("autoId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toAutomationDTO(*a))
}
