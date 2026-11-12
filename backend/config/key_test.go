package config

import (
	"crypto/ed25519"
	"testing"

	"github.com/godeh/dotenvgo"
)

// A throwaway key made with `openssl genpkey -algorithm ed25519 | base64 -w0`, as
// config/env.sh does; nothing else uses it.
const opensslKey = "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1DNENBUUF3QlFZREsyVndCQ0lFSURBTFlqTjlHSDlxUkxzNE85UkhHWVk0Rmd0WW41enN6c2prRHZEMU9aRmwKLS0tLS1FTkQgUFJJVkFURSBLRVktLS0tLQo="

func TestJWTKeyReadsOpenSSLOutput(t *testing.T) {
	var cfg struct {
		Key ed25519.PrivateKey `env:"TEST_JWT_KEY"`
	}
	t.Setenv("TEST_JWT_KEY", opensslKey)
	if err := dotenvgo.Load(&cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Key) != ed25519.PrivateKeySize {
		t.Fatalf("key of %d bytes", len(cfg.Key))
	}
	var optional struct {
		Key ed25519.PrivateKey `env:"TEST_JWT_KEY_UNSET" default:""`
	}
	if err := dotenvgo.Load(&optional); err != nil || optional.Key != nil {
		t.Fatalf("an unset optional key: %v %v", optional.Key, err)
	}
	t.Setenv("TEST_JWT_KEY", "bm90IGEga2V5")
	if err := dotenvgo.Load(&cfg); err == nil {
		t.Fatal("a non-key was accepted")
	}
}
