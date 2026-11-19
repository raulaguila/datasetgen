// Package webhook serves a collection's webhooks and their deliveries.
package webhook

import (
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
	at.Get("/webhooks", h.Webhooks)
	at.Post("/webhooks", h.CreateWebhook)
	at.Patch("/webhooks/:hookId", h.UpdateWebhook)
	at.Delete("/webhooks/:hookId", h.DeleteWebhook)
	at.Post("/webhooks/:hookId/test", h.TestWebhook)
	at.Get("/webhooks/:hookId/deliveries", h.Deliveries)
	at.Post("/webhooks/:hookId/deliveries/:deliveryId/redeliver", h.Redeliver)
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

func badBody(c fiber.Ctx) error {
	return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
}

type webhookDTO struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Events     []string   `json:"events"`
	CreatedAt  time.Time  `json:"created_at"`
	LastStatus *int       `json:"last_status,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	LastAt     *time.Time `json:"last_at,omitempty"`
	// Failures counts failed calls in a row; at 10 the hook pauses (paused_at) until resumed.
	Failures int        `json:"failures"`
	PausedAt *time.Time `json:"paused_at,omitempty"`
} // @name Webhook

func toHookDTO(w domain.Webhook) webhookDTO {
	return webhookDTO{ID: w.ID, URL: w.URL, Events: w.Events, CreatedAt: w.CreatedAt, LastStatus: w.LastStatus, LastError: w.LastError, LastAt: w.LastAt, Failures: w.Failures, PausedAt: w.PausedAt}
}

// @Summary A collection's webhooks; owners
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]webhookDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookList
// @Router /v1/collections/{owner}/{slug}/webhooks [get]
func (h *Handler) Webhooks(c fiber.Ctx) error {
	hooks, err := h.svc.Webhooks(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return fail(c, err)
	}
	out := make([]webhookDTO, 0, len(hooks))
	for _, w := range hooks {
		out = append(out, toHookDTO(w))
	}
	return c.JSON(fiber.Map{"items": out})
}

type createHookDTO struct {
	URL    string   `json:"url" example:"https://example.com/hooks/dataset"`
	Events []string `json:"events" example:"record.created"`
} // @name WebhookCreateInput

type newHookDTO struct {
	webhookDTO
	// Secret signs every call (X-Dataset-Signature: t=<unix>,v1=<HMAC-SHA256 of "<t>.<body>">); shown once.
	Secret string `json:"secret"`
} // @name NewWebhook

// @Summary Add a webhook; owners
// @Description Events: record.created, record.updated, record.deleted, records.imported. Calls are POSTs with a JSON body, retried with backoff on failure.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body createHookDTO true "Webhook"
// @Security BearerAuth
// @Success 201 {object} newHookDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookCreate
// @Router /v1/collections/{owner}/{slug}/webhooks [post]
func (h *Handler) CreateWebhook(c fiber.Ctx) error {
	var body createHookDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	w, err := h.svc.CreateWebhook(c.Context(), middleware.CollectionID(c), body.URL, body.Events)
	if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(newHookDTO{webhookDTO: toHookDTO(w.Webhook), Secret: w.Secret})
}

type updateHookDTO struct {
	URL    string   `json:"url" example:"https://example.com/hooks/dataset"`
	Events []string `json:"events" example:"record.created"`
	// Enabled false pauses the hook; true resumes it and clears its failures.
	Enabled bool `json:"enabled" example:"true"`
} // @name WebhookUpdateInput

// @Summary Change, pause or resume a webhook; owners
// @Description Keeps the signing secret. A hook pauses by itself after 10 failed calls in a row; send enabled true to resume it.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Param body body updateHookDTO true "Webhook"
// @Security BearerAuth
// @Success 200 {object} webhookDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookUpdate
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId} [patch]
func (h *Handler) UpdateWebhook(c fiber.Ctx) error {
	var body updateHookDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	w, err := h.svc.UpdateWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId"), body.URL, body.Events, body.Enabled)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toHookDTO(*w))
}

// @Summary Delete a webhook; owners
// @Tags Webhooks
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookDelete
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId} [delete]
func (h *Handler) DeleteWebhook(c fiber.Ctx) error {
	if err := h.svc.DeleteWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId")); err != nil {
		return fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary Send a ping to a webhook now; owners
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 200 {object} webhookDTO "with the answer"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookTest
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId}/test [post]
func (h *Handler) TestWebhook(c fiber.Ctx) error {
	w, err := h.svc.TestWebhook(c.Context(), middleware.CollectionID(c), c.Params("hookId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toHookDTO(*w))
}

type deliveryDTO struct {
	ID        string    `json:"id"`
	Event     string    `json:"event"`
	Status    *int      `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
} // @name WebhookDelivery

func toDeliveryDTO(d domain.WebhookDelivery) deliveryDTO {
	return deliveryDTO{ID: d.ID, Event: d.Event, Status: d.Status, Error: d.Error, CreatedAt: d.CreatedAt}
}

// @Summary A webhook's newest calls (up to 50) and their answers; owners
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Security BearerAuth
// @Success 200 {object} object{items=[]deliveryDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookDeliveries
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId}/deliveries [get]
func (h *Handler) Deliveries(c fiber.Ctx) error {
	list, err := h.svc.Deliveries(c.Context(), middleware.CollectionID(c), c.Params("hookId"))
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
// @Tags Webhooks
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param hookId path string true "Webhook ID"
// @Param deliveryId path string true "Delivery ID"
// @Security BearerAuth
// @Success 200 {object} deliveryDTO "the new call"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID webhookRedeliver
// @Router /v1/collections/{owner}/{slug}/webhooks/{hookId}/deliveries/{deliveryId}/redeliver [post]
func (h *Handler) Redeliver(c fiber.Ctx) error {
	d, err := h.svc.Redeliver(c.Context(), middleware.CollectionID(c), c.Params("hookId"), c.Params("deliveryId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(toDeliveryDTO(*d))
}
