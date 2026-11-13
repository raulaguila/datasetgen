package record

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
)

type commentRow struct {
	ID         uuid.UUID  `gorm:"column:id"`
	RecordID   uuid.UUID  `gorm:"column:record_id"`
	UserID     *uuid.UUID `gorm:"column:user_id"`
	AuthorName *string    `gorm:"column:author_name"`
	Body       string     `gorm:"column:body"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (m commentRow) toDomain() domain.RecordComment {
	c := domain.RecordComment{ID: m.ID.String(), RecordID: m.RecordID.String(), Body: m.Body, CreatedAt: m.CreatedAt}
	if m.UserID != nil {
		c.UserID = m.UserID.String()
	}
	if m.AuthorName != nil {
		c.AuthorName = *m.AuthorName
	}
	return c
}

func (r *Repository) Comments(ctx context.Context, recordID string, limit int) ([]domain.RecordComment, error) {
	var rows []commentRow
	err := r.db.WithContext(ctx).Table("app.record_comment c").
		Select("c.id, c.record_id, c.user_id, u.name AS author_name, c.body, c.created_at").
		Joins("LEFT JOIN usr.usr_user u ON u.id = c.user_id").
		Where("c.record_id = ?", recordID).Order("c.created_at").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, shared.WrapError(err, "comment")
	}
	out := make([]domain.RecordComment, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.toDomain())
	}
	return out, nil
}

func (r *Repository) AddComment(ctx context.Context, c *domain.RecordComment) error {
	var m commentRow
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO app.record_comment (record_id, user_id, body) VALUES (?, ?, ?) RETURNING id, created_at`,
		c.RecordID, c.UserID, c.Body).Take(&m).Error
	if err != nil {
		return shared.WrapError(err, "comment")
	}
	c.ID, c.CreatedAt = m.ID.String(), m.CreatedAt
	return nil
}

func (r *Repository) Comment(ctx context.Context, id string) (*domain.RecordComment, error) {
	var m commentRow
	if err := r.db.WithContext(ctx).Table("app.record_comment").Select("id, record_id, user_id").Where("id = ?", id).Take(&m).Error; err != nil {
		return nil, shared.WrapError(err, "comment")
	}
	c := m.toDomain()
	return &c, nil
}

func (r *Repository) DeleteComment(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM app.record_comment WHERE id = ?`, id)
	if res.Error != nil {
		return shared.WrapError(res.Error, "comment")
	}
	if res.RowsAffected == 0 {
		return shared.WrapError(gorm.ErrRecordNotFound, "comment")
	}
	return nil
}
