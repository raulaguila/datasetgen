package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
)

type fakeRepo struct {
	jobs     []domain.Job
	kinds    []string
	done     []string
	failed   map[string]bool // id -> terminal
	attempts map[string]int
}

func (f *fakeRepo) Enqueue(context.Context, string, []byte) error { return nil }
func (f *fakeRepo) Claim(_ context.Context, kinds []string, _ int, _ time.Time, _ time.Duration) ([]domain.Job, error) {
	f.kinds = kinds
	return f.jobs, nil
}
func (f *fakeRepo) MarkDone(_ context.Context, id, _ string, _ time.Time) error {
	f.done = append(f.done, id)
	return nil
}
func (f *fakeRepo) MarkFailed(_ context.Context, id, _ string, attempts int, _ time.Time, _ string, terminal bool) error {
	if f.failed == nil {
		f.failed, f.attempts = map[string]bool{}, map[string]int{}
	}
	f.failed[id], f.attempts[id] = terminal, attempts
	return nil
}

type clock struct{}

func (clock) Now() time.Time { return time.Unix(0, 0) }

func TestTickMarksOutcomes(t *testing.T) {
	repo := &fakeRepo{jobs: []domain.Job{
		{ID: "ok", Kind: "k", MaxAttempts: 5},
		{ID: "retry", Kind: "fail", MaxAttempts: 5},
		{ID: "last", Kind: "fail", Attempts: 4, MaxAttempts: 5},
		{ID: "bad", Kind: "permanent", MaxAttempts: 5},
		{ID: "panic", Kind: "panic", MaxAttempts: 5},
	}}
	w := &Worker{Repo: repo, Clock: clock{}, Handlers: map[string]policy.JobHandler{
		"k":         func(context.Context, []byte) error { return nil },
		"fail":      func(context.Context, []byte) error { return errors.New("down") },
		"permanent": func(context.Context, []byte) error { return policy.PermanentJobError(errors.New("bad payload")) },
		"panic":     func(context.Context, []byte) error { panic("boom") },
	}}
	if err := w.Tick(context.Background(), []string{"k"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.done) != 1 || repo.done[0] != "ok" {
		t.Fatalf("done %v", repo.done)
	}
	want := map[string]bool{"retry": false, "last": true, "bad": true, "panic": false}
	for id, terminal := range want {
		if got, ok := repo.failed[id]; !ok || got != terminal {
			t.Errorf("%s: terminal %v (recorded %v), want %v", id, got, ok, terminal)
		}
	}
	if repo.attempts["last"] != 5 {
		t.Errorf("attempts %d", repo.attempts["last"])
	}
}
