package audit

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
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
	ID          string         `json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	ActorID     string         `json:"actor_id,omitempty"`
	ActorName   string         `json:"actor_name,omitempty"`
	ActorEmail  string         `json:"actor_email,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	Action      string         `json:"action" example:"role.update"`
	TargetID    string         `json:"target_id,omitempty"`
	TargetLabel string         `json:"target_label,omitempty"`
	Details     map[string]any `json:"details"`
}

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

// maxExport caps one file; past it the caller narrows the filters.
const maxExport = 10_000

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
// @Router /v1/audit/export [get]
func (h *Handler) Export(c fiber.Ctx) error {
	// A one-row page first: refusing must not cost the 10000 rows.
	filter, err := mapper.AuditFilter(c, domain.Page{Number: 1, Limit: 1})
	if err != nil {
		return mapper.WriteError(c, err)
	}
	_, total, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	if total > maxExport {
		return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "more than 10000 records: narrow the filters"))
	}
	filter.Page.Limit = maxExport
	records, _, err := h.svc.List(c.Context(), filter)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	body, err := auditCSV(records)
	if err != nil {
		return mapper.WriteError(c, err)
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="audit-`+time.Now().UTC().Format("20060102-150405")+`.csv"`)
	return c.Send(body)
}

func auditCSV(records []domain.AuditRecord) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\uFEFF") // BOM: spreadsheets then read the accents as UTF-8
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"created_at", "action", "actor_name", "actor_email", "actor_id", "target", "target_id", "details", "request_id"})
	for _, r := range records {
		details, err := json.Marshal(r.Details)
		if err != nil {
			return nil, err
		}
		_ = w.Write([]string{
			r.CreatedAt.UTC().Format(time.RFC3339), r.Action, cell(r.ActorName), cell(r.ActorEmail), r.ActorID,
			cell(r.TargetLabel), r.TargetID, cell(string(details)), cell(r.RequestID),
		})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// cell keeps a spreadsheet from running user text as a formula: names and
// emails come from users, and "=HYPERLINK(...)" would otherwise be live.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
