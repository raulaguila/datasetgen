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
	// The same numbers for a Prometheus scrape: HTTP, pool, queues and memory.
	mux.HandleFunc("GET /metrics/prometheus", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		db, err := d.resources.PostgresDB.DB()
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var q struct {
			OutboxPending int64
			OutboxFailed  int64
			JobsPending   int64
			JobsFailed    int64
		}
		err = d.resources.PostgresDB.WithContext(ctx).Raw(`SELECT
			(SELECT count(*) FROM sys.outbox WHERE status IN ('pending','processing')) AS outbox_pending,
			(SELECT count(*) FROM sys.outbox WHERE status = 'failed') AS outbox_failed,
			(SELECT count(*) FROM sys.job WHERE status IN ('pending','processing')) AS jobs_pending,
			(SELECT count(*) FROM sys.job WHERE status = 'failed') AS jobs_failed`).Find(&q).Error
		if err != nil {
			http.Error(w, "queues unavailable", http.StatusServiceUnavailable)
			return
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		stats := db.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		telemetry.WritePrometheus(w)
		for _, g := range []struct {
			name, help string
			value      float64
		}{
			{"dataset_db_open_connections", "Open database connections.", float64(stats.OpenConnections)},
			{"dataset_db_in_use_connections", "Database connections in use.", float64(stats.InUse)},
			{"dataset_db_wait_count", "Waits for a free database connection, since start.", float64(stats.WaitCount)},
			{"dataset_outbox_pending", "Emails waiting to be sent.", float64(q.OutboxPending)},
			{"dataset_outbox_failed", "Emails that gave up.", float64(q.OutboxFailed)},
			{"dataset_jobs_pending", "Background jobs waiting or running.", float64(q.JobsPending)},
			{"dataset_jobs_failed", "Background jobs that gave up.", float64(q.JobsFailed)},
			{"dataset_heap_bytes", "Heap in use.", float64(mem.HeapAlloc)},
			{"dataset_goroutines", "Goroutines.", float64(runtime.NumGoroutine())},
		} {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", g.name, g.help, g.name, g.name, g.value)
		}
	})
	return mux
}
