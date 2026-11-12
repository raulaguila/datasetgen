// Package emailpayload owns the encrypted JSON used by the email outbox.
package emailpayload

import (
	"encoding/json"
	"errors"

	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/aescrypt"
)

type wireEmail struct {
	Email string `json:"email"`
	Token string `json:"token"`
	Lang  string `json:"lang,omitempty"`
}

type Codec struct{ key []byte }

func New(key []byte) *Codec { return &Codec{key: append([]byte(nil), key...)} }

var _ output.EmailTokenCodec = (*Codec)(nil)

func (c *Codec) Encode(p output.EmailToken) ([]byte, error) {
	sealed, err := aescrypt.Encrypt(c.key, p.Token)
	if err != nil {
		return nil, err
	}
	p.Token = sealed
	return json.Marshal(wireEmail(p))
}

func (c *Codec) Decode(raw []byte) (output.EmailToken, error) {
	var p wireEmail
	if err := json.Unmarshal(raw, &p); err != nil {
		return output.EmailToken(p), errors.New("invalid_payload")
	}
	if p.Email == "" || p.Token == "" {
		return output.EmailToken(p), errors.New("invalid_payload")
	}
	token, err := aescrypt.Decrypt(c.key, p.Token)
	if err != nil {
		return output.EmailToken{}, errors.New("invalid_payload")
	}
	p.Token = token
	return output.EmailToken(p), nil
}
