package aescrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var ErrKeyLength = errors.New("aescrypt: key must be 32 hex-encoded bytes (64 hex chars)")

// ParseKey decodes a 64-character hex string into a 32-byte AES-256 key.
func ParseKey(hexKey string) ([]byte, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeyLength, err)
	}
	if len(b) != 32 {
		return nil, ErrKeyLength
	}
	return b, nil
}

// Encrypt encrypts plaintext with AES-256-GCM and returns a hex-encoded
// nonce+ciphertext string safe for storage.
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Returns the plaintext string.
func Decrypt(key []byte, cipherHex string) (string, error) {
	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("aescrypt: ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
