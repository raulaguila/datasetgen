package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

// JobHandler runs one background job of a kind. An error retries it with backoff
// until the job's max_attempts; wrap it with PermanentJobError to stop at once.
// A handler may run more than once for one job (a lease can run out mid-run),
// so it must be safe to repeat.
type JobHandler func(ctx context.Context, payload []byte) error

// EnqueueJob queues kind with v as its JSON payload. Pass the UnitOfWork of the
// change the job follows, so both commit or neither does.
func EnqueueJob(ctx context.Context, uow output.UnitOfWork, kind string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("job %s payload: %w", kind, err)
	}
	return uow.Jobs().Enqueue(ctx, kind, payload)
}

type permanentJobError struct{ error }

func (p permanentJobError) Unwrap() error { return p.error }

// PermanentJobError marks an error that retrying cannot fix, such as a bad payload.
func PermanentJobError(err error) error { return permanentJobError{err} }

// IsPermanentJobError reports whether err, or an error it wraps, came from PermanentJobError.
func IsPermanentJobError(err error) bool { return errors.As(err, new(permanentJobError)) }
