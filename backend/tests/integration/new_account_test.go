package integration_test

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/usecase/authorize"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// A new account starts with no collection and may create its own.
func TestNewAccountStartsEmptyAndMayCreate(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	ctx := context.Background()
	addr := e.activeUser(t)

	list, _, err := collections.List(reqctx.WithActor(ctx, e.userID(t, addr)), domain.CollectionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("a new account got collections: %+v", list)
	}

	p, err := e.svc.Authenticate(ctx, e.loginAt(t, addr, "203.0.113.20").AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize.New(rbac.NewRepository(testDB), e.clock).Authorize(ctx, *p, domain.ResourceCollections, domain.ActionCreate); err != nil {
		t.Fatalf("a new account cannot create collections: %v", err)
	}
}
