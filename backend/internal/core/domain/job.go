package domain

// Job is one claimed background job (sys.job).
type Job struct {
	ID          string
	ClaimID     string // changes whenever a worker leases the job
	RequestID   string // of the request that queued it, empty outside HTTP
	Kind        string
	Payload     []byte
	Attempts    int // failed runs so far
	MaxAttempts int
}
