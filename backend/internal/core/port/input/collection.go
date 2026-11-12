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
	Schema      json.RawMessage
}

type UpdateCollectionCommand struct {
	ID          string
	Name        *string
	Description *string
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

// QualityReport lists the stored records that no longer match the schema.
type QualityReport struct {
	Version      int // the collection version checked against
	Checked      int
	InvalidCount int
	Invalid      []InvalidRecord // the first 100
	Truncated    bool            // stopped before the end of the collection
}

type InvalidRecord struct {
	ID        string
	Data      json.RawMessage
	UpdatedAt time.Time
	Issues    []schema.Issue
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
	Quality(ctx context.Context, id string) (*QualityReport, error)
}
