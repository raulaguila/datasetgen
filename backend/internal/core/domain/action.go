package domain

// Action bits (ADR-004). Shared vocabulary across resources.
type Action int32

const (
	ActionCreate Action = 1 << 0 // 1
	ActionRead   Action = 1 << 1 // 2
	ActionUpdate Action = 1 << 2 // 4
	ActionDelete Action = 1 << 3 // 8
)

// Allows reports whether have includes every bit in need.
func Allows(have, need Action) bool {
	return have&need == need
}

var actionNames = map[string]Action{
	"create": ActionCreate,
	"read":   ActionRead,
	"update": ActionUpdate,
	"delete": ActionDelete,
}

// ActionNames lists the actions in a, in the order of their bits.
func ActionNames(a Action) []string {
	out := []string{}
	for _, name := range []string{"create", "read", "update", "delete"} {
		if Allows(a, actionNames[name]) {
			out = append(out, name)
		}
	}
	return out
}

// Resources the routes check, or the sentinel "*". A grant may name only these
// (chk_usr_role_permission_resource); collections and records follow membership.
const (
	ResourceAll         = "*"
	ResourceAudit       = "audit"
	ResourceCollections = "collections"
	ResourceRoles       = "roles"
	ResourceUsers       = "users"
)
