package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres"

	productrepo "github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/product"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/product"
	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestProductListPagesSearchesAndSorts(t *testing.T) {
	if err := testDB.Exec("DELETE FROM app.product").Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	for _, p := range []struct {
		name, price string
		enabled     bool
	}{{"100% cotton", "3", true}, {"Cotton", "1", true}, {"Wool", "2", true}, {"Hidden", "4", false}} {
		enabled := p.enabled
		if _, err := svc.Create(ctx, input.CreateProductCommand{Name: p.name, Price: p.price, Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	names := func(f domain.ProductFilter) ([]string, int64) {
		t.Helper()
		items, total, err := svc.List(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(items))
		for _, p := range items {
			out = append(out, p.Name)
		}
		return out, total
	}
	all := domain.NewPage(1, 100)

	if got, total := names(domain.ProductFilter{Page: domain.NewPage(2, 3), Sort: "price", Order: "asc"}); total != 4 || len(got) != 1 || got[0] != "Hidden" {
		t.Fatalf("page: %v of %d", got, total)
	}
	if got, total := names(domain.ProductFilter{Page: all, Search: "COTTON", Sort: "name", Order: "asc"}); total != 2 || got[0] != "100% cotton" {
		t.Fatalf("search: %v of %d", got, total)
	}
	if got, total := names(domain.ProductFilter{Page: all, Search: "%"}); total != 1 || got[0] != "100% cotton" {
		t.Fatalf("wildcard must be literal: %v of %d", got, total)
	}
	disabled := false
	if got, total := names(domain.ProductFilter{Page: all, Enabled: &disabled}); total != 1 || got[0] != "Hidden" {
		t.Fatalf("enabled filter: %v of %d", got, total)
	}
	if got, total := names(domain.ProductFilter{Page: domain.NewPage(9, 100)}); total != 4 || len(got) != 0 {
		t.Fatalf("past the end: %v of %d", got, total)
	}

	_, _, err := svc.List(ctx, domain.ProductFilter{Page: all, Sort: "password_hash"})
	wantCode(t, err, apperror.CodeInvalidInput)
	_, _, err = svc.List(ctx, domain.ProductFilter{Page: all, Order: "desc; DROP TABLE app.product"})
	wantCode(t, err, apperror.CodeInvalidInput)
	_, _, err = svc.List(ctx, domain.ProductFilter{Page: all, Search: "a\x00b"})
	wantCode(t, err, apperror.CodeInvalidInput)

	options, err := svc.Options(ctx)
	if err != nil || len(options) != 3 || options[0].Name != "100% cotton" || options[0].ID == "" {
		t.Fatalf("options: %+v %v", options, err)
	}
}

func TestProductUpdateReturnsNewTimestamp(t *testing.T) {
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	p, err := svc.Create(ctx, input.CreateProductCommand{Name: "Stamp", Price: "1"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Stamped"
	updated, err := svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.After(p.CreatedAt) {
		t.Fatalf("updated_at stale: %v vs %v", updated.UpdatedAt, p.CreatedAt)
	}
}

func TestProductUpdateKeepsFieldsItDoesNotCarry(t *testing.T) {
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	p, err := svc.Create(ctx, input.CreateProductCommand{Name: "Patch", Price: "5"})
	if err != nil {
		t.Fatal(err)
	}
	name, price := "Patched", "7.5"
	if _, err := svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Name: &name}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Price: &price})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Patched" || got.Price != "7.50" || !got.Enabled {
		t.Fatalf("%+v", got)
	}
	missing := "11111111-1111-4111-8111-111111111111"
	_, err = svc.Update(ctx, input.UpdateProductCommand{ID: missing, Name: &name})
	wantCode(t, err, apperror.CodeNotFound)
}

func TestProductPriceSortUsesNumericColumn(t *testing.T) {
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	for _, p := range []struct{ name, price string }{{"Sort audit two", "2"}, {"Sort audit ten", "10"}, {"Sort audit hundred", "100"}} {
		if _, err := svc.Create(ctx, input.CreateProductCommand{Name: p.name, Price: p.price}); err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range []string{"asc", "desc"} {
		items, _, err := svc.List(ctx, domain.ProductFilter{Page: domain.NewPage(1, 20), Search: "Sort audit", Sort: "price", Order: order})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"2.00", "10.00", "100.00"}
		if order == "desc" {
			want = []string{"100.00", "10.00", "2.00"}
		}
		if len(items) != 3 {
			t.Fatalf("items=%d", len(items))
		}
		for i, p := range items {
			if p.Price != want[i] {
				t.Fatalf("%s: item %d price=%s", order, i, p.Price)
			}
		}
	}
}

func TestProductCursorTraversesTiesWithoutCounting(t *testing.T) {
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	if err := testDB.Exec("DELETE FROM app.product").Error; err != nil {
		t.Fatal(err)
	}
	for range 7 {
		if _, err := svc.Create(ctx, input.CreateProductCommand{Name: "Cursor test", Price: "2"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.Exec("UPDATE app.product SET created_at = '2026-01-01T00:00:00Z'").Error; err != nil {
		t.Fatal(err)
	}
	filter := domain.ProductFilter{Page: domain.NewPage(1, 2), Cursor: true}
	seen := map[string]bool{}
	for range 5 {
		items, total, err := svc.List(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if total != -1 {
			t.Fatal("cursor performed total count")
		}
		for _, p := range items {
			if seen[p.ID] {
				t.Fatalf("duplicate %s", p.ID)
			}
			seen[p.ID] = true
		}
		if len(items) < 2 {
			break
		}
		last := items[len(items)-1]
		filter.After = &domain.ProductCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if len(seen) != 7 {
		t.Fatalf("read %d of 7", len(seen))
	}
	filter.Sort = "price"
	if _, _, err := svc.List(ctx, filter); !apperror.IsCode(err, apperror.CodeInvalidInput) {
		t.Fatalf("unsupported cursor sort: %v", err)
	}
}

func TestProductChangesAreAudited(t *testing.T) {
	ctx := context.Background()
	store := postgres.NewStore(testDB)
	svc := product.New(productrepo.NewRepository(testDB), store)
	p, err := svc.Create(ctx, input.CreateProductCommand{Name: "Audited " + t.Name(), Price: "9.5"})
	if err != nil {
		t.Fatal(err)
	}
	price := "10"
	if _, err := svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Price: &price}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMany(ctx, []string{p.ID}); err != nil {
		t.Fatal(err)
	}
	records, _, err := store.Audit().List(ctx, domain.AuditFilter{Action: "product.", Page: domain.NewPage(1, 100)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range records {
		if r.TargetID == p.ID {
			got = append(got, r.Action)
			// The row is gone, so every label comes from the name the delete recorded.
			if r.TargetLabel != p.Name {
				t.Errorf("%s label %q, want %q", r.Action, r.TargetLabel, p.Name)
			}
		}
	}
	if want := []string{"product.delete", "product.update", "product.create"}; !slices.Equal(got, want) {
		t.Fatalf("audit %v, want %v", got, want)
	}
}

func TestProductUpdateRefusesStaleVersion(t *testing.T) {
	ctx := context.Background()
	svc := product.New(productrepo.NewRepository(testDB), postgres.NewStore(testDB))
	p, err := svc.Create(ctx, input.CreateProductCommand{Name: "Versioned", Price: "1"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := "First", "Second"
	read := p.Version
	got, err := svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Name: &first, Version: &read})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != read+1 {
		t.Fatalf("version %d, want %d", got.Version, read+1)
	}
	// A second editor still holding the old version loses, and the row keeps the first edit.
	_, err = svc.Update(ctx, input.UpdateProductCommand{ID: p.ID, Name: &second, Version: &read})
	wantCode(t, err, apperror.CodeConflict)
	if now, _ := svc.Get(ctx, p.ID); now.Name != "First" {
		t.Fatalf("name %q", now.Name)
	}
	missing := "11111111-1111-4111-8111-111111111111"
	_, err = svc.Update(ctx, input.UpdateProductCommand{ID: missing, Name: &first, Version: &read})
	wantCode(t, err, apperror.CodeNotFound)
}
