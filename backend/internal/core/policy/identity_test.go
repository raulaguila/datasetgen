package policy

import (
	"strings"
	"testing"
)

func TestIdentityRules(t *testing.T) {
	name, email, err := ValidateIdentity(" Ana ", " ANA@example.org ")
	if err != nil || name != "Ana" || email != "ana@example.org" {
		t.Fatalf("%q %q %v", name, email, err)
	}
	for _, bad := range []string{"a", strings.Repeat("x", 81), "Ana\nMaria"} {
		if _, _, err := ValidateIdentity(bad, "a@acme.io"); err == nil {
			t.Fatalf("accepted name %q", bad)
		}
	}
	for _, bad := range []string{"bad", "Name <a@acme.io>", "a\x00@acme.io"} {
		if _, _, err := ValidateIdentity("Ana", bad); err == nil {
			t.Fatalf("accepted email %q", bad)
		}
	}
	if ValidatePassword("1234567") == nil || ValidatePassword(strings.Repeat("x", 129)) == nil {
		t.Fatal("bad password length accepted")
	}
	if ValidatePassword("long enough") != nil {
		t.Fatal("valid password rejected")
	}
}

func TestPasswordRefusesTheFirstGuesses(t *testing.T) {
	for _, bad := range []string{"short", "Password123", "12345678", "aaaaaaaaaa", "éééééééé"} {
		if err := ValidatePassword(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, own := range []string{"Ana Souza", "ana.souza@acme.io", "ANA.SOUZA"} {
		if err := ValidatePasswordFor(own, "Ana Souza", "ana.souza@acme.io"); err == nil {
			t.Errorf("%q accepted for its own account", own)
		}
	}
	if err := ValidatePasswordFor("correct horse battery", "Ana Souza", "ana.souza@acme.io"); err != nil {
		t.Fatal(err)
	}
}
