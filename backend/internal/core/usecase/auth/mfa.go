package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"strings"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/policy"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/aescrypt"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/password"
	"github.com/raulaguila/dataset/pkg/reqctx"
	"github.com/raulaguila/dataset/pkg/tokenhash"
	"github.com/raulaguila/dataset/pkg/totp"
)

const (
	mfaIssuer     = "Dataset"
	recoveryCodes = 10
)

func mfaRequired() error {
	return apperror.New(apperror.CodeMFARequired, "enter the code from your authenticator app")
}

func (s *Service) MFAStatus(ctx context.Context) (*domain.MFAStatus, error) {
	userID := reqctx.Actor(ctx)
	m, err := s.UOW.Credentials().MFA(ctx, userID)
	if apperror.IsNotFound(err) || (err == nil && m.EnabledAt == nil) {
		return &domain.MFAStatus{}, nil
	}
	if err != nil {
		return nil, err
	}
	left, err := s.UOW.Credentials().RecoveryCodesLeft(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.MFAStatus{Enabled: true, RecoveryLeft: left}, nil
}

// SetupMFA makes a new secret for the caller to add to an authenticator app;
// it does nothing at login until EnableMFA confirms a code from it.
func (s *Service) SetupMFA(ctx context.Context) (secret, uri string, err error) {
	userID := reqctx.Actor(ctx)
	user, err := s.UOW.Users().FindByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if secret, err = totp.NewSecret(); err != nil {
		return "", "", err
	}
	sealed, err := aescrypt.Encrypt(s.SealKey, secret)
	if err != nil {
		return "", "", apperror.New(apperror.CodeInternal, "secret sealing failed").WithCause(err)
	}
	stored, err := s.UOW.Credentials().SetupMFA(ctx, userID, sealed)
	if err != nil {
		return "", "", err
	}
	if !stored {
		return "", "", apperror.New(apperror.CodeConflict, "two-step login is on already; turn it off first")
	}
	return secret, totp.URI(mfaIssuer, user.Email, secret), nil
}

// EnableMFA turns the second step on with a code of the new secret, and returns
// the recovery codes, shown this once.
func (s *Service) EnableMFA(ctx context.Context, code string) ([]string, error) {
	userID := reqctx.Actor(ctx)
	m, err := s.UOW.Credentials().MFA(ctx, userID)
	if apperror.IsNotFound(err) || (err == nil && m.EnabledAt != nil) {
		return nil, apperror.New(apperror.CodeConflict, "start the setup first")
	}
	if err != nil {
		return nil, err
	}
	secret, err := aescrypt.Decrypt(s.SealKey, m.Secret)
	if err != nil {
		return nil, apperror.New(apperror.CodeInternal, "secret opening failed").WithCause(err)
	}
	step, ok := totp.Match(secret, code, s.Clock.Now())
	if !ok {
		return nil, apperror.New(apperror.CodeInvalidInput, "the code does not match; check the phone's clock and try the next one").WithField("code")
	}
	plain := make([]string, recoveryCodes)
	hashes := make([][]byte, recoveryCodes)
	for i := range plain {
		if plain[i], err = recoveryCode(); err != nil {
			return nil, err
		}
		hashes[i] = tokenhash.SHA256Bytes(plain[i])
	}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		enabled, err := uow.Credentials().EnableMFA(ctx, userID, step, hashes, s.Clock.Now())
		if err != nil {
			return err
		}
		if !enabled {
			return apperror.New(apperror.CodeConflict, "start the setup first")
		}
		return policy.Audit(ctx, uow, "auth.mfa_enable", userID, nil)
	})
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// DisableMFA turns the second step off; the password proves it is the owner.
func (s *Service) DisableMFA(ctx context.Context, pass string) error {
	userID := reqctx.Actor(ctx)
	cred, err := s.UOW.Credentials().FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if cred.PasswordHash == nil {
		return invalidCredentials()
	}
	if ok, err := password.Verify(ctx, *cred.PasswordHash, pass); err != nil || !ok {
		return apperror.New(apperror.CodeInvalidCredentials, "the password is wrong").WithField("password")
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context, uow output.UnitOfWork) error {
		if err := uow.Credentials().DeleteMFA(ctx, userID); err != nil {
			return err
		}
		return policy.Audit(ctx, uow, "auth.mfa_disable", userID, nil)
	})
}

// secondStep checks a login's code: a TOTP code (each step once) or an unused
// recovery code. ok false is a wrong code.
func (s *Service) secondStep(ctx context.Context, m *domain.MFA, code string, now time.Time) (bool, error) {
	code = strings.TrimSpace(code)
	if len(code) > 64 {
		return false, nil
	}
	if strings.Contains(code, "-") { // recovery codes look like abcde-fghij
		return s.UOW.Credentials().UseRecoveryCode(ctx, m.UserID, tokenhash.SHA256Bytes(strings.ToLower(code)), now)
	}
	secret, err := aescrypt.Decrypt(s.SealKey, m.Secret)
	if err != nil {
		return false, apperror.New(apperror.CodeInternal, "secret opening failed").WithCause(err)
	}
	step, ok := totp.Match(secret, code, now)
	if !ok || step <= m.LastStep {
		return false, nil
	}
	return s.UOW.Credentials().UseMFAStep(ctx, m.UserID, step)
}

var recoveryAlphabet = base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding)

// recoveryCode is 10 random characters (50 bits), dashed in the middle.
func recoveryCode() (string, error) {
	b := make([]byte, 7)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := recoveryAlphabet.EncodeToString(b)[:10]
	return s[:5] + "-" + s[5:], nil
}
