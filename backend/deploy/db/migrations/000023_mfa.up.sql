-- Two-step login: a TOTP secret per user (sealed with the SMTP token key),
-- unconfirmed until enabled_at is set, and one-use recovery codes (SHA-256).
CREATE TABLE usr.usr_mfa (
    user_id    UUID        PRIMARY KEY REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    secret     TEXT        NOT NULL,
    enabled_at TIMESTAMPTZ NULL,
    -- The last 30-second step a code was taken for: a code works once.
    last_step  BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE usr.usr_recovery_code (
    user_id   UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    code_hash BYTEA       NOT NULL,
    used_at   TIMESTAMPTZ NULL,
    PRIMARY KEY (user_id, code_hash)
);

-- A wrong second-step code counts against the account like a wrong password.
ALTER TABLE usr.usr_login_attempt DROP CONSTRAINT chk_usr_login_attempt_failure_kind;
ALTER TABLE usr.usr_login_attempt ADD CONSTRAINT chk_usr_login_attempt_failure_kind CHECK (
    failure_kind IN (
        'bad_password', 'bad_code', 'user_not_found', 'account_locked', 'account_disabled',
        'account_pending', 'rate_limited_ip', 'rate_limited_email'
    )
);
