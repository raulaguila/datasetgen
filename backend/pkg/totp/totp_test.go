package totp

import (
	"encoding/base32"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA1 with the 20-byte key "12345678901234567890" (8 digits there; the last 6 here).
func TestCodesMatchTheRFC(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		got, err := Code(secret, Step(time.Unix(unix, 0)))
		if err != nil || got != want {
			t.Errorf("t=%d: %s, %v; want %s", unix, got, err, want)
		}
	}
}

func TestMatchTakesTheNeighbouringSteps(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	prev, _ := Code(secret, Step(now)-1)
	if s, ok := Match(secret, prev[:3]+" "+prev[3:], now); !ok || s != Step(now)-1 {
		t.Fatalf("previous step: %d, %v", s, ok)
	}
	old, _ := Code(secret, Step(now)-2)
	if _, ok := Match(secret, old, now); ok {
		t.Fatal("a code two steps old matched")
	}
	if _, ok := Match(secret, "12345", now); ok {
		t.Fatal("a short code matched")
	}
}
