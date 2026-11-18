package telemetry

import (
	"errors"
	"io"
	"testing"
)

func TestPrometheusReturnsWriteFailure(t *testing.T) {
	r, w := io.Pipe()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	err := WritePrometheus(w)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error = %v, want %v", err, io.ErrClosedPipe)
	}
}
