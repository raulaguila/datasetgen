package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func input(t *testing.T, text string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestReadBootstrapKeepsPasswordExact(t *testing.T) {
	cmd, err := readBootstrap(input(t, "Ana\nana@acme.io\n pass word \n pass word "), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "Ana" || cmd.Email != "ana@acme.io" || cmd.Password != " pass word " {
		t.Fatalf("got %+v", cmd)
	}
}

func TestNoArgumentMeansBootstrap(t *testing.T) {
	if _, err := parse(nil, input(t, "Ana\nana@acme.io\nsecret-password\nsecret-password\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteAndDisableAskTheEmail(t *testing.T) {
	for _, command := range []string{"promote", "disable", "enable"} {
		if _, err := parse([]string{command}, input(t, "ana@acme.io\n"), io.Discard); err != nil {
			t.Fatal(command, err)
		}
		if _, err := parse([]string{command}, input(t, ""), io.Discard); err == nil {
			t.Fatal(command, "ran with no email")
		}
	}
}

func TestReadBootstrapRefusesBadInput(t *testing.T) {
	for name, text := range map[string]string{
		"mismatch":  "Ana\nana@acme.io\none-password\nother-password\n",
		"truncated": "Ana\nana@acme.io\n",
	} {
		if _, err := readBootstrap(input(t, text), io.Discard); err == nil {
			t.Fatal(name)
		}
	}
}

func TestParseRefusesUnknownCommands(t *testing.T) {
	for _, args := range [][]string{{"promote", "ana@acme.io"}, {"nope", "a@b.c"}, {"bootstrap", "extra"}} {
		if _, err := parse(args, input(t, ""), io.Discard); err == nil {
			t.Fatal(args)
		}
	}
}
