UPDATE usr.usr_login_attempt SET failure_kind = 'bad_password' WHERE failure_kind = 'bad_code';
ALTER TABLE usr.usr_login_attempt DROP CONSTRAINT chk_usr_login_attempt_failure_kind;
ALTER TABLE usr.usr_login_attempt ADD CONSTRAINT chk_usr_login_attempt_failure_kind CHECK (
    failure_kind IN (
        'bad_password', 'user_not_found', 'account_locked', 'account_disabled',
        'account_pending', 'rate_limited_ip', 'rate_limited_email'
    )
);
DROP TABLE usr.usr_recovery_code;
DROP TABLE usr.usr_mfa;
