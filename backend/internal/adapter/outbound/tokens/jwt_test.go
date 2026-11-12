package tokens

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func issuer(t *testing.T) *Issuer {
	t.Helper()
	_, access, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, refresh, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Issuer{AccessKey: access, RefreshKey: refresh, AccessExpire: time.Minute, RefreshExpire: time.Hour}
}

func TestTokensRoundTripAndDoNotCross(t *testing.T) {
	i := issuer(t)
	now := time.Now()
	access, err := i.IssueAccess("u", "s", now)
	if err != nil {
		t.Fatal(err)
	}
	refresh, jti, err := i.IssueRefresh("u", "s", now)
	if err != nil {
		t.Fatal(err)
	}

	if c, err := i.ParseAccess(access); err != nil || c.UserID != "u" || c.SessionID != "s" {
		t.Fatalf("access: %+v %v", c, err)
	}
	if c, err := i.ParseRefresh(refresh); err != nil || c.JTI != jti {
		t.Fatalf("refresh: %+v %v", c, err)
	}
	if _, err := i.ParseAccess(refresh); err == nil {
		t.Fatal("refresh token accepted as access")
	}
	if _, err := i.ParseRefresh(access); err == nil {
		t.Fatal("access token accepted as refresh")
	}
}

func TestExpiredAndForeignTokensRefused(t *testing.T) {
	i := issuer(t)
	old, err := i.IssueAccess("u", "s", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.ParseAccess(old); err == nil {
		t.Fatal("expired token accepted")
	}
	foreign, err := issuer(t).IssueAccess("u", "s", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.ParseAccess(foreign); err == nil {
		t.Fatal("foreign signature accepted")
	}
}

func TestRotationKeepsTokensOfThePreviousKeyOnly(t *testing.T) {
	old, next := issuer(t), issuer(t)
	now := time.Now()
	access, _ := old.IssueAccess("u", "s", now)
	refresh, _, _ := old.IssueRefresh("u", "s", now)
	if _, err := next.ParseAccess(access); err == nil {
		t.Fatal("a token of an unknown key was accepted")
	}

	next.AccessPrevious, next.RefreshPrevious = old.AccessKey, old.RefreshKey
	if c, err := next.ParseAccess(access); err != nil || c.SessionID != "s" {
		t.Fatalf("access of the previous key: %+v %v", c, err)
	}
	if _, err := next.ParseRefresh(refresh); err != nil {
		t.Fatalf("refresh of the previous key: %v", err)
	}
	if _, err := next.ParseAccess(refresh); err == nil {
		t.Fatal("previous refresh token accepted as access")
	}
	expired, _ := old.IssueAccess("u", "s", now.Add(-time.Hour))
	if _, err := next.ParseAccess(expired); err == nil {
		t.Fatal("expired token of the previous key accepted")
	}
}
