package tools

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type deliveryDTO struct {
	ID        string    `json:"id"`
	Event     string    `json:"event"`
	Status    *int      `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toDeliveryDTO(d domain.WebhookDelivery) deliveryDTO {
	return deliveryDTO{ID: d.ID, Event: d.Event, Status: d.Status, Error: d.Error, CreatedAt: d.CreatedAt}
}

// @Summary A webhook's newest calls (up to 50) and their answers; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]deliveryDTO}
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/webhooks/{hookId}/deliveries [get]
func (h *Handler) Deliveries(c fiber.Ctx) error {
	list, err := h.integrations.Deliveries(c.Context(), c.Params("id"), c.Params("hookId"))
	if err != nil {
		return fail(c, err)
	}
	out := make([]deliveryDTO, 0, len(list))
	for _, d := range list {
		out = append(out, toDeliveryDTO(d))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Send a webhook call again now, with the same body; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Param hookId path string true "Webhook ID"
// @Param deliveryId path string true "Delivery ID"
// @Security BearerAuth
// @Success 200 {object} deliveryDTO "the new call"
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/webhooks/{hookId}/deliveries/{deliveryId}/redeliver [post]
func (h *Handler) Redeliver(c fiber.Ctx) error {
	d, err := h.integrations.Redeliver(c.Context(), c.Params("id"), c.Params("hookId"), c.Params("deliveryId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toDeliveryDTO(*d))
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
}

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
	Action     string          `json:"action" example:"notify" enums:"set_field,notify"`
	Config     json.RawMessage `json:"config" swaggertype:"object"`
}

func (b automationBody) input() input.AutomationInput {
	return input.AutomationInput{Name: b.Name, Enabled: b.Enabled, Trigger: b.Trigger, Hour: b.Hour, Conditions: b.Conditions, Action: b.Action, Config: b.Config}
}

// @Summary A collection's automations; owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]automationDTO}
// @Failure 403 {object} mapper.errorBody
// @Router /v1/collections/{id}/automations [get]
func (h *Handler) Automations(c fiber.Ctx) error {
	list, err := h.integrations.Automations(c.Context(), c.Params("id"))
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
// @Description When a record is created or changed (or daily at hour), the records passing conditions get the action: set_field {"field","value"} or notify {"message"}. It acts as its maker.
// @Tags Integrations
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param body body automationBody true "Automation"
// @Security BearerAuth
// @Success 201 {object} automationDTO
// @Failure 400 {object} mapper.errorBody
// @Router /v1/collections/{id}/automations [post]
func (h *Handler) CreateAutomation(c fiber.Ctx) error {
	var body automationBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	a, err := h.integrations.CreateAutomation(c.Context(), c.Params("id"), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toAutomationDTO(*a))
}

// @Summary Change an automation; owners
// @Tags Integrations
// @Accept json
// @Produce json
// @Param id path string true "Collection ID"
// @Param autoId path string true "Automation ID"
// @Param body body automationBody true "Automation"
// @Security BearerAuth
// @Success 200 {object} automationDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/automations/{autoId} [put]
func (h *Handler) UpdateAutomation(c fiber.Ctx) error {
	var body automationBody
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	a, err := h.integrations.UpdateAutomation(c.Context(), c.Params("id"), c.Params("autoId"), body.input())
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toAutomationDTO(*a))
}

// @Summary Delete an automation; owners
// @Tags Integrations
// @Param id path string true "Collection ID"
// @Param autoId path string true "Automation ID"
// @Security BearerAuth
// @Success 204
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/automations/{autoId} [delete]
func (h *Handler) DeleteAutomation(c fiber.Ctx) error {
	if err := h.integrations.DeleteAutomation(c.Context(), c.Params("id"), c.Params("autoId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Run an automation now on every matching record (up to 100); owners
// @Tags Integrations
// @Produce json
// @Param id path string true "Collection ID"
// @Param autoId path string true "Automation ID"
// @Security BearerAuth
// @Success 200 {object} automationDTO "with the run's result"
// @Failure 404 {object} mapper.errorBody
// @Router /v1/collections/{id}/automations/{autoId}/run [post]
func (h *Handler) RunAutomation(c fiber.Ctx) error {
	a, err := h.integrations.RunAutomation(c.Context(), c.Params("id"), c.Params("autoId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toAutomationDTO(*a))
}
