package integration_test

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/internal/adapter/outbound/persistence/postgres/rbac"
	"github.com/raulaguila/dataset/internal/core/domain"
)

func TestCatalogHoldsEveryResourceTheCodeChecks(t *testing.T) {
	resources, err := rbac.NewRepository(testDB).ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, r := range resources {
		found[r.Code] = true
		if r.Actions == 0 {
			t.Errorf("%s takes no action", r.Code)
		}
	}
	for _, code := range []string{domain.ResourceProducts, domain.ResourceRoles, domain.ResourceUsers} {
		if !found[code] {
			t.Errorf("%s is missing from usr_resource: %v", code, resources)
		}
	}
}

func TestGrantsOnlyNameCatalogResources(t *testing.T) {
	var roleID string
	if err := testDB.Raw(`
		INSERT INTO usr.usr_role (name) VALUES ('catalog_test') RETURNING id
	`).Scan(&roleID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = testDB.Exec("DELETE FROM usr.usr_role WHERE id = ?", roleID).Error
	})
	grant := func(resource string) error {
		return testDB.Exec(`
			INSERT INTO usr.usr_role_permission (role_id, resource, actions) VALUES (?, ?, 2)
		`, roleID, resource).Error
	}

	if err := grant("unknown"); err == nil {
		t.Fatal("a grant on a resource outside the catalog must fail")
	}
	if err := grant(domain.ResourceAll); err != nil {
		t.Fatalf("the sentinel stays allowed: %v", err)
	}
	if err := grant(domain.ResourceProducts); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec("DELETE FROM usr.usr_resource WHERE code = ?", domain.ResourceProducts).Error; err == nil {
		t.Fatal("a resource with grants must stay in the catalog")
	}
}
