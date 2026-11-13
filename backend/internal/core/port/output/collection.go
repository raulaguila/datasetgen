package output

import (
	"context"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// CollectionRepository persists app.collection.
type CollectionRepository interface {
	// List returns one page, with record counts, and the count of all matches.
	List(ctx context.Context, filter domain.CollectionFilter) ([]domain.Collection, int64, error)
	// ListOptions returns the collections userID can see, with only id and name.
	ListOptions(ctx context.Context, userID string, limit int) ([]domain.Collection, error)
	// Access is userID's role in the collection: owner for a platform admin
	// (superuser or '*'), else the member role. NotFound when the collection is
	// missing or userID is not a member.
	Access(ctx context.Context, id, userID string) (domain.MemberRole, error)
	FindByID(ctx context.Context, id string) (*domain.Collection, error)
	// SchemaForWrite reads only the schema and holds a FOR SHARE lock until the
	// transaction ends, so a schema update waits for records being checked against it.
	SchemaForWrite(ctx context.Context, id string) (json.RawMessage, error)
	Create(ctx context.Context, c *domain.Collection) error
	// Update writes only the fields set in patch; with patch.Version set, a row
	// changed since that version is a conflict.
	Update(ctx context.Context, id string, patch domain.CollectionPatch) (*domain.Collection, error)

	// Members lists the collection's members, owners first, then by name.
	Members(ctx context.Context, id string) ([]domain.CollectionMember, error)
	// AddMember fails with AlreadyExists when userID is already a member.
	AddMember(ctx context.Context, id, userID string, role domain.MemberRole) error
	SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error
	RemoveMember(ctx context.Context, id, userID string) error
	// Stats is the overview: totals, records created per day in tz over the last
	// days, and the most active contributors in that window.
	Stats(ctx context.Context, id, tz string, days int) (*domain.CollectionStats, error)
	// Quality is the latest check of the collection, nil when it never ran.
	Quality(ctx context.Context, id string, withIDs bool) (*domain.QualityRun, error)
	// StartQuality marks a check as running and reports whether it did: false
	// while another check started less than stale ago is still running.
	StartQuality(ctx context.Context, id string, schemaVersion int, stale time.Duration) (bool, error)
	// FinishQuality stores the result of the running check.
	FinishQuality(ctx context.Context, id string, run domain.QualityRun) error
	// HasMembership reports whether userID belongs to any collection.
	HasMembership(ctx context.Context, userID string) (bool, error)
	// Owners counts the collection's owner members.
	Owners(ctx context.Context, id string) (int64, error)
}
