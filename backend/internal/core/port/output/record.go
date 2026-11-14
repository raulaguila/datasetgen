package output

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// RecordRepository persists app.record.
type RecordRepository interface {
	// List returns one page; each Data holds only top-level plain values, strings cut to 120 characters.
	List(ctx context.Context, filter domain.RecordFilter) ([]domain.Record, int64, error)
	// Aggregate counts the records the filter selects, by group field (or all
	// together when group is ""), with sums and averages of numeric fields.
	Aggregate(ctx context.Context, filter domain.RecordFilter, group string, numeric []string) ([]domain.RecordGroup, error)
	// Stats profiles the selected records over the query's fields, in one pass per part.
	Stats(ctx context.Context, filter domain.RecordFilter, q domain.StatsQuery) (*domain.RecordStats, error)
	FindByID(ctx context.Context, id string) (*domain.Record, error)
	Create(ctx context.Context, r *domain.Record) error
	// Update replaces data; with version set, a row changed since that version is a conflict.
	Update(ctx context.Context, id string, data json.RawMessage, version *int) (*domain.Record, error)
	// CollectionsOf maps each listed record that exists to its collection id.
	CollectionsOf(ctx context.Context, ids []string) (map[string]string, error)
	// Scan returns up to limit records of the collection after afterID (by id),
	// with id, full data and updated_at only.
	Scan(ctx context.Context, collectionID, afterID string, limit int) ([]domain.Record, error)
	// Keys lists the top-level keys the collection's records hold, sorted, up to limit.
	Keys(ctx context.Context, collectionID string, limit int) ([]string, error)
	// CreateMany inserts one record per JSON text; the caller has validated them.
	CreateMany(ctx context.Context, collectionID string, data []string) (int64, error)
	// CopyAll copies every record of one collection into another, as new records.
	CopyAll(ctx context.Context, fromCollection, toCollection string) (int64, error)
	// Versions lists the record's earlier versions, newest first.
	Versions(ctx context.Context, id string, limit int) ([]domain.RecordVersion, error)
	// Version is the data of one earlier version.
	Version(ctx context.Context, id string, version int) (json.RawMessage, error)
	// Fix applies a bulk change to the collection's records, saving a version of
	// each record it changes, and returns how many changed.
	Fix(ctx context.Context, collectionID string, fix domain.RecordFix) (int64, error)
	// CountFix counts the records Fix would change.
	CountFix(ctx context.Context, collectionID string, fix domain.RecordFix) (int64, error)
	// DeleteListed deletes the listed records of the collection that were not
	// changed after unchangedSince, and returns the ids it deleted.
	DeleteListed(ctx context.Context, collectionID string, ids []string, unchangedSince time.Time) ([]string, error)
	// LockUnique serializes, until the transaction ends, the writes of one
	// collection that check unique fields, so two of them cannot both pass.
	LockUnique(ctx context.Context, collectionID string) error
	// TakenValues returns the positions in values of those another record of the
	// collection (not excludeID) already holds under key.
	TakenValues(ctx context.Context, collectionID, excludeID, key string, values []json.RawMessage) ([]int, error)
	// Comments lists a record's comments, oldest first, up to limit.
	Comments(ctx context.Context, recordID string, limit int) ([]domain.RecordComment, error)
	AddComment(ctx context.Context, c *domain.RecordComment) error
	// Comment reads one comment's record and author (NotFound when gone).
	Comment(ctx context.Context, id string) (*domain.RecordComment, error)
	DeleteComment(ctx context.Context, id string) error
	// MaxNumber is the largest number the collection's records hold under key, 0 when none.
	MaxNumber(ctx context.Context, collectionID, key string) (int64, error)
	// Matching finds, per value (by index), the record of the collection whose key holds it.
	Matching(ctx context.Context, collectionID, key string, values []json.RawMessage) (map[int]domain.Record, error)
	// DuplicatedKeys returns the listed keys that two or more records share a value of.
	DuplicatedKeys(ctx context.Context, collectionID string, keys []string) ([]string, error)
}
