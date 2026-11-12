package session

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.AccountService
}

func New(svc input.AccountService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router) {
	r.Get("/", h.List)
	r.Delete("/others", h.RevokeOthers) // before /:id, which would swallow it
	r.Delete("/:id", h.Revoke)
}

type sessionDTO struct {
	ID         string    `json:"id"`
	IPAddress  *string   `json:"ip_address"`
	UserAgent  *string   `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

// @Summary List the live sessions of the current user
// @Tags Sessions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]sessionDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/sessions [get]
func (h *Handler) List(c fiber.Ctx) error {
	sessions, err := h.svc.Sessions(c.Context(), middleware.UserID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	current := middleware.SessionID(c)
	out := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionDTO{
			ID: s.ID, IPAddress: s.IPAddress, UserAgent: s.UserAgent, CreatedAt: s.CreatedAt,
			LastSeenAt: s.LastSeenAt, ExpiresAt: s.ExpiresAt, Current: s.ID == current,
		})
	}
	return c.JSON(fiber.Map{"items": out})
}

// @Summary End one session of the current user
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 404 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/sessions/{id} [delete]
func (h *Handler) Revoke(c fiber.Ctx) error {
	if err := h.svc.RevokeSession(c.Context(), middleware.UserID(c), c.Params("id")); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// @Summary End every session of the current user except this one
// @Tags Sessions
// @Produce json
// @Security BearerAuth
// @Success 204
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/sessions/others [delete]
func (h *Handler) RevokeOthers(c fiber.Ctx) error {
	if err := h.svc.RevokeOtherSessions(c.Context(), middleware.UserID(c), middleware.SessionID(c)); err != nil {
		return mapper.WriteError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
