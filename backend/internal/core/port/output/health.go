package output

import "context"

// Pinger is a dependency that can be probed for health.
type Pinger interface {
	Ping(ctx context.Context) error
}
