DELETE FROM app.automation WHERE action IN ('email', 'webhook', 'create_record');
ALTER TABLE app.automation DROP CONSTRAINT automation_action_check;
ALTER TABLE app.automation ADD CONSTRAINT automation_action_check CHECK (action IN ('set_field', 'notify'));

DELETE FROM sys.outbox WHERE purpose = 'automation';
ALTER TABLE sys.outbox DROP CONSTRAINT chk_sys_outbox_purpose;
ALTER TABLE sys.outbox ADD CONSTRAINT chk_sys_outbox_purpose CHECK (purpose IN ('activation', 'password_reset'));
