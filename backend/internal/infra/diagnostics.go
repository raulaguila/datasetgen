package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/raulaguila/dataset/pkg/password"
	"github.com/raulaguila/dataset/pkg/telemetry"
)

// Diagnostics is separate from the public API. It defaults to container loopback
// and exposes no request bodies, credentials, query text or user identifiers.
type Diagnostics struct {
	Address   string // actual listener address, including an assigned ephemeral port
	resources *Resources
	worker    bool
	lastTick  atomic.Int64
}

func (d *Diagnostics) Heartbeat() { d.lastTick.Store(time.Now().UnixNano()) }

func (r *Resources) StartDiagnostics(address string, worker bool) (*Diagnostics, func(), error) {
	d := &Diagnostics{resources: r, worker: worker}
	d.Heartbeat()
	if address == "" {
		return d, func() {}, nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("diagnostics listener: %w", err)
	}
	d.Address = listener.Addr().String()
	server := &http.Server{Handler: d.handler(), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return d, func() { _ = server.Close() }, nil
}

func (d *Diagnostics) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		db, err := d.resources.PostgresDB.DB()
		if err != nil || db.PingContext(ctx) != nil || (d.worker && time.Since(time.Unix(0, d.lastTick.Load())) > time.Minute) {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		db, err := d.resources.PostgresDB.DB()
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var queue struct {
			Pending       int64
			Failed        int64
			OldestSeconds float64
		}
		err = d.resources.PostgresDB.WithContext(ctx).Raw(`SELECT count(*) FILTER (WHERE status IN ('pending','processing')) AS pending, count(*) FILTER (WHERE status = 'failed') AS failed, COALESCE(EXTRACT(EPOCH FROM (now()-min(created_at) FILTER (WHERE status IN ('pending','processing')))),0)::float8 AS oldest_seconds FROM sys.outbox`).Find(&queue).Error
		if err != nil {
			http.Error(w, "outbox unavailable", http.StatusServiceUnavailable)
			return
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"http": telemetry.HTTPStats(), "hashing": password.Stats(), "database": db.Stats(), "outbox": queue, "heap_bytes": mem.HeapAlloc, "heap_objects": mem.HeapObjects, "gc_cycles": mem.NumGC, "goroutines": runtime.NumGoroutine()})
	})
	return mux
}
