-- Automations also send an email, call one of the collection's webhooks, or
-- create a record in a collection; the email goes through the mail outbox.
ALTER TABLE app.automation DROP CONSTRAINT automation_action_check;
ALTER TABLE app.automation ADD CONSTRAINT automation_action_check
    CHECK (action IN ('set_field', 'notify', 'email', 'webhook', 'create_record'));

ALTER TABLE sys.outbox DROP CONSTRAINT chk_sys_outbox_purpose;
ALTER TABLE sys.outbox ADD CONSTRAINT chk_sys_outbox_purpose
    CHECK (purpose IN ('activation', 'password_reset', 'automation'));
