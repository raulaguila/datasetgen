package preference

import (
	"context"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
)

type Repository struct {
	db *gorm.DB
}

var _ output.PreferenceRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type row struct {
	UserID   string `gorm:"column:user_id"`
	Theme    string `gorm:"column:theme"`
	Language string `gorm:"column:language"`
}

func (r *Repository) Find(ctx context.Context, userID string) (*domain.Preferences, error) {
	var m row
	err := r.db.WithContext(ctx).Table("usr.usr_preference").Where("user_id = ?", userID).Take(&m).Error
	if err != nil {
		return nil, shared.WrapError(err, "preference")
	}
	return &domain.Preferences{UserID: m.UserID, Theme: m.Theme, Language: m.Language}, nil
}

func (r *Repository) Save(ctx context.Context, p *domain.Preferences) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO usr.usr_preference (user_id, theme, language)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET theme = EXCLUDED.theme, language = EXCLUDED.language
	`, p.UserID, p.Theme, p.Language).Error
	return shared.WrapError(err, "preference")
}
