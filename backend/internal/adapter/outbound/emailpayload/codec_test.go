package emailpayload

import (
	"bytes"
	"testing"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

func TestCodecSealsAndAuthenticatesToken(t *testing.T) {
	codec := New(bytes.Repeat([]byte{1}, 32))
	p := output.EmailToken{Email: "a@acme.io", Token: "private-token"}
	raw, err := codec.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(p.Token)) {
		t.Fatal("plaintext token persisted")
	}
	got, err := codec.Decode(raw)
	if err != nil || got != p {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	if _, err := New(bytes.Repeat([]byte{2}, 32)).Decode(raw); err == nil {
		t.Fatal("wrong key accepted")
	}
	for _, bad := range []string{`{`, `{}`, `{"email":"a@acme.io","token":"plaintext"}`} {
		if _, err := codec.Decode([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
