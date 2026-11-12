package preference

import (
	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.AccountService
}

func New(svc input.AccountService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router) {
	r.Get("/", h.Get)
	r.Put("/", h.Update)
}

// @Summary Preferences of the current user
// @Tags Preferences
// @Produce json
// @Security BearerAuth
// @Success 200 {object} mapper.PreferencesDTO
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/preferences [get]
func (h *Handler) Get(c fiber.Ctx) error {
	prefs, err := h.svc.Preferences(c.Context(), middleware.UserID(c))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(mapper.Preferences(*prefs))
}

// @Summary Replace the preferences of the current user
// @Tags Preferences
// @Produce json
// @Accept json
// @Param body body mapper.PreferencesDTO true "Request"
// @Security BearerAuth
// @Success 200 {object} mapper.PreferencesDTO
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/me/preferences [put]
func (h *Handler) Update(c fiber.Ctx) error {
	var body mapper.PreferencesDTO
	if err := c.Bind().Body(&body); err != nil {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
	}
	prefs, err := h.svc.UpdatePreferences(c.Context(), domain.Preferences{
		UserID: middleware.UserID(c), Theme: body.Theme, Language: body.Language,
	})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	return c.JSON(mapper.Preferences(*prefs))
}
