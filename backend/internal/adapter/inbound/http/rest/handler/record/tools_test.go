package record

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/input"
)

func TestRecordsCSVFollowsTheFormOrder(t *testing.T) {
	recs := []json.RawMessage{
		json.RawMessage(`{"alpha":"=1+1","zeta":"z","tags":["a","b"]}`),
		json.RawMessage(`{"alpha":2,"extra":true}`),
	}
	exp := &input.Export{
		Schema: json.RawMessage(`{"properties":{"zeta":{},"alpha":{}}}`),
		Keys:   []string{"alpha", "extra", "tags", "zeta"},
		Each: func(fn func(json.RawMessage) error) error {
			for _, r := range recs {
				if err := fn(r); err != nil {
					return err
				}
			}
			return nil
		},
	}
	write := func(f func(*bufio.Writer, *input.Export) error) string {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := f(w, exp); err != nil {
			t.Fatal(err)
		}
		_ = w.Flush()
		return buf.String()
	}
	got := strings.TrimPrefix(write(writeCSV), "\uFEFF")
	want := "zeta,alpha,extra,tags\nz,'=1+1,,\"[\"\"a\"\",\"\"b\"\"]\"\n,2,true,\n"
	if got != want {
		t.Fatalf("csv:\n%q\nwant\n%q", got, want)
	}
	if lines := write(writeJSONL); strings.Count(lines, "\n") != 2 || !strings.HasPrefix(lines, `{"alpha":"=1+1"`) {
		t.Fatalf("jsonl: %q", lines)
	}
}
