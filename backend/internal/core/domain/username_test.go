package domain

import (
	"os"
	"strings"
	"testing"
)

func TestValidUsername(t *testing.T) {
	for name, want := range map[string]bool{
		"ana": true, "ana-silva": true, "a1": true, "x": true, strings.Repeat("a", 39): true,
		"": false, "-ana": false, "ana-": false, "ana--silva": false, "Ana": false, "ana_silva": false,
		"ana.silva": false, strings.Repeat("a", 40): false, "admin": false, "collections": false,
	} {
		if got := ValidUsername(name); got != want {
			t.Errorf("ValidUsername(%q) = %v, want %v", name, got, want)
		}
	}
}

// The database refuses the same names: its list must match.
func TestReservedUsernamesMatchTheMigration(t *testing.T) {
	sql, err := os.ReadFile("../../../deploy/db/migrations/000002_identity.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range reservedUsernames {
		if !strings.Contains(string(sql), "'"+name+"'") {
			t.Errorf("%q is reserved here but not in the migration", name)
		}
	}
	if got := strings.Count(string(sql)[strings.Index(string(sql), "ARRAY[")+6:strings.Index(string(sql), "])")], "'") / 2; got != len(reservedUsernames) {
		t.Errorf("migration reserves %d names, Go %d", got, len(reservedUsernames))
	}
}
