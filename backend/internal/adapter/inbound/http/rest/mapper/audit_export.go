package mapper

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// maxAuditExport caps one file; past it the caller narrows the filters.
const maxAuditExport = 10_000

// AuditExport answers with the log list reads, as CSV, newest first, under the
// list's filters: the whole log's export and a collection's share it.
func AuditExport(c fiber.Ctx, list func(context.Context, domain.AuditFilter) ([]domain.AuditRecord, int64, error)) error {
	// A one-row page first: refusing must not cost the 10000 rows.
	filter, err := AuditFilter(c, domain.Page{Number: 1, Limit: 1})
	if err != nil {
		return WriteError(c, err)
	}
	_, total, err := list(c.Context(), filter)
	if err != nil {
		return WriteError(c, err)
	}
	if total > maxAuditExport {
		return WriteError(c, apperror.New(apperror.CodeInvalidInput, "more than 10000 records: narrow the filters"))
	}
	filter.Page.Limit = maxAuditExport
	records, _, err := list(c.Context(), filter)
	if err != nil {
		return WriteError(c, err)
	}
	body, err := auditCSV(records)
	if err != nil {
		return WriteError(c, err)
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="audit-`+time.Now().UTC().Format("20060102-150405")+`.csv"`)
	return c.Send(body)
}

func auditCSV(records []domain.AuditRecord) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\uFEFF") // BOM: spreadsheets then read the accents as UTF-8
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"created_at", "action", "actor_name", "actor_email", "actor_id", "target", "target_id", "collection", "details", "request_id"})
	for _, r := range records {
		details, err := json.Marshal(r.Details)
		if err != nil {
			return nil, err
		}
		_ = w.Write([]string{
			r.CreatedAt.UTC().Format(time.RFC3339), r.Action, CSVCell(r.ActorName), CSVCell(r.ActorEmail), r.ActorID,
			CSVCell(r.TargetLabel), r.TargetID, CSVCell(r.CollectionName), CSVCell(string(details)), CSVCell(r.RequestID),
		})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
