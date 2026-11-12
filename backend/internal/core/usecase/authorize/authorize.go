package authorize

import (
	"context"
	"fmt"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

// Service loads effective permissions and checks CRUD bits.
type Service struct {
	Reader output.PermissionReader
	Clock  output.Clock
}

var (
	_ input.Authorizer      = (*Service)(nil)
	_ input.ResourceCatalog = (*Service)(nil)
)

func New(reader output.PermissionReader, clock output.Clock) *Service {
	return &Service{Reader: reader, Clock: clock}
}

func (s *Service) Authorize(ctx context.Context, p domain.Principal, resource string, need domain.Action) error {
	if resource == "" {
		return apperror.New(apperror.CodeInvalidInput, "resource required")
	}
	if need == 0 {
		return apperror.New(apperror.CodeInvalidInput, "action required")
	}
	if p.UserID == "" || p.SessionID == "" {
		return apperror.New(apperror.CodeUnauthorized, "unauthenticated")
	}
	sess, have, err := s.Reader.Access(ctx, p.SessionID, resource)
	if apperror.IsNotFound(err) {
		return apperror.New(apperror.CodeUnauthorized, "session not found")
	}
	if err != nil {
		return err
	}
	if err := policy.CheckSession(sess, p.UserID, s.Clock.Now()); err != nil {
		return err
	}
	if domain.Allows(have, need) {
		return nil
	}
	return apperror.New(apperror.CodeForbidden, fmt.Sprintf("missing %s permission on %s", needLabel(need), resource))
}

func (s *Service) ListResources(ctx context.Context) ([]domain.Resource, error) {
	return s.Reader.ListResources(ctx)
}

func needLabel(need domain.Action) string {
	switch {
	case domain.Allows(need, domain.ActionCreate):
		return "create"
	case domain.Allows(need, domain.ActionRead):
		return "read"
	case domain.Allows(need, domain.ActionUpdate):
		return "update"
	case domain.Allows(need, domain.ActionDelete):
		return "delete"
	default:
		return "action"
	}
}
