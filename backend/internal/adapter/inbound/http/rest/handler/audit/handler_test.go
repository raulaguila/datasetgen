package audit

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
)

func TestCSVDisarmsFormulas(t *testing.T) {
	body, err := auditCSV([]domain.AuditRecord{{
		CreatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), Action: "user.invite",
		ActorName: `=HYPERLINK("http://x")`, ActorEmail: "a@acme.io", TargetLabel: "+1", Details: map[string]any{"n": 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %v %v", rows, err)
	}
	got := rows[1]
	if got[0] != "2026-11-08T12:00:00Z" || got[2] != `'=HYPERLINK("http://x")` || got[3] != "a@acme.io" || got[5] != "'+1" || got[7] != `{"n":1}` {
		t.Fatalf("row %q", got)
	}
}
