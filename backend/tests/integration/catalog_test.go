package integration_test

import (
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
)

// A grant may name only a resource the routes check, or the sentinel.
func TestGrantsOnlyNameKnownResources(t *testing.T) {
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
		t.Fatal("a grant on a resource no route checks must fail")
	}
	for _, r := range []string{domain.ResourceAll, domain.ResourceAudit, domain.ResourceCollections, domain.ResourceRoles, domain.ResourceUsers} {
		if err := grant(r); err != nil {
			t.Fatalf("%s: %v", r, err)
		}
	}
}
