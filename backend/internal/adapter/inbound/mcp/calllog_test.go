package mcp

import "testing"

func TestCutKeepsWholeCharacters(t *testing.T) {
	if got := cut("abc", 5); got != "abc" {
		t.Fatalf("short: %q", got)
	}
	if got := cut("aç", 2); got != "a" { // ç is 2 bytes: cut in its middle drops it
		t.Fatalf("split: %q", got)
	}
}
