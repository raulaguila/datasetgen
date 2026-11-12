package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"
	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/job"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/internal/core/usecase/jobs"
)

func TestJobsCommitWithTheirChangeAndRetry(t *testing.T) {
	ctx := context.Background()
	store := postgres.NewStore(testDB)
	kind := "test.job." + time.Now().Format("150405.000000")

	// A rolled back transaction leaves no job behind.
	rollback := errors.New("rollback")
	err := store.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := policy.EnqueueJob(ctx, uow, kind, map[string]string{"n": "gone"}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	err = store.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		return policy.EnqueueJob(ctx, uow, kind, map[string]string{"n": "kept"})
	})
	if err != nil {
		t.Fatal(err)
	}

	runs := 0
	var seen string
	w := &jobs.Worker{Repo: job.NewRepository(testDB), Clock: output.RealClock{}, Handlers: map[string]policy.JobHandler{
		kind: func(_ context.Context, payload []byte) error {
			runs++
			seen = string(payload)
			if runs == 1 {
				return errors.New("first run fails")
			}
			return nil
		},
	}}
	// Another kind is not claimed by a worker that cannot run it.
	if err := w.Tick(ctx, []string{"test.other"}); err != nil || runs != 0 {
		t.Fatalf("other kind ran: %v %d", err, runs)
	}
	if err := w.Tick(ctx, []string{kind}); err != nil || runs != 1 {
		t.Fatalf("first tick: %v %d", err, runs)
	}
	// The failed run waits for its backoff, so an immediate tick finds nothing due.
	if err := w.Tick(ctx, []string{kind}); err != nil || runs != 1 {
		t.Fatalf("retried before backoff: %v %d", err, runs)
	}
	if err := testDB.Exec(`UPDATE sys.job SET available_at = now() WHERE kind = ?`, kind).Error; err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx, []string{kind}); err != nil || runs != 2 {
		t.Fatalf("second tick: %v %d", err, runs)
	}
	var status string
	var attempts int
	if err := testDB.Raw(`SELECT status, attempts FROM sys.job WHERE kind = ?`, kind).Row().Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "done" || attempts != 1 || seen != `{"n": "kept"}` {
		t.Fatalf("status %s attempts %d payload %s", status, attempts, seen)
	}
}

func TestJobLeaseFencesAStaleWorker(t *testing.T) {
	ctx := context.Background()
	repo := job.NewRepository(testDB)
	kind := "test.lease." + time.Now().Format("150405.000000")
	if err := repo.Enqueue(ctx, kind, nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first, err := repo.Claim(ctx, []string{kind}, 1, now, time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: %v %d", err, len(first))
	}
	// The lease ran out: a second worker takes the job, and the first one's result is refused.
	second, err := repo.Claim(ctx, []string{kind}, 1, now.Add(time.Second), time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim: %v %d", err, len(second))
	}
	if err := repo.MarkDone(ctx, first[0].ID, first[0].ClaimID, now); err == nil {
		t.Fatal("stale worker marked the job")
	}
	if err := repo.MarkDone(ctx, second[0].ID, second[0].ClaimID, now); err != nil {
		t.Fatal(err)
	}
}
