package integration_test

import (
	"testing"

	"github.com/google/uuid"
)

// newRole inserts an enabled role straight in the database (roles have no API)
// and removes it with its members after the test.
func newRole(t *testing.T, isDefault bool) (id, name string) {
	t.Helper()
	name = "t" + uuid.NewString()[:8]
	if err := testDB.Raw(`INSERT INTO usr.usr_role (name, is_default) VALUES (?, ?) RETURNING id`, name, isDefault).Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = testDB.Exec(`DELETE FROM usr.usr_user_role WHERE role_id = ?`, id).Error
		_ = testDB.Exec(`DELETE FROM usr.usr_role WHERE id = ?`, id).Error
	})
	return id, name
}
