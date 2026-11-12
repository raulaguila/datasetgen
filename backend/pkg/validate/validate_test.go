package validate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextKeepsWhatPostgresStores(t *testing.T) {
	cases := map[string]struct {
		in   string
		max  int
		want string
	}{
		"untouched":          {"Chrome/1", 10, "Chrome/1"},
		"invalid byte":       {"abc\xffdef", 10, "abcdef"},
		"NUL":                {"a\x00b", 10, "ab"},
		"cut in a character": {"aé", 2, "a"},
		"cut":                {strings.Repeat("a", 9), 3, "aaa"},
	}
	for name, c := range cases {
		got := Text(c.in, c.max)
		if got != c.want || !utf8.ValidString(got) {
			t.Errorf("%s: %q", name, got)
		}
	}
}
