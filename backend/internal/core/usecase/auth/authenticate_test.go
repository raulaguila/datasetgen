package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/output"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type stubTokens struct {
	output.TokenIssuer
	claims *output.AccessClaims
}

func (s stubTokens) ParseAccess(token string) (*output.AccessClaims, error) {
	if token != "good" {
		return nil, errors.New("bad token")
	}
	return s.claims, nil
}

type stubSessions struct {
	output.SessionRepository
	sess *domain.Session
	err  error
}

func (s stubSessions) FindLive(context.Context, string) (*domain.Session, error) {
	return s.sess, s.err
}

type stubUOW struct {
	output.UnitOfWork
	sessions stubSessions
}

func (s stubUOW) Sessions() output.SessionRepository { return s.sessions }

func TestAuthenticate(t *testing.T) {
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	claims := &output.AccessClaims{UserID: "u1", SessionID: "s1"}
	live := &domain.Session{UserID: "u1", ExpiresAt: future}

	cases := []struct {
		name   string
		token  string
		claims *output.AccessClaims
		sess   *domain.Session
		err    error
		want   apperror.Code // empty means accepted
	}{
		{"valid", "good", claims, live, nil, ""},
		{"bad token", "bad", claims, live, nil, apperror.CodeUnauthorized},
		{"no subject", "good", &output.AccessClaims{SessionID: "s1"}, live, nil, apperror.CodeUnauthorized},
		{"no session claim", "good", &output.AccessClaims{UserID: "u1"}, live, nil, apperror.CodeUnauthorized},
		{"expired token", "good", &output.AccessClaims{UserID: "u1", SessionID: "s1", ExpiresAt: past}, live, nil, apperror.CodeTokenExpired},
		{"unknown session", "good", claims, nil, apperror.New(apperror.CodeNotFound, "x"), apperror.CodeUnauthorized},
		{"database down", "good", claims, nil, apperror.New(apperror.CodeDatabaseError, "x"), apperror.CodeDatabaseError},
		{"other user's session", "good", claims, &domain.Session{UserID: "u2", ExpiresAt: future}, nil, apperror.CodeUnauthorized},
		{"revoked session", "good", claims, &domain.Session{UserID: "u1", ExpiresAt: future, RevokedAt: &past}, nil, apperror.CodeUnauthorized},
		{"expired session", "good", claims, &domain.Session{UserID: "u1", ExpiresAt: past}, nil, apperror.CodeTokenExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(nil, stubUOW{sessions: stubSessions{sess: tc.sess, err: tc.err}}, stubTokens{claims: tc.claims}, output.RealClock{}, Config{}, nil)
			got, err := svc.Authenticate(context.Background(), tc.token)
			if tc.want != "" {
				if !apperror.IsCode(err, tc.want) {
					t.Fatalf("want %s, got %v", tc.want, err)
				}
				return
			}
			if err != nil || got.UserID != "u1" || got.SessionID != "s1" {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}
