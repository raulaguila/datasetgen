package domain

import "time"

const (
	OutboxStatusPending = "pending"
	OutboxStatusFailed  = "failed"
)

// OutboxMessage is a durable email intent row (sys.outbox).
type OutboxMessage struct {
	ID          string
	ClaimID     string // changes whenever a worker leases the message
	RequestID   string // of the request that queued it, empty outside HTTP
	Purpose     string
	Payload     []byte
	Status      string
	Attempts    int
	AvailableAt time.Time
	CreatedAt   time.Time
	ProcessedAt *time.Time
	LastError   *string
}
