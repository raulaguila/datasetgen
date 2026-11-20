package audit

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.AuditService
}

func New(svc input.AuditService) *Handler {
	return &Handler{svc: svc}
}

// Routes expects a router already behind Bearer.
func (h *Handler) Routes(r fiber.Router, authz input.Authorizer) {
	read := middleware.Require(authz, domain.ResourceAudit, domain.ActionRead)
	r.Get("/", read, h.List)
	r.Get("/export", read, h.Export)
}

type recordDTO struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	ActorID     string    `json:"actor_id,omitempty"`
	ActorName   string    `json:"actor_name,omitempty"`
	ActorEmail  string    `json:"actor_email,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	Action      string    `json:"action" example:"role.update"`
	TargetID    string    `json:"target_id,omitempty"`
	TargetLabel string    `json:"target_label,omitempty"`
	TargetGone  bool      `json:"target_gone,omitempty"`
	TargetOwner string    `json:"target_owner,omitempty" example:"ana"` // a collection target's address: /target_owner/target_slug
	TargetSlug  string    `json:"target_slug,omitempty" example:"livros"`
	// The collection a record entry belongs to.
	CollectionName string         `json:"collection_name,omitempty" example:"Pedidos"`
	Details        map[string]any `json:"details"`
} // @name AuditEntry

// @Summary List the audit log, newest first
// @Tags Audit
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Items per page, up to 100" default(20)
// @Param action query string false "Action prefix, such as role. or user.disable"
// @Param user_id query string false "The actor or the target"
// @Param from query string false "From this time on (RFC 3339)"
// @Param to query string false "Before this time (RFC 3339)"
// @Security BearerAuth
// @Success 200 {object} object{items=[]recordDTO,pagination=mapper.PaginationDTO}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID auditList
// @Router /v1/audit [get]
func (h *Handler) List(c fiber.Ctx) error {
	filter, err := mapper.AuditFilter(c, domain.NewPage(fiber.Query[int](c, "page"), fiber.Query[int](c, "limit")))
	if err != nil {
		return mapper.WriteError(c, err)
	}
	records, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	out := make([]recordDTO, 0, len(records))
	for _, r := range records {
		out = append(out, recordDTO(r))
	}
	return c.JSON(fiber.Map{"items": out, "pagination": mapper.Pagination(filter.Page, total)})
}

// @Summary Download the audit log as CSV, newest first, with the filters of the list
// @Tags Audit
// @Produce text/csv
// @Param action query string false "Action prefix, such as role. or user.disable"
// @Param user_id query string false "The actor or the target"
// @Param from query string false "From this time on (RFC 3339)"
// @Param to query string false "Before this time (RFC 3339)"
// @Security BearerAuth
// @Success 200 {string} string "CSV with a header row"
// @Failure 400 {object} mapper.errorBody "bad filter, or more than 10000 records"
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID auditExport
// @Router /v1/audit/export [get]
func (h *Handler) Export(c fiber.Ctx) error {
	return mapper.AuditExport(c, h.svc.List)
}
