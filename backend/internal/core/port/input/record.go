package input

import (
	"context"
	"encoding/json"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/schema"
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
	// Versions lists earlier versions (viewers); Restore writes one back as a new version (editors).
	Versions(ctx context.Context, id string) ([]domain.RecordVersion, error)
	Restore(ctx context.Context, cmd RestoreRecordCommand) (*domain.Record, error)
	// Import checks a batch against the schema and inserts it (editors).
	Import(ctx context.Context, cmd ImportRecordsCommand) (*ImportResult, error)
	// Export returns every record of the collection (viewers).
	Export(ctx context.Context, collectionID string, validOnly bool) (*Export, error)
	// BulkUpdate sets the same values in several records of one collection (editors).
	BulkUpdate(ctx context.Context, cmd BulkUpdateCommand) (int, error)
}

type RestoreRecordCommand struct {
	ID             string
	Version        int  // the earlier version to write back
	CurrentVersion *int // the version the caller read; nil skips the check
}

type ImportRecordsCommand struct {
	CollectionID string
	Records      []json.RawMessage
	DryRun       bool // check only
	SkipInvalid  bool // insert the valid records even when some are invalid
}

type ImportResult struct {
	Created int64
	Invalid int
	Errors  []ImportError // the first 100
}

// ImportError names an invalid record by its index in the request.
type ImportError struct {
	Index  int
	Issues []schema.Issue
}

type Export struct {
	Name    string
	Schema  json.RawMessage
	Records []json.RawMessage
}
