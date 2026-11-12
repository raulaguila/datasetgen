package tokenhash

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// RandomURLSafe returns a high-entropy URL-safe token (32 bytes → base64url).
func RandomURLSafe() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SHA256Bytes returns the raw SHA-256 digest of s.
func SHA256Bytes(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}
