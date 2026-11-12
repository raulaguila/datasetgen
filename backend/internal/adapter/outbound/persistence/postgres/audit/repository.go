package audit

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.AuditRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Record(ctx context.Context, e domain.AuditEntry) error {
	details := []byte("{}")
	if len(e.Details) > 0 {
		var err error
		if details, err = json.Marshal(e.Details); err != nil {
			return err
		}
	}
	return shared.WrapError(r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_audit_log (actor_id, request_id, action, target_id, details)
		VALUES (NULLIF(?, '')::uuid, NULLIF(?, ''), ?, NULLIF(?, '')::uuid, ?::jsonb)
	`, e.ActorID, e.RequestID, e.Action, e.TargetID, string(details)).Error, "audit log")
}

type recordRow struct {
	ID          string    `gorm:"column:id"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	ActorID     *string   `gorm:"column:actor_id"`
	ActorName   *string   `gorm:"column:actor_name"`
	ActorEmail  *string   `gorm:"column:actor_email"`
	RequestID   *string   `gorm:"column:request_id"`
	Action      string    `gorm:"column:action"`
	TargetID    *string   `gorm:"column:target_id"`
	TargetLabel *string   `gorm:"column:target_label"`
	Details     []byte    `gorm:"column:details"`
}

// countCap is one past the largest export (10 000 records), which tells "too many" from "exactly the limit".
const countCap = 10_001

// shortcut: OFFSET pages, fine for a log read by hand; move to keyset on created_at beyond a few hundred thousand rows.
func (r *Repository) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error) {
	where, args := "TRUE", []any{}
	if filter.Action != "" {
		where += " AND a.action LIKE ?"
		args = append(args, filter.Action+"%") // the service allows only [a-z_.] here
	}
	if filter.UserID != "" {
		where += " AND (a.actor_id = ? OR a.target_id = ?)"
		args = append(args, filter.UserID, filter.UserID)
	}
	if filter.CollectionID != "" {
		where += " AND (a.target_id = ? OR a.details->>'collection_id' = ?)"
		args = append(args, filter.CollectionID, filter.CollectionID)
	}
	if filter.From != nil {
		where += " AND a.created_at >= ?"
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		where += " AND a.created_at < ?"
		args = append(args, *filter.To)
	}
	// The log is never cleaned, so the count stops at countCap: past it the total
	// reads "more than 10 000" and the caller narrows the filters, as the export asks.
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM (SELECT 1 FROM usr.usr_audit_log a WHERE `+where+` LIMIT ?) capped`, append(args[:len(args):len(args)], countCap)...).Find(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "audit log")
	}
	var rows []recordRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id::text AS id, a.created_at, a.actor_id::text AS actor_id, au.name AS actor_name, au.email::text AS actor_email,
		       a.request_id, a.action, a.target_id::text AS target_id,
		       COALESCE(tu.email::text, tr.name, (
		         -- Any other target (every module): the newest name the log recorded for it,
		         -- which outlives a delete and needs no join per module.
		         SELECT n.details->>'name' FROM usr.usr_audit_log n
		         WHERE n.target_id = a.target_id AND n.details->>'name' IS NOT NULL
		         ORDER BY n.created_at DESC LIMIT 1
		       )) AS target_label, a.details
		FROM usr.usr_audit_log a
		LEFT JOIN usr.usr_user au ON au.id = a.actor_id
		LEFT JOIN usr.usr_user tu ON tu.id = a.target_id
		LEFT JOIN usr.usr_role tr ON tr.id = a.target_id
		WHERE `+where+`
		ORDER BY a.created_at DESC, a.id
		LIMIT ? OFFSET ?
	`, append(args, filter.Page.Limit, filter.Page.Offset())...).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "audit log")
	}
	out := make([]domain.AuditRecord, 0, len(rows))
	for _, m := range rows {
		rec := domain.AuditRecord{
			ID: m.ID, CreatedAt: m.CreatedAt, Action: m.Action,
			ActorID: deref(m.ActorID), ActorName: deref(m.ActorName), ActorEmail: deref(m.ActorEmail),
			RequestID: deref(m.RequestID), TargetID: deref(m.TargetID), TargetLabel: deref(m.TargetLabel),
		}
		if err := json.Unmarshal(m.Details, &rec.Details); err != nil {
			return nil, 0, shared.WrapError(err, "audit log")
		}
		out = append(out, rec)
	}
	return out, total, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
