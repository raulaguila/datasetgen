package password_test

import (
	"context"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/pkg/password"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	ctx := context.Background()
	h, err := password.Hash(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := password.Verify(ctx, h, "correct horse battery")
	if err != nil || !ok {
		t.Fatalf("verify want true, got %v %v", ok, err)
	}
	ok, err = password.Verify(ctx, h, "wrong")
	if err != nil || ok {
		t.Fatalf("verify want false, got %v %v", ok, err)
	}
}

func TestVerifyRejectsUnsafePHC(t *testing.T) {
	const valid = "$argon2id$v=19$m=65536,t=3,p=4$MDEyMzQ1Njc4OWFiY2RlZg$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for _, tc := range []struct{ old, new string }{{"v=19", "v=16"}, {"m=65536", "m=-1"}, {"m=65536", "m=1048576"}, {"t=3", "t=0"}, {"t=3", "t=999"}, {"p=4", "p=0"}, {"p=4", "p=256"}, {"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", ""}} {
		if ok, err := password.Verify(context.Background(), strings.Replace(valid, tc.old, tc.new, 1), "anything"); err == nil || ok {
			t.Fatalf("accepted %s", tc.new)
		}
	}
}

func TestWasteRunsARealVerify(t *testing.T) {
	before := password.Stats()["operations"]
	password.Waste(context.Background(), "password")
	if password.Stats()["operations"] != before+1 {
		t.Fatal("Verify refused the timing hash, so Waste spent no time")
	}
}

func TestCancelledHashAndWaste(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := password.Hash(ctx, "password"); err == nil {
		t.Fatal("accepted cancelled work")
	}
	password.Waste(ctx, "password")
	if password.Stats()["active"] != 0 || password.Stats()["waiting"] != 0 {
		t.Fatal("leaked hashing slot")
	}
}
