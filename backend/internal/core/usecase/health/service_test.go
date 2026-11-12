package health

import (
	"context"
	"testing"
	"time"
)

type countingPinger struct{ calls int }

func (p *countingPinger) Ping(context.Context) error {
	p.calls++
	return nil
}

func TestRepeatedChecksReuseTheReport(t *testing.T) {
	db := &countingPinger{}
	svc := New(db)
	for range 1000 {
		if _, err := svc.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if db.calls != 1 {
		t.Fatalf("%d database pings", db.calls)
	}
}

type blockedPinger struct {
	entered chan struct{}
	release chan struct{}
}

func (p *blockedPinger) Ping(context.Context) error {
	close(p.entered)
	<-p.release
	return nil
}

func TestChecksDoNotQueueBehindASlowProbe(t *testing.T) {
	db := &blockedPinger{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(db.release)
	svc := New(db)
	go func() { _, _ = svc.Check(context.Background()) }()
	<-db.entered

	done := make(chan string, 1)
	go func() {
		r, _ := svc.Check(context.Background())
		done <- r.Status
	}()
	select {
	case status := <-done:
		if status != "down" {
			t.Fatalf("status %q before the first probe ended", status)
		}
	case <-time.After(time.Second):
		t.Fatal("second check waited for the probe")
	}
}
