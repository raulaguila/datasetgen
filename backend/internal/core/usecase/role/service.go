// Package role manages roles at runtime. Protected roles stay out of reach
// here: giving or taking full access is an operator action (cmd/admin).
package role

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	listCap        = 1000
	maxSearchBytes = 120
	maxBatch       = 100 // ids in one batch request; a page holds at most 100 rows
)

// Same rule as chk_usr_role_name.
var nameFormat = regexp.MustCompile(`^[a-z0-9_-]{2,40}$`)

type Service struct {
	UOW   output.IdentityStore
	Tx    output.TxManager
	Perms output.PermissionReader
}

var _ input.RoleService = (*Service)(nil)

func New(uow output.IdentityStore, tx output.TxManager, perms output.PermissionReader) *Service {
	return &Service{UOW: uow, Tx: tx, Perms: perms}
}

func errInvalidName() error {
	return apperror.New(apperror.CodeInvalidInput, "name must have 2 to 40 of a-z, 0-9, _ and -").WithField("name")
}

// List leaves out protected roles: the API cannot change them, so a role form
// has nothing to do with them.
func (s *Service) List(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, int64, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	if err := validate.Sort(&filter.Sort, &filter.Order, []string{"name", "created_at"}, "name", "asc"); err != nil {
		return nil, 0, err
	}
	return s.UOW.Roles().Page(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Role, error) {
	return s.editable(ctx, s.UOW, id, true)
}

func (s *Service) Create(ctx context.Context, name string) (*domain.Role, error) {
	if !nameFormat.MatchString(name) {
		return nil, errInvalidName()
	}
	var created *domain.Role
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		var err error
		if created, err = uow.Roles().Create(ctx, name); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "role.create", created.ID, map[string]any{"name": name})
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) DeleteMany(ctx context.Context, ids []string) error {
	if err := checkBatch(ids); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		for _, id := range slices.Compact(slices.Sorted(slices.Values(ids))) {
			role, err := s.editable(ctx, uow, id, false)
			if err != nil {
				return err
			}
			if role.IsDefault {
				return apperror.New(apperror.CodeConflict, "a default role cannot be deleted: "+role.Name) // unmark it first
			}
			if err := uow.Roles().Delete(ctx, role.ID); err != nil {
				return err
			}
			if err := policy.Audit(ctx, uow, "role.delete", role.ID, map[string]any{"name": role.Name}); err != nil {
				return err
			}
		}
		return nil
	})
}

func checkBatch(ids []string) error {
	if len(ids) == 0 || len(ids) > maxBatch {
		return apperror.New(apperror.CodeInvalidInput, "ids must hold 1 to 100 ids").WithField("ids")
	}
	return nil
}

