// Package jobs runs background jobs from sys.job. A module registers a
// policy.JobHandler per kind and enqueues with policy.EnqueueJob inside its own transaction.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/validate"
)

const (
	defaultBatch   = 10
	defaultPoll    = 2 * time.Second
	defaultLease   = 5 * time.Minute // must outlast the slowest handler times the batch
	maxBackoffSecs = 15 * 60
	maxErrorBytes  = 500
	markTimeout    = 5 * time.Second
)

// Worker claims due jobs of the kinds in Handlers and runs them.
// shortcut: polls every Poll per instance; LISTEN/NOTIFY when jobs must start within milliseconds.
type Worker struct {
	Repo     output.JobRepository
	Handlers map[string]policy.JobHandler
	Log      output.Logger
	Clock    output.Clock
	Batch    int
	Poll     time.Duration
	Lease    time.Duration
	// Periodic, when set, runs at most once per Every, between ticks.
	Periodic func(ctx context.Context) error
	Every    time.Duration
	lastRun  time.Time
}

func (w *Worker) Run(ctx context.Context) error {
	if w.Poll <= 0 {
		w.Poll = defaultPoll
	}
	kinds := slices.Sorted(maps.Keys(w.Handlers))
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	output.Log(w.Log).Info("Job worker started", slog.Any("kinds", kinds))
	for {
		if err := w.Tick(ctx, kinds); err != nil && ctx.Err() == nil {
			output.Log(w.Log).Warn("Job tick error", slog.String("error", err.Error()))
		}
		if now := w.Clock.Now(); w.Periodic != nil && now.Sub(w.lastRun) >= w.Every {
			w.lastRun = now
			if err := w.Periodic(ctx); err != nil && ctx.Err() == nil {
				output.Log(w.Log).Warn("Periodic job error", slog.String("error", err.Error()))
			}
		}
		select {
		case <-ctx.Done():
			output.Log(w.Log).Info("Job worker stopped")
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Tick claims and runs one batch.
func (w *Worker) Tick(ctx context.Context, kinds []string) error {
	jobs, err := w.Repo.Claim(ctx, kinds, w.batch(), w.Clock.Now(), w.lease())
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if ctx.Err() != nil { // shutting down: unrun jobs return when the lease ends
			return ctx.Err()
		}
		w.run(reqctx.WithRequestID(ctx, job.RequestID), job.ID, job.ClaimID, job.Kind, job.Payload, job.Attempts, job.MaxAttempts)
	}
	return nil
}

func (w *Worker) run(ctx context.Context, id, claimID, kind string, payload []byte, attempts, maxAttempts int) {
	err := w.call(ctx, kind, payload)
	mark, cancel := context.WithTimeout(context.WithoutCancel(ctx), markTimeout)
	defer cancel()
	log := output.Log(w.Log)
	if err == nil {
		if err := w.Repo.MarkDone(mark, id, claimID, w.Clock.Now()); err != nil {
			log.Warn("Job MarkDone failed", slog.String("job_id", id), slog.String("error", err.Error()))
		}
		return
	}
	attempts++
	terminal := policy.IsPermanentJobError(err) || attempts >= maxAttempts
	backoff := time.Duration(math.Min(math.Pow(2, float64(attempts)), maxBackoffSecs)) * time.Second
	msg := validate.Text(err.Error(), maxErrorBytes)
	if err := w.Repo.MarkFailed(mark, id, claimID, attempts, w.Clock.Now().Add(backoff), msg, terminal); err != nil {
		log.Warn("Job MarkFailed failed", slog.String("job_id", id), slog.String("error", err.Error()))
		return
	}
	log.Warn("Job failed",
		slog.String("job_id", id), slog.String("kind", kind), slog.String("request_id", reqctx.RequestID(ctx)),
		slog.Int("attempts", attempts), slog.Bool("terminal", terminal), slog.String("error", msg))
}

// call runs the handler, turning a panic into a failed run instead of a dead worker.
func (w *Worker) call(ctx context.Context, kind string, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	h, ok := w.Handlers[kind]
	if !ok { // Claim only returns registered kinds; kept for a caller that passes others
		return policy.PermanentJobError(fmt.Errorf("no handler for %q", kind))
	}
	return h(ctx, payload)
}

func (w *Worker) batch() int {
	if w.Batch <= 0 {
		return defaultBatch
	}
	return w.Batch
}

func (w *Worker) lease() time.Duration {
	if w.Lease <= 0 {
		return defaultLease
	}
	return w.Lease
}
