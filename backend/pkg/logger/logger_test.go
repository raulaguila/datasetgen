package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestJSONFieldsAndLevels(t *testing.T) {
	var out bytes.Buffer
	log := New(Config{Level: "warning", Format: "json", Out: &out, Fields: map[string]any{"service": "x"}})
	log.Info("hidden")
	log.Warn("shown", slog.String("s", "v"), slog.Int("n", 7), slog.Bool("b", true), slog.Duration("d", 2*time.Millisecond))

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got["level"] != "warn" || got["message"] != "shown" || got["service"] != "x" ||
		got["s"] != "v" || got["n"] != float64(7) || got["b"] != true || got["d"] != "2ms" {
		t.Fatalf("%v", got)
	}
}

func TestRequestLogStaysCheap(t *testing.T) {
	log := New(Config{Format: "json", Out: io.Discard})
	attrs := []slog.Attr{slog.String("path", "/x"), slog.Int("status", 200), slog.Duration("d", time.Millisecond)}
	// Interface() costs 14 allocations here. Not zero: -race drops pooled events.
	if n := testing.AllocsPerRun(100, func() { log.Info("Request", attrs...) }); n >= 5 {
		t.Fatalf("%v allocs", n)
	}
}

func TestNilLoggerIsSilent(t *testing.T) {
	var log *Logger
	log.Error("nothing")
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]zerolog.Level{
		"": zerolog.InfoLevel, "junk": zerolog.InfoLevel, " DEBUG ": zerolog.DebugLevel, "warning": zerolog.WarnLevel,
	} {
		if got := parseLevel(in); got != want {
			t.Fatalf("%q: got %v", in, got)
		}
	}
}
