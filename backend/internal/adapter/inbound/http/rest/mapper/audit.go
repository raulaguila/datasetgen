package mapper

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// AuditFilter reads the audit filters shared by the log, its export and a collection's log.
func AuditFilter(c fiber.Ctx, page domain.Page) (domain.AuditFilter, error) {
	filter := domain.AuditFilter{Page: page, Action: c.Query("action"), UserID: c.Query("user_id")}
	var err error
	if filter.From, err = queryTime(c, "from"); err != nil {
		return filter, err
	}
	filter.To, err = queryTime(c, "to")
	return filter, err
}

func queryTime(c fiber.Ctx, key string) (*time.Time, error) {
	raw := c.Query(key)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, key+" must be an RFC 3339 time").WithField(key)
	}
	return &t, nil
}