// Update renames, switches a role on and off, or marks it default. A default
// role stays enabled: activation gives only enabled ones.
func (s *Service) Update(ctx context.Context, id string, patch domain.RolePatch) (*domain.Role, error) {
	if patch.Name != nil && !nameFormat.MatchString(*patch.Name) {
		return nil, errInvalidName()
	}
	var updated *domain.Role
	err := s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		role, err := s.editable(ctx, uow, id, false)
		if err != nil {
			return err
		}
		enabled, isDefault := role.Enabled, role.IsDefault
		if patch.Enabled != nil {
			enabled = *patch.Enabled
		}
		if patch.IsDefault != nil {
			isDefault = *patch.IsDefault
		}
		if isDefault && !enabled {
			return apperror.New(apperror.CodeConflict, "a default role cannot be disabled")
		}
		if err := uow.Roles().Update(ctx, role.ID, patch); err != nil {
			return err
		}
		changes := map[string]any{}
		if patch.Name != nil {
			changes["name"] = *patch.Name
		}
		if patch.Enabled != nil {
			changes["enabled"] = *patch.Enabled
		}
		if patch.IsDefault != nil {
			changes["is_default"] = *patch.IsDefault
		}
		if err := policy.Audit(ctx, uow, "role.update", role.ID, changes); err != nil {
			return err
		}
		updated, err = uow.Roles().FindByID(ctx, role.ID, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// SetPermissions replaces every permission of a role in one transaction, so a
// failure leaves the previous ones. Only catalog resources pass, which also
// refuses "*": that sentinel would turn the role into full access. A resource
// with no actions gets no row, and actions the resource does not take are refused.
func (s *Service) SetPermissions(ctx context.Context, roleID string, permissions map[string]domain.Action) (*domain.Role, error) {
	resources, err := s.Perms.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	for resource, actions := range permissions {
		i := slices.IndexFunc(resources, func(r domain.Resource) bool { return r.Code == resource })
		if i < 0 {
			return nil, apperror.New(apperror.CodeInvalidInput, "unknown resource").WithField("permissions")
		}
		if !domain.Allows(resources[i].Actions, actions) {
			return nil, apperror.New(apperror.CodeInvalidInput, resource+" does not take these actions").WithField("permissions")
		}
	}
	// Records are listed and edited on the collection screen, which reads the schema.
	if permissions[domain.ResourceRecords] != 0 {
		permissions[domain.ResourceCollections] |= domain.ActionRead
	}
	var updated *domain.Role
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := validate.ID(roleID); err != nil {
			return err
		}
		if err := uow.Lock(ctx, "role:"+roleID); err != nil { // two saves at once would collide on the rows
			return err
		}
		if _, err := s.editable(ctx, uow, roleID, false); err != nil {
			return err
		}
		if err := uow.Roles().ClearPermissions(ctx, roleID); err != nil {
			return err
		}
		for resource, actions := range permissions {
			if actions == 0 {
				continue
			}
			if err := uow.Roles().Grant(ctx, roleID, resource, actions); err != nil {
				return err
			}
		}
		if err := policy.Audit(ctx, uow, "role.permissions", roleID, map[string]any{"permissions": permissions}); err != nil {
			return err
		}
		updated, err = uow.Roles().FindByID(ctx, roleID, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) Members(ctx context.Context, roleID string, filter domain.UserFilter) ([]domain.User, int64, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if err := validate.Search(filter.Search, maxSearchBytes); err != nil {
		return nil, 0, err
	}
	if _, err := s.editable(ctx, s.UOW, roleID, false); err != nil {
		return nil, 0, err
	}
	return s.UOW.Roles().Members(ctx, roleID, filter)
}

func (s *Service) AddMember(ctx context.Context, roleID, userID string) error {
	return s.changeMember(ctx, roleID, userID, "role.member_add", output.RoleRepository.Assign)
}

func (s *Service) RemoveMember(ctx context.Context, roleID, userID string) error {
	return s.changeMember(ctx, roleID, userID, "role.member_remove", output.RoleRepository.Unassign)
}

func (s *Service) changeMember(ctx context.Context, roleID, userID, action string, change func(output.RoleRepository, context.Context, string, string) error) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		role, err := s.editable(ctx, uow, roleID, false)
		if err != nil {
			return err
		}
		target, err := policy.Reachable(ctx, uow, userID)
		if err != nil {
			return err
		}
		if err := change(uow.Roles(), ctx, target.ID, role.ID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, action, role.ID, map[string]any{"user_id": target.ID})
	})
}

func (s *Service) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	if err := validate.ID(userID); err != nil {
		return err
	}
	if len(roleIDs) > listCap {
		return apperror.New(apperror.CodeInvalidInput, "too many roles").WithField("role_ids")
	}
	want := map[string]bool{}
	for _, id := range roleIDs {
		want[id] = true
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Lock(ctx, "user-roles:"+userID); err != nil { // two saves at once would race on the diff
			return err
		}
		target, err := policy.Reachable(ctx, uow, userID)
		if err != nil {
			return err
		}
		for id := range want {
			if _, err := s.editable(ctx, uow, id, false); err != nil {
				return err
			}
		}
		for _, r := range target.Roles {
			if want[r.ID] {
				delete(want, r.ID) // already held
			} else if err := uow.Roles().Unassign(ctx, target.ID, r.ID); err != nil {
				return err
			}
		}
		for id := range want {
			if err := uow.Roles().Assign(ctx, target.ID, id); err != nil {
				return err
			}
		}
		return policy.Audit(ctx, uow, "user.roles", target.ID, map[string]any{"role_ids": roleIDs})
	})
}

// editable loads a role the API may change. No transaction: nothing here can
// make a role protected, so the answer cannot go stale before the write.
func (s *Service) editable(ctx context.Context, uow output.IdentityStore, id string, members bool) (*domain.Role, error) {
	if err := validate.ID(id); err != nil {
		return nil, err
	}
	role, err := uow.Roles().FindByID(ctx, id, members)
	if err != nil {
		return nil, err
	}
	if role.Protected() {
		return nil, apperror.New(apperror.CodeForbidden, "a superuser role cannot be managed here")
	}
	return role, nil
}
