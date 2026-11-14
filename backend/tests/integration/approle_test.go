package integration_test

import (
	"testing"

	"gorm.io/gorm"
)

func TestAppRoleReadsAndWritesDataButNotSchema(t *testing.T) {
	err := testDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL ROLE dataset_app").Error; err != nil {
			return err
		}
		for _, sql := range []string{
			"SELECT count(*) FROM usr.vw_user_effective_permissions",
			"SELECT count(*) FROM sys.outbox",
			"DELETE FROM usr.usr_session WHERE false",
			"UPDATE app.record SET data = data WHERE false",
			"INSERT INTO usr.usr_audit_log (action) VALUES ('test.app_role')",
			"INSERT INTO sys.rate_limit (key, window_start) VALUES ('test', now())",
		} {
			if err := tx.Exec(sql).Error; err != nil {
				t.Errorf("%s: %v", sql, err)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = testDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL ROLE dataset_app").Error; err != nil {
			t.Fatal(err)
		}
		return tx.Exec("DROP TABLE app.record").Error
	})
	if err == nil {
		t.Fatal("app role dropped a table")
	}
}

func TestMailerRoleIsLimitedToOutboxDelivery(t *testing.T) {
	for _, sql := range []string{
		"SELECT count(*) FROM sys.outbox",
		"UPDATE sys.outbox SET status='processing',claim_id=gen_random_uuid() WHERE false",
	} {
		if err := testDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL ROLE dataset_mailer").Error; err != nil {
				return err
			}
			return tx.Exec(sql).Error
		}); err != nil {
			t.Fatalf("allowed operation: %v", err)
		}
	}
	for _, sql := range []string{
		"SELECT password_hash FROM usr.usr_credential",
		"UPDATE usr.usr_user SET status='disabled' WHERE false",
		"SELECT * FROM app.record",
		"INSERT INTO sys.outbox(purpose,payload) VALUES ('activation','{}')",
		"DELETE FROM sys.outbox WHERE false",
		"UPDATE sys.outbox SET purpose='password_reset' WHERE false",
	} {
		err := testDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL ROLE dataset_mailer").Error; err != nil {
				return err
			}
			return tx.Exec(sql).Error
		})
		if err == nil {
			t.Fatalf("mailer allowed forbidden operation: %s", sql)
		}
	}
}
