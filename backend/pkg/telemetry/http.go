// Package telemetry holds bounded, process-local operational counters.
package telemetry

import (
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
