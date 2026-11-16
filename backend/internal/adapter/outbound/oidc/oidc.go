// Package oidc signs users in with an OpenID Connect provider.
package oidc

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Provider discovers the issuer on first use, so the backend starts while it is down.
type Provider struct {
	Issuer, ClientID, ClientSecret, RedirectURL, Label string

	mu       sync.Mutex
	conf     *oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

var client = &http.Client{Timeout: 10 * time.Second}

func (p *Provider) Name() string { return p.Label }

func (p *Provider) setup(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conf != nil {
		return nil
	}
	// The provider keeps this context to fetch signing keys later; it must outlive the request.
	prov, err := gooidc.NewProvider(gooidc.ClientContext(context.WithoutCancel(ctx), client), p.Issuer)
	if err != nil {
		return err
	}
	p.conf = &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  p.RedirectURL,
		Endpoint:     prov.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
	}
	p.verifier = prov.Verifier(&gooidc.Config{ClientID: p.ClientID})
	return nil
}

func (p *Provider) AuthURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	if err := p.setup(ctx); err != nil {
		return "", err
	}
	return p.conf.AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

func (p *Provider) Email(ctx context.Context, code, verifier, nonce string) (string, error) {
	if err := p.setup(ctx); err != nil {
		return "", err
	}
	ctx = gooidc.ClientContext(ctx, client)
	tok, err := p.conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", err
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return "", errors.New("no id_token in the answer")
	}
	id, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return "", err
	}
	if id.Nonce != nonce {
		return "", errors.New("id_token nonce does not match")
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := id.Claims(&claims); err != nil {
		return "", err
	}
	// An unverified email could be anyone's; it never links to an account.
	if claims.Email == "" || !claims.Verified {
		return "", errors.New("the provider gave no verified email")
	}
	return claims.Email, nil
}
