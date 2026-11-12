package shared

// ProtectedIDs selects the users the API may not manage: members of a superuser
// or wildcard role. It must agree with domain.Role.Protected.
const ProtectedIDs = `SELECT ur.user_id FROM usr.usr_role r
	JOIN usr.usr_user_role ur ON ur.role_id = r.id
	WHERE r.is_superuser OR EXISTS (SELECT 1 FROM usr.usr_role_permission p WHERE p.role_id = r.id AND p.resource = '*')`

// Unprotected filters a query on usr.usr_user u down to users the API may manage.
// NOT IN over the few protected ids, not a correlated NOT EXISTS: the planner
// estimates the anti join at one row and then sorts every user for a page.
const Unprotected = `u.id NOT IN (` + ProtectedIDs + `)`
