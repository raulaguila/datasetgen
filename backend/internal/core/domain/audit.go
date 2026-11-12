package domain

import "time"

// AuditEntry records an administrative change. The action names the kind of
// target ("role.update"); an empty ActorID means the operator CLI.
type AuditEntry struct {
	ActorID   string
	RequestID string
	Action    string
	TargetID  string
	Details   map[string]any
}

// AuditFilter selects one page of the audit log, newest first.
type AuditFilter struct {
	Page   Page
	Action string // prefix, such as "role." or "user.disable"
	UserID string // the actor or the target
	From   *time.Time
	To     *time.Time // exclusive
}

// AuditRecord is a stored AuditEntry with names to show beside the ids.
type AuditRecord struct {
	ID          string
	CreatedAt   time.Time
	ActorID     string
	ActorName   string
	ActorEmail  string
	RequestID   string
	Action      string
	TargetID    string
	TargetLabel string // the user's email, the role's or the product's name
	Details     map[string]any
}
