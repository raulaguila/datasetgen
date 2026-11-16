package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/aescrypt"
	"github.com/raulaguila/dataset/pkg/apperror"
)

const ssoFlowTTL = 10 * time.Minute

// ssoFlow is what the browser keeps, sealed, between start and callback.
type ssoFlow struct {
	State    string    `json:"s"`
	Nonce    string    `json:"n"`
	Verifier string    `json:"v"`
	Expires  time.Time `json:"e"`
}

func ssoOff() error { return apperror.New(apperror.CodeNotFound, "single sign-on is not set up") }

// SSOName is the provider label; empty when single sign-on is off.
func (s *Service) SSOName() string {
	if s.SSO == nil {
		return ""
	}
	return s.SSO.Name()
}

// SSOStart returns the provider's sign-in address and the sealed flow the
// caller keeps (an httpOnly cookie) until the callback.
func (s *Service) SSOStart(ctx context.Context) (url, flow string, err error) {
	if s.SSO == nil {
		return "", "", ssoOff()
	}
	f := ssoFlow{State: random(), Nonce: random(), Verifier: random(), Expires: s.Clock.Now().Add(ssoFlowTTL)}
	if url, err = s.SSO.AuthURL(ctx, f.State, f.Nonce, f.Verifier); err != nil {
		return "", "", apperror.New(apperror.CodeInternal, "the sign-in provider did not answer").WithCause(err)
	}
	raw, _ := json.Marshal(f)
	if flow, err = aescrypt.Encrypt(s.SealKey, string(raw)); err != nil {
		return "", "", apperror.New(apperror.CodeInternal, "flow sealing failed").WithCause(err)
	}
	return url, flow, nil
}

// SSOLogin opens a session for the existing, active account whose email the
// provider verified. It never creates accounts, and two-step login is left to
// the provider.
func (s *Service) SSOLogin(ctx context.Context, cmd input.SSOCommand) (*domain.TokenPair, error) {
	if s.SSO == nil {
		return nil, ssoOff()
	}
	now := s.Clock.Now()
	var f ssoFlow
	raw, err := aescrypt.Decrypt(s.SealKey, cmd.Flow)
	if err != nil || json.Unmarshal([]byte(raw), &f) != nil || now.After(f.Expires) ||
		subtle.ConstantTimeCompare([]byte(f.State), []byte(cmd.State)) != 1 {
		return nil, apperror.New(apperror.CodeInvalidInput, "the sign-in expired or was started elsewhere; try again")
	}
	email, err := s.SSO.Email(ctx, cmd.Code, f.Verifier, f.Nonce)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidCredentials, "the sign-in provider refused the login").WithCause(err)
	}
	user, err := s.UOW.Users().FindByEmail(ctx, policy.NormalizeEmail(email))
	if apperror.IsNotFound(err) {
		return nil, apperror.New(apperror.CodeForbidden, "no account uses this email; ask an administrator for an invitation")
	}
	if err != nil {
		return nil, err
	}
	if reason, err := s.loginState(ctx, user, now); err != nil || reason != "" {
		if err == nil {
			err = invalidCredentials()
		}
		return nil, err
	}
	cred, err := s.UOW.Credentials().FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if cred.PasswordHash == nil || (cred.LockedUntil != nil && cred.LockedUntil.After(now)) {
		return nil, invalidCredentials()
	}
	pair, _, err := s.issueSession(ctx, user.ID, *cred.PasswordHash, cmd.IPAddress, cmd.UserAgent, now)
	return pair, err
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}
