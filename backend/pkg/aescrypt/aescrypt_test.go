package aescrypt

import (
	"errors"
	"strings"
	"testing"
)

func mustKey(t *testing.T, fill string) []byte {
	t.Helper()
	key, err := ParseKey(strings.Repeat(fill, 64))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := mustKey(t, "a")
	sealed, err := Encrypt(key, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "secret") {
		t.Fatal("plaintext visible in ciphertext")
	}
	got, err := Decrypt(key, sealed)
	if err != nil || got != "secret" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDecryptRejectsWrongKeyAndGarbage(t *testing.T) {
	sealed, err := Encrypt(mustKey(t, "a"), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(mustKey(t, "b"), sealed); err == nil {
		t.Fatal("wrong key must fail")
	}
	if _, err := Decrypt(mustKey(t, "a"), "not-hex"); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestParseKeyRejectsBadLength(t *testing.T) {
	if _, err := ParseKey("abcd"); !errors.Is(err, ErrKeyLength) {
		t.Fatalf("got %v", err)
	}
}
