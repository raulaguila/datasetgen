package tokens

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

type accessClaims struct {
	Sid string `json:"sid"`
	jwt.RegisteredClaims
}

type refreshClaims struct {
	Sid string `json:"sid"`
	jwt.RegisteredClaims
}

// Issuer signs and parses EdDSA (Ed25519) JWTs: small signatures and a cheap
// verification, which runs on every authenticated request.
type Issuer struct {
	AccessKey  ed25519.PrivateKey
	RefreshKey ed25519.PrivateKey
	// Optional keys from before a rotation: still verified, never used to sign.
	// Drop them once the refresh lifetime has passed since the switch.
	AccessPrevious  ed25519.PrivateKey
	RefreshPrevious ed25519.PrivateKey
	AccessExpire    time.Duration
	RefreshExpire   time.Duration
}

var _ output.TokenIssuer = (*Issuer)(nil)

func (i *Issuer) AccessTTL() time.Duration  { return i.AccessExpire }
func (i *Issuer) RefreshTTL() time.Duration { return i.RefreshExpire }

func (i *Issuer) IssueAccess(userID, sessionID string, now time.Time) (string, error) {
	claims := accessClaims{
		Sid: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.AccessExpire)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return t.SignedString(i.AccessKey)
}

func (i *Issuer) IssueRefresh(userID, sessionID string, now time.Time) (token string, jti string, err error) {
	jti = uuid.NewString()
	claims := refreshClaims{
		Sid: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.RefreshExpire)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token, err = t.SignedString(i.RefreshKey)
	return token, jti, err
}

// verify fills claims from a token signed by the current key or, after a
// rotation, by the previous one.
func verify(token string, claims jwt.Claims, keys ...ed25519.PrivateKey) error {
	err := errors.New("no signing key")
	for _, key := range keys {
		if key == nil {
			continue
		}
		var parsed *jwt.Token
		parsed, err = jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodEdDSA {
				return nil, fmt.Errorf("unexpected method")
			}
			return key.Public(), nil
		})
		if err == nil && !parsed.Valid {
			err = errors.New("invalid token")
		}
		if !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return err // accepted, or refused for a reason another key cannot change
		}
	}
	return err
}

func (i *Issuer) ParseAccess(token string) (*output.AccessClaims, error) {
	claims := &accessClaims{}
	if err := verify(token, claims, i.AccessKey, i.AccessPrevious); err != nil {
		return nil, err
	}
	out := &output.AccessClaims{UserID: claims.Subject, SessionID: claims.Sid}
	if claims.ExpiresAt != nil {
		out.ExpiresAt = claims.ExpiresAt.Time
	}
	return out, nil
}

func (i *Issuer) ParseRefresh(token string) (*output.RefreshClaims, error) {
	claims := &refreshClaims{}
	if err := verify(token, claims, i.RefreshKey, i.RefreshPrevious); err != nil {
		return nil, err
	}
	out := &output.RefreshClaims{UserID: claims.Subject, SessionID: claims.Sid, JTI: claims.ID}
	if claims.ExpiresAt != nil {
		out.ExpiresAt = claims.ExpiresAt.Time
	}
	return out, nil
}
