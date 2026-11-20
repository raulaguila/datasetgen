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
	// Overview returns the listed collections userID can see, by name: schema, role,
	// record and trash counts and the newest record change.
	Overview(ctx context.Context, userID string, ids []string) ([]domain.Collection, error)
	// ListOptions returns the collections userID can see: id, name, role and membership.
	ListOptions(ctx context.Context, userID string, limit int) ([]domain.Collection, error)
	// Access is userID's role in the collection: owner for a platform admin
	// (superuser or '*'), else the member role. NotFound when the collection is
	// missing or userID is not a member.
	Access(ctx context.Context, id, userID string) (domain.MemberRole, error)
	// Referrers lists the collections userID belongs to whose reference fields point at targetID.
	Referrers(ctx context.Context, targetID, userID string) ([]domain.Referrer, error)
	FindByID(ctx context.Context, id string) (*domain.Collection, error)
	// FindIDByPath finds the collection at /<username>/<slug>, or at an old path of it
	// (a rename, transfer or username change). No access check.
	FindIDByPath(ctx context.Context, username, slug string) (string, error)
	// OwnerOf is the id of the account in the collection's URL.
	OwnerOf(ctx context.Context, id string) (string, error)
	// SetOwner moves the collection's URL to userID's account.
	SetOwner(ctx context.Context, id, userID string) error
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
	// With force it starts over even while one runs. The token names this start
	// ("" when it did not start) for FinishQuality.
	StartQuality(ctx context.Context, id string, schemaVersion int, stale time.Duration, force bool) (token string, err error)
	// FinishQuality stores the result of the running check, unless a later start
	// replaced it (token no longer current; "" always stores).
	FinishQuality(ctx context.Context, id, token string, run domain.QualityRun) error
	// Owners counts the collection's owner members.
	Owners(ctx context.Context, id string) (int64, error)
}
