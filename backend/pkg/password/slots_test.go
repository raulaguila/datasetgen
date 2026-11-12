package password

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSlotsStayBounded(t *testing.T) {
	if cap(slots) < 1 || cap(slots) > maxHashes {
		t.Fatalf("%d slots", cap(slots))
	}
}

func TestWaitingForASlotEndsWithTheRequest(t *testing.T) {
	for range cap(slots) {
		slots <- struct{}{}
	}
	defer func() {
		for range cap(slots) {
			<-slots
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Hash(ctx, "whatever"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("kept waiting for %v", elapsed)
	}
}
