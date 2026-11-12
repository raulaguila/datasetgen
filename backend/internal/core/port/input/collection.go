package input

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/schema"
)

type CreateCollectionCommand struct {
	Name        string
	Description string
	Icon        string
	Schema      json.RawMessage
}

type UpdateCollectionCommand struct {
	ID          string
	Name        *string
	Description *string
	Icon        *string
	Schema      json.RawMessage // nil keeps the stored schema
	Version     *int            // the version the caller read; nil skips the check
}

// AddMemberCommand adds a person by email; an email without an account is
// invited (Name is then required) and joins at once, pending until activation.
type AddMemberCommand struct {
	CollectionID string
	Email        string
	Name         string
	Role         domain.MemberRole
}

// AddMemberResult carries the invite's activation token when the server hands it over (manual delivery).
type AddMemberResult struct {
	Member          domain.CollectionMember
	Invited         bool
	ActivationToken string
}

// QualityState is the latest quality check (nil when it never ran) and the
// collection's current version: a check of an older version is stale.
type QualityState struct {
	Run            *domain.QualityRun
	CurrentVersion int
}

// InvalidRecord is one entry of QualityRun.Invalid, stored as JSON.
type InvalidRecord struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt time.Time       `json:"updated_at"`
	Issues    []schema.Issue  `json:"issues"`
}

type DuplicateCollectionCommand struct {
	ID      string
	Name    string
	Records bool // copy the records too
}

// CollectionService is the inbound port for collections and their schemas.
type CollectionService interface {
	List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error)
	Options(ctx context.Context) ([]domain.Collection, error)
	Get(ctx context.Context, id string) (*domain.Collection, error)
	Create(ctx context.Context, cmd CreateCollectionCommand) (*domain.Collection, error)
	Update(ctx context.Context, cmd UpdateCollectionCommand) (*domain.Collection, error)
	// DeleteMany deletes every listed collection or none; collections with records are refused.
	DeleteMany(ctx context.Context, ids []string) error
	// Members, AddMember, SetMemberRole and RemoveMember are for the collection's owners.
	Members(ctx context.Context, id string) ([]domain.CollectionMember, error)
	AddMember(ctx context.Context, cmd AddMemberCommand) (*AddMemberResult, error)
	SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error
	RemoveMember(ctx context.Context, id, userID string) error
	// AuditLog is the collection's audit log (owners); Quality checks its records against the schema (editors).
	AuditLog(ctx context.Context, id string, filter domain.AuditFilter) ([]domain.AuditRecord, int64, error)
	// Quality is the latest check; StartQuality queues a new one unless one is running (editors).
	Quality(ctx context.Context, id string) (*QualityState, error)
	StartQuality(ctx context.Context, id string) (*QualityState, error)
	// Fix applies a bulk change to the records' data (editors); DeleteInvalid deletes the
	// records the latest check found invalid and nobody changed since (editors).
	Fix(ctx context.Context, id string, fix domain.RecordFix) (int64, error)
	DeleteInvalid(ctx context.Context, id string) (int, error)
	// Duplicate copies a collection the caller can read into a new one they own.
	Duplicate(ctx context.Context, cmd DuplicateCollectionCommand) (*domain.Collection, error)
	// Stats is the overview of the last 30 days, days counted in tz (an IANA name; "" is UTC). Every member.
	Stats(ctx context.Context, id, tz string) (*domain.CollectionStats, error)
	// Leave removes the caller from the collection; the last owner cannot leave.
	Leave(ctx context.Context, id string) error
}
