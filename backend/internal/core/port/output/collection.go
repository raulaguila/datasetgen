package output

import (
	"context"
	"encoding/json"

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
	// DeleteMany deletes every listed collection or, when one is missing, none,
	// and returns the deleted names by id for the audit log.
	DeleteMany(ctx context.Context, ids []string) (map[string]string, error)

	// Members lists the collection's members, owners first, then by name.
	Members(ctx context.Context, id string) ([]domain.CollectionMember, error)
	// AddMember fails with AlreadyExists when userID is already a member.
	AddMember(ctx context.Context, id, userID string, role domain.MemberRole) error
	SetMemberRole(ctx context.Context, id, userID string, role domain.MemberRole) error
	RemoveMember(ctx context.Context, id, userID string) error
	// Owners counts the collection's owner members.
	Owners(ctx context.Context, id string) (int64, error)
}
