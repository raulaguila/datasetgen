package integration_test

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/core/usecase/authorize"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// A new account starts with a collection it owns and may create more; someone
// invited into a collection starts there instead of getting a second one.
func TestNewAccountStartsWithACollection(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	ctx := context.Background()
	addr := e.activeUser(t)
	as := reqctx.WithActor(ctx, e.userID(t, addr))

	list, _, err := collections.List(as, domain.CollectionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Minha coleção" || list[0].Role != domain.RoleOwner {
		t.Fatalf("starter: %+v", list)
	}

	p, err := e.svc.Authenticate(ctx, e.loginAt(t, addr, "203.0.113.20").AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize.New(rbac.NewRepository(testDB), e.clock).Authorize(ctx, *p, domain.ResourceCollections, domain.ActionCreate); err != nil {
		t.Fatalf("a new account cannot create collections: %v", err)
	}

	// Invited into the starter collection, then activated: no starter of their own.
	guest := email()
	res, err := collections.AddMember(as, input.AddMemberCommand{CollectionID: list[0].ID, Email: guest, Name: "Bia", Role: domain.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Activate(ctx, input.ActivateCommand{Token: res.ActivationToken, Password: strongPassword}); err != nil {
		t.Fatal(err)
	}
	theirs, _, err := collections.List(reqctx.WithActor(ctx, e.userID(t, guest)), domain.CollectionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 1 || theirs[0].ID != list[0].ID {
		t.Fatalf("invited account got %+v", theirs)
	}
}
