package authorize

import (
	"context"
	"testing"
	"time"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/pkg/apperror"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type clock struct{}

func (clock) Now() time.Time { return now }

type stubReader struct {
	sess *domain.Session
	have domain.Action
}

func (s stubReader) LoadEffective(context.Context, string) (*domain.EffectivePermissions, error) {
	return nil, nil
}

func (s stubReader) Access(context.Context, string, string) (*domain.Session, domain.Action, error) {
	if s.sess == nil {
		return nil, 0, apperror.New(apperror.CodeNotFound, "session not found")
	}
	return s.sess, s.have, nil
}

func (s stubReader) ListResources(context.Context) ([]domain.Resource, error) {
	return nil, nil
}

func TestAuthorizeChecksTheSessionThenEveryNeededBit(t *testing.T) {
	ctx := context.Background()
	p := domain.Principal{UserID: "u1", SessionID: "s1"}
	live := &domain.Session{UserID: "u1", ExpiresAt: now.Add(time.Hour)}
	revoked := &domain.Session{UserID: "u1", ExpiresAt: now.Add(time.Hour), RevokedAt: &now}
	for _, tc := range []struct {
		name string
		sess *domain.Session
		need domain.Action
		want apperror.Code
	}{
		{"allowed", live, domain.ActionRead, ""},
		{"missing bit", live, domain.ActionCreate, apperror.CodeForbidden},
		{"one of two bits", live, domain.ActionRead | domain.ActionUpdate, apperror.CodeForbidden},
		{"no session", nil, domain.ActionRead, apperror.CodeUnauthorized},
		{"revoked", revoked, domain.ActionRead, apperror.CodeUnauthorized},
		{"expired", &domain.Session{UserID: "u1", ExpiresAt: now}, domain.ActionRead, apperror.CodeTokenExpired},
		{"other user", &domain.Session{UserID: "u2", ExpiresAt: now.Add(time.Hour)}, domain.ActionRead, apperror.CodeUnauthorized},
	} {
		err := New(stubReader{sess: tc.sess, have: domain.ActionRead}, clock{}).Authorize(ctx, p, "products", tc.need)
		if tc.want == "" && err != nil || tc.want != "" && !apperror.IsCode(err, tc.want) {
			t.Errorf("%s: want %q, got %v", tc.name, tc.want, err)
		}
	}
}
