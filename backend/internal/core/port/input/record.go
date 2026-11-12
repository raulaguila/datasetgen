package input

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
)

type CreateRecordCommand struct {
	CollectionID string
	Data         json.RawMessage
}

type UpdateRecordCommand struct {
	ID      string
	Data    json.RawMessage
	Version *int // the version the caller read; nil skips the check
}

// RecordService is the inbound port for records; data must match the collection schema.
type RecordService interface {
	List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error)
	Get(ctx context.Context, id string) (*domain.Record, error)
	Create(ctx context.Context, cmd CreateRecordCommand) (*domain.Record, error)
	Update(ctx context.Context, cmd UpdateRecordCommand) (*domain.Record, error)
	// DeleteMany deletes every listed record or none.
	DeleteMany(ctx context.Context, ids []string) error
}
