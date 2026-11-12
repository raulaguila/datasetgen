package integration_test

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/raulaguila/dataset/internal/core/usecase/admin"
)

// Includes Fiber, JWT/session validation, live RBAC and PostgreSQL queries.
// app.Test excludes TCP/TLS costs; this is not a production capacity claim.
func BenchmarkHTTPProducts(b *testing.B) {
	if err := testDB.Exec(`INSERT INTO app.product(name,price,created_at) SELECT 'Benchmark '||g, g%1000, now()-g*interval '1 second' FROM generate_series(1,10000) g`).Error; err != nil {
		b.Fatal(err)
	}
	e := newEnv(b)
	addr := e.activeUser(b)
	if err := admin.New(e.store, e.clock).Promote(context.Background(), addr); err != nil {
		b.Fatal(err)
	}
	pair, err := e.login(addr, strongPassword)
	if err != nil {
		b.Fatal(err)
	}
	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/v1/products?cursor=start&limit=20", nil)
			req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
			res, err := app.Test(req)
			if err != nil {
				b.Error(err)
				continue
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode != 200 {
				b.Errorf("status %d", res.StatusCode)
			}
		}
	})
}
