package integration_test

import (
	"encoding/json"
	"io"
	"strings"
	"net/http"
	"testing"

	"github.com/raulaguila/dataset/internal/infra"
)

func TestPrivateDiagnostics(t *testing.T) {
	resources := &infra.Resources{PostgresDB: testDB}
	d, stop, err := resources.StartDiagnostics("127.0.0.1:0", true)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	d.Heartbeat()
	for _, path := range []string{"/health", "/metrics", "/metrics/prometheus"} {
		res, err := http.Get("http://" + d.Address + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			_ = res.Body.Close()
			t.Fatalf("%s status %d", path, res.StatusCode)
		}
		if path == "/metrics" {
			var data map[string]any
			if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"http", "hashing", "database", "outbox", "heap_bytes"} {
				if _, ok := data[key]; !ok {
					t.Fatalf("missing %s", key)
				}
			}
		}
		if path == "/metrics/prometheus" {
			body, _ := io.ReadAll(res.Body)
			for _, want := range []string{"dataset_http_requests_total", `dataset_http_request_duration_seconds_bucket{le="+Inf"}`, "dataset_jobs_pending"} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("prometheus output misses %s:\n%s", want, body)
				}
			}
		}
		_ = res.Body.Close()
	}
}
