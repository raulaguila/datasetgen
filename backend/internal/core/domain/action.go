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

// ParseActions is the inverse of ActionNames. It reports false on an unknown name.
func ParseActions(names []string) (Action, bool) {
	var out Action
	for _, name := range names {
		action, ok := actionNames[name]
		if !ok {
			return 0, false
		}
		out |= action
	}
	return out, true
}

// Resource names are catalog codes (usr_resource) or the sentinel "*".
const (
	ResourceAll         = "*"
	ResourceAudit       = "audit"
	ResourceProducts    = "products"
	ResourceCollections = "collections"
	ResourceRecords     = "records"
	ResourceRoles       = "roles"
	ResourceUsers       = "users"
)

// Resource is an entry of the catalog that grants may name.
type Resource struct {
	Code        string
	Description string
	Actions     Action // the actions its routes check; grants outside them are refused
}
