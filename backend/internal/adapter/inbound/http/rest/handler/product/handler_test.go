package product

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
)

type emptyProducts struct{ input.ProductService }

func (emptyProducts) List(context.Context, domain.ProductFilter) ([]domain.Product, int64, error) {
	return nil, 0, nil
}

func TestPaginationPreservesNumberedResponseAndOmitsCursorTotals(t *testing.T) {
	app := fiber.New()
	app.Get("/", New(emptyProducts{}).List)
	for _, cursor := range []bool{false, true} {
		path := "/"
		if cursor {
			path += "?cursor=start"
		}
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Items      []any          `json:"items"`
			Pagination map[string]any `json:"pagination"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body.Items == nil {
			t.Fatal("empty list must be [] rather than null")
		}
		p := body.Pagination
		if p["limit"] != float64(20) {
			t.Fatalf("limit: %v", p)
		}
		if cursor {
			if _, ok := p["next_cursor"]; ok {
				t.Fatalf("last cursor page: %v", p)
			}
			if _, ok := p["total_items"]; ok {
				t.Fatalf("cursor fabricated total: %v", p)
			}
		} else {
			if p["page"] != float64(1) || p["total_items"] != float64(0) || p["total_pages"] != float64(0) {
				t.Fatalf("numbered response changed: %v", p)
			}
			if _, ok := p["next_cursor"]; ok {
				t.Fatalf("cursor in numbered response: %v", p)
			}
		}
	}
}
