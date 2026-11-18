// Package telemetry holds bounded, process-local operational counters.
package telemetry

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

var (
	count, serverErrors, durationNS atomic.Int64
	latencyBuckets                  [8]atomic.Int64
	bounds                          = [...]int64{5, 10, 50, 100, 500, 1000, 5000}
)

func ObserveRequest(d time.Duration, status int) {
	count.Add(1)
	durationNS.Add(d.Nanoseconds())
	if status >= 500 {
		serverErrors.Add(1)
	}
	bucket := len(bounds)
	for i, limit := range bounds {
		if d <= time.Duration(limit)*time.Millisecond {
			bucket = i
			break
		}
	}
	latencyBuckets[bucket].Add(1)
}

func HTTPStats() any {
	buckets := make([]int64, len(latencyBuckets))
	for i := range buckets {
		buckets[i] = latencyBuckets[i].Load()
	}
	return struct {
		Count, ServerErrors, DurationNS int64
		BucketUpperMS                   []int64
		BucketCounts                    []int64
	}{count.Load(), serverErrors.Load(), durationNS.Load(), bounds[:], buckets}
}

// WritePrometheus writes the HTTP counters in the Prometheus text format, with
// the latency buckets made cumulative as a histogram needs.
func WritePrometheus(w io.Writer) error {
	const counters = "# HELP dataset_http_requests_total HTTP requests served.\n" +
		"# TYPE dataset_http_requests_total counter\n" +
		"dataset_http_requests_total %d\n" +
		"# HELP dataset_http_server_errors_total HTTP answers with a 5xx status.\n" +
		"# TYPE dataset_http_server_errors_total counter\n" +
		"dataset_http_server_errors_total %d\n" +
		"# HELP dataset_http_request_duration_seconds Time to answer HTTP requests.\n" +
		"# TYPE dataset_http_request_duration_seconds histogram\n"
	if _, err := fmt.Fprintf(w, counters, count.Load(), serverErrors.Load()); err != nil {
		return err
	}
	var sum int64
	for i, limit := range bounds {
		sum += latencyBuckets[i].Load()
		if _, err := fmt.Fprintf(w, "dataset_http_request_duration_seconds_bucket{le=\"%g\"} %d\n", float64(limit)/1000, sum); err != nil {
			return err
		}
	}
	sum += latencyBuckets[len(bounds)].Load()
	const totals = "dataset_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n" +
		"dataset_http_request_duration_seconds_sum %g\n" +
		"dataset_http_request_duration_seconds_count %d\n"
	_, err := fmt.Fprintf(w, totals, sum, float64(durationNS.Load())/1e9, sum)
	return err
}
