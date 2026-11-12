package health

import (
	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type Handler struct {
	svc input.HealthChecker
}

func NewHandler(svc input.HealthChecker) *Handler {
	return &Handler{svc: svc}
}

type responseDTO struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Get handles GET /health.
//
// @Summary Service status
// @Tags Health
// @Produce json
// @Success 200 {object} responseDTO
// @Failure 503 {object} responseDTO
// @Router /health [get]
func (h *Handler) Get(c fiber.Ctx) error {
	report, err := h.svc.Check(c.Context())
	if err != nil {
		return mapper.WriteError(c, err)
	}

	checks := make(map[string]string, len(report.Checks))
	for k, v := range report.Checks {
		checks[k] = string(v)
	}

	status := fiber.StatusOK
	if report.Status == "down" {
		status = fiber.StatusServiceUnavailable
	}

	return c.Status(status).JSON(responseDTO{
		Status: report.Status,
		Checks: checks,
	})
}
