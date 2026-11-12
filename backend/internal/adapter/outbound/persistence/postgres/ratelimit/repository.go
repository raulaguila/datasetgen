package ratelimit

import (
	"context"

	"gorm.io/gorm"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/shared"
)

// Repository counts requests in sys.rate_limit, so every instance shares one limit.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Hit counts one request for key in the current minute and returns the total.
// The upsert is atomic, so concurrent instances never lose a hit.
func (r *Repository) Hit(ctx context.Context, key string) (int64, error) {
	var hits int64
	err := r.db.WithContext(ctx).Raw(`
		INSERT INTO sys.rate_limit AS l (key, window_start) VALUES (?, date_trunc('minute', now()))
		ON CONFLICT (key, window_start) DO UPDATE SET hits = l.hits + 1
		RETURNING hits
	`, key).Find(&hits).Error
	return hits, shared.WrapError(err, "rate limit")
}
