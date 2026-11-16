package integration

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/aescrypt"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// OAuth lets an assistant that cannot take a pasted key (the ChatGPT app) ask
// the member for one: the member picks the collections on a consent page and
// the assistant gets an API key as its access token. Nothing is stored until
// the key: the client id and the code are sealed with SealKey.
const (
	oauthCodeTTL    = 5 * time.Minute
	maxRedirectURIs = 5
	maxOAuthName    = 80
	oauthClientKind = "client"
	oauthCodeKind   = "code"
)

type oauthClient struct {
	Kind         string   `json:"k"`
	Name         string   `json:"n"`
	RedirectURIs []string `json:"r"`
}

type oauthCode struct {
	Kind        string          `json:"k"`
	UserID      string          `json:"u"`
	ClientID    string          `json:"c"`
	RedirectURI string          `json:"r"`
	Challenge   string          `json:"p"`
	Name        string          `json:"n"`
	Scopes      json.RawMessage `json:"s"`
	Days        int             `json:"d"`
	Expires     int64           `json:"e"`
}

func badOAuth(msg string) error { return apperror.New(apperror.CodeInvalidInput, msg) }

// redirectOK takes https anywhere and plain http only back to this machine.
func redirectOK(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" {
		return false
	}
	host := u.Hostname()
	return u.Scheme == "https" || (u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1"))
}

func (s *Service) seal(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return aescrypt.Encrypt(s.SealKey, string(raw))
}

func (s *Service) open(sealed string, v any) bool {
	raw, err := aescrypt.Decrypt(s.SealKey, sealed)
	return err == nil && json.Unmarshal([]byte(raw), v) == nil
}

func (s *Service) RegisterOAuthClient(_ context.Context, name string, redirectURIs []string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Assistant"
	}
	if utf8.RuneCountInString(name) > maxOAuthName {
		return "", badOAuth("client_name must be up to 80 characters")
	}
	if len(redirectURIs) < 1 || len(redirectURIs) > maxRedirectURIs {
		return "", badOAuth("give 1 to 5 redirect_uris")
	}
	for _, u := range redirectURIs {
		if len(u) > maxURL || !redirectOK(u) {
			return "", badOAuth("a redirect_uri must be https, or http on localhost")
		}
	}
	return s.seal(oauthClient{Kind: oauthClientKind, Name: name, RedirectURIs: redirectURIs})
}

func (s *Service) client(clientID, redirectURI string) (*oauthClient, error) {
	var c oauthClient
	if !s.open(clientID, &c) || c.Kind != oauthClientKind {
		return nil, badOAuth("unknown client_id")
	}
	if !slices.Contains(c.RedirectURIs, redirectURI) {
		return nil, badOAuth("redirect_uri was not registered for this client")
	}
	return &c, nil
}

func (s *Service) OAuthClient(_ context.Context, clientID, redirectURI string) (string, error) {
	c, err := s.client(clientID, redirectURI)
	if err != nil {
		return "", err
	}
	return c.Name, nil
}

func (s *Service) ApproveOAuth(ctx context.Context, g input.OAuthGrant) (string, error) {
	if _, err := s.client(g.ClientID, g.RedirectURI); err != nil {
		return "", err
	}
	if g.ChallengeMethod != "S256" || len(g.Challenge) < 43 || len(g.Challenge) > 128 {
		return "", badOAuth("a code_challenge with method S256 is required")
	}
	name, scopes, err := keyInput(ctx, g.Name, g.Scopes)
	if err != nil {
		return "", err
	}
	if _, err := expiryOf(g.Days, "expires_in_days"); err != nil {
		return "", err
	}
	if err := allowed(ctx, s.Store, scopes); err != nil {
		return "", err
	}
	raw, err := json.Marshal(scopes)
	if err != nil {
		return "", err
	}
	return s.seal(oauthCode{
		Kind: oauthCodeKind, UserID: reqctx.Actor(ctx), ClientID: g.ClientID, RedirectURI: g.RedirectURI, Challenge: g.Challenge,
		Name: name, Scopes: raw, Days: g.Days, Expires: time.Now().Add(oauthCodeTTL).Unix(),
	})
}

// ExchangeOAuthCode makes the key the member approved. The code proves the
// approval; the verifier proves the caller started it (PKCE).
// shortcut: a code is not marked used, so it works again until it expires (5 min);
// PKCE keeps it to the client that holds the verifier. Store used codes if that matters.
func (s *Service) ExchangeOAuthCode(ctx context.Context, code, clientID, redirectURI, verifier string) (*input.NewKey, error) {
	var c oauthCode
	if !s.open(code, &c) || c.Kind != oauthCodeKind || time.Now().Unix() > c.Expires {
		return nil, badOAuth("the code is invalid or expired")
	}
	if c.ClientID != clientID || c.RedirectURI != redirectURI {
		return nil, badOAuth("the code was issued to another client or redirect_uri")
	}
	sum := sha256.Sum256([]byte(verifier))
	if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(c.Challenge)) != 1 {
		return nil, badOAuth("code_verifier does not match the code_challenge")
	}
	var scopes []domain.KeyScope
	if err := json.Unmarshal(c.Scopes, &scopes); err != nil {
		return nil, err
	}
	return s.CreateMyKey(reqctx.WithActor(ctx, c.UserID), c.Name, scopes, c.Days)
}
