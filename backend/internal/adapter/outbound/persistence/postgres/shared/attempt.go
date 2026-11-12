package shared

// CountedFailure excludes refusals that tested no password: counting them would
// let any repeated request keep a victim blocked forever. Literal on purpose: it
// must match the partial indexes of migration 000002 for the planner to use them.
const CountedFailure = `success = false AND failure_kind NOT IN ('rate_limited_email', 'rate_limited_ip', 'account_locked')`
