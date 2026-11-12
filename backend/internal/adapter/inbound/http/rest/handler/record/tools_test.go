package record

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
)

func TestRecordsCSVFollowsTheFormOrder(t *testing.T) {
	exp := &input.Export{
		Schema: json.RawMessage(`{"properties":{"zeta":{},"alpha":{}}}`),
		Records: []json.RawMessage{
			json.RawMessage(`{"alpha":"=1+1","zeta":"z","tags":["a","b"]}`),
			json.RawMessage(`{"alpha":2,"extra":true}`),
		},
	}
	body, err := recordsCSV(exp)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimPrefix(string(body), "\uFEFF")
	want := "zeta,alpha,extra,tags\nz,'=1+1,,\"[\"\"a\"\",\"\"b\"\"]\"\n,2,true,\n"
	if got != want {
		t.Fatalf("csv:\n%q\nwant\n%q", got, want)
	}
	if lines := string(recordsJSONL(exp)); strings.Count(lines, "\n") != 2 || !strings.HasPrefix(lines, `{"alpha":"=1+1"`) {
		t.Fatalf("jsonl: %q", lines)
	}
}
