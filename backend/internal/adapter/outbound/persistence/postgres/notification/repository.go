package notification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.NotificationRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	ID        uuid.UUID  `gorm:"column:id"`
	UserID    uuid.UUID  `gorm:"column:user_id"`
	Level     string     `gorm:"column:level"`
	Kind      *string    `gorm:"column:kind"`
	Params    []byte     `gorm:"column:params"`
	Title     string     `gorm:"column:title"`
	Message   string     `gorm:"column:message"`
	ReadAt    *time.Time `gorm:"column:read_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	// The address of params.collection_id, read with the list.
	CollectionOwner *string `gorm:"column:collection_owner"`
	CollectionSlug  *string `gorm:"column:collection_slug"`
}

func (r *Repository) Create(ctx context.Context, n *domain.Notification) error {
	id, now := uuid.New(), time.Now()
	params := []byte("{}")
	if len(n.Params) > 0 {
		var err error
		if params, err = json.Marshal(n.Params); err != nil {
			return err
		}
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_notification (id, user_id, level, kind, params, title, message, created_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?::jsonb, ?, ?, ?)
	`, id, n.UserID, n.Level, n.Kind, string(params), n.Title, n.Message, now).Error
	if err != nil {
		return shared.WrapError(err, "notification")
	}
	n.ID = id.String()
	n.CreatedAt = now
	return nil
}

func (r *Repository) List(ctx context.Context, userID string, page domain.Page) ([]domain.Notification, int64, error) {
	owned := func() *gorm.DB {
		return r.db.WithContext(ctx).Table("usr.usr_notification").Where("user_id = ?", userID)
	}
	var total int64
	if err := owned().Count(&total).Error; err != nil {
		return nil, 0, shared.WrapError(err, "notification")
	}
	var rows []row
	// A notification about a collection links to its current address; none once it is deleted.
	err := r.db.WithContext(ctx).Table("usr.usr_notification n").
		Select("n.id, n.user_id, n.level, n.kind, n.params, n.title, n.message, n.read_at, n.created_at, o.username AS collection_owner, c.slug AS collection_slug").
		Joins("LEFT JOIN app.collection c ON c.id = (n.params->>'collection_id')::uuid AND c.deleted_at IS NULL").
		Joins("LEFT JOIN usr.usr_user o ON o.id = c.owner_id").
		Where("n.user_id = ?", userID).
		Order("n.created_at DESC, n.id").Limit(page.Limit).Offset(page.Offset()).Find(&rows).Error
	if err != nil {
		return nil, 0, shared.WrapError(err, "notification")
	}
	out := make([]domain.Notification, 0, len(rows))
	for _, m := range rows {
		n := domain.Notification{
			ID: m.ID.String(), UserID: m.UserID.String(), Level: m.Level, Title: m.Title,
			Message: m.Message, ReadAt: m.ReadAt, CreatedAt: m.CreatedAt,
		}
		if m.Kind != nil {
			n.Kind = *m.Kind
		}
		if m.CollectionOwner != nil && m.CollectionSlug != nil {
			n.CollectionOwner, n.CollectionSlug = *m.CollectionOwner, *m.CollectionSlug
		}
		if err := json.Unmarshal(m.Params, &n.Params); err != nil {
			return nil, 0, shared.WrapError(err, "notification")
		}
		out = append(out, n)
	}
	return out, total, nil
}

func (r *Repository) CountUnread(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("usr.usr_notification").
		Where("user_id = ? AND read_at IS NULL", userID).Count(&n).Error
	if err != nil {
		return 0, shared.WrapError(err, "notification")
	}
	return n, nil
}

func (r *Repository) MarkRead(ctx context.Context, userID string, ids []string, now time.Time) (bool, error) {
	if ids == nil {
		err := r.db.WithContext(ctx).Exec(`
			UPDATE usr.usr_notification SET read_at = ? WHERE user_id = ? AND read_at IS NULL
		`, now, userID).Error
		return true, shared.WrapError(err, "notification")
	}
	// One statement: the ownership count and the update see the same snapshot.
	res := r.db.WithContext(ctx).Exec(`
		UPDATE usr.usr_notification SET read_at = COALESCE(read_at, ?) WHERE user_id = ? AND id IN ?
		AND (SELECT count(*) FROM usr.usr_notification WHERE user_id = ? AND id IN ?) = ?
	`, now, userID, ids, userID, ids, len(ids))
	if res.Error != nil {
		return false, shared.WrapError(res.Error, "notification")
	}
	return res.RowsAffected == int64(len(ids)), nil
}
