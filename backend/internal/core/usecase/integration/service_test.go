package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

// A receiver recomputes HMAC-SHA256 of "<t>.<body>" with the secret.
func TestSignMatchesWhatAReceiverChecks(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`{"event":"ping"}`)
	mac := hmac.New(sha256.New, []byte("whsec_x"))
	mac.Write([]byte("1700000000." + string(body)))
	want := "t=1700000000,v1=" + hex.EncodeToString(mac.Sum(nil))
	if got := Sign("whsec_x", at, body); got != want {
		t.Fatalf("Sign = %s, want %s", got, want)
	}
}
