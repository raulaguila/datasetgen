// Package share serves a collection's share links: a public form or a read-only list.
package share

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.IntegrationService
}

func New(svc input.IntegrationService) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts on /v1/collections/:owner/:slug.
func (h *Handler) Routes(at fiber.Router) {
	at.Get("/shares", h.Shares)
	at.Post("/shares", h.CreateShare)
	at.Delete("/shares/:shareId", h.DeleteShare)
}

type shareDTO struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind" example:"form"`
	Token      string     `json:"token"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
} // @name ShareLink

func toShareDTO(s domain.Share) shareDTO {
	return shareDTO{ID: s.ID, Kind: s.Kind, Token: s.Token, CreatedAt: s.CreatedAt, LastUsedAt: s.LastUsedAt, ExpiresAt: s.ExpiresAt}
}

// @Summary A collection's share links; owners
// @Tags Share links
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Security BearerAuth
// @Success 200 {object} object{items=[]shareDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID shareList
// @Router /v1/collections/{owner}/{slug}/shares [get]
func (h *Handler) Shares(c fiber.Ctx) error {
	shares, err := h.svc.Shares(c.Context(), middleware.CollectionID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]shareDTO, 0, len(shares))
	for _, s := range shares {
		out = append(out, toShareDTO(s))
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary Make a share link: a public form (kind form) or a read-only list (kind view); owners
// @Tags Share links
// @Accept json
// @Produce json
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param body body object{kind=string,expires_in_days=int} true "Link; expires_in_days 1 to 365, or 0 for no end"
// @Security BearerAuth
// @Success 201 {object} shareDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID shareCreate
// @Router /v1/collections/{owner}/{slug}/shares [post]
func (h *Handler) CreateShare(c fiber.Ctx) error {
	var body struct {
		Kind          string `json:"kind"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return mapper.BadBody(c)
	}
	s, err := h.svc.CreateShare(c.Context(), middleware.CollectionID(c), body.Kind, body.ExpiresInDays)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(toShareDTO(*s))
}

// @Summary Delete a share link; owners
// @Tags Share links
// @Param owner path string true "Owner's username"
// @Param slug path string true "Collection slug"
// @Param shareId path string true "Link ID"
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID shareDelete
// @Router /v1/collections/{owner}/{slug}/shares/{shareId} [delete]
func (h *Handler) DeleteShare(c fiber.Ctx) error {
	if err := h.svc.DeleteShare(c.Context(), middleware.CollectionID(c), c.Params("shareId")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
