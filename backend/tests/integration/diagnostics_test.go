package integration_test

import (
	"encoding/json"
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
	for _, path := range []string{"/health", "/metrics"} {
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
		_ = res.Body.Close()
	}
}
