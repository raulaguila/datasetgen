package mapper

import (
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
)

func TestPaginationCountsPartialPages(t *testing.T) {
	for total, want := range map[int64]int64{0: 0, 1: 1, 20: 1, 21: 2, 40: 2} {
		if got := Pagination(domain.NewPage(1, 20), total).TotalPages; got != want {
			t.Errorf("total %d: got %d pages, want %d", total, got, want)
		}
	}
}

func TestPermissionsCollapsesToTheWildcard(t *testing.T) {
	all := domain.ActionCreate | domain.ActionRead | domain.ActionUpdate | domain.ActionDelete
	got := Permissions(map[string]domain.Action{domain.ResourceAll: all, "users": all, "roles": domain.ActionRead})
	if len(got) != 1 || len(got["*"]) != 4 {
		t.Fatalf("got %v, want only *", got)
	}
	if got := Permissions(map[string]domain.Action{"users": domain.ActionRead}); len(got) != 1 || got["users"][0] != "read" {
		t.Fatalf("got %v", got)
	}
}
