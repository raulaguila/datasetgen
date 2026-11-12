package permission

import (
	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.ResourceCatalog
}

func New(svc input.ResourceCatalog) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	r.Get("/resources", middleware.Require(authz, domain.ResourceRoles, domain.ActionRead), h.ListResources)
}

type resourceDTO struct {
	Code        string   `json:"code" example:"products"`
	Description string   `json:"description"`
	Actions     []string `json:"actions" example:"create,read,update,delete"`
}

// @Summary List the resources that roles may be granted
// @Tags Roles
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{items=[]resourceDTO}
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @Router /v1/permissions/resources [get]
func (h *Handler) ListResources(c fiber.Ctx) error {
	resources, err := h.svc.ListResources(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]resourceDTO, 0, len(resources))
	for _, r := range resources {
		out = append(out, resourceDTO{Code: r.Code, Description: r.Description, Actions: domain.ActionNames(r.Actions)})
	}
	return c.JSON(fiber.Map{"items": out})
}
