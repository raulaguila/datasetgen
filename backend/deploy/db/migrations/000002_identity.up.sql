-- Identity: users, sessions, email tokens, login attempts, RBAC, preferences and
-- notifications, with the default roles.

CREATE TABLE usr.usr_user (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    email      CITEXT      NOT NULL UNIQUE,
    status     TEXT        NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- When the user last set a profile picture; NULL means none. The image itself
    -- lives in the file store under avatars/<user id>.
    avatar_at  TIMESTAMPTZ NULL,
    CONSTRAINT chk_usr_user_name_len CHECK (char_length(name) BETWEEN 2 AND 80),
    CONSTRAINT chk_usr_user_status CHECK (status IN ('pending', 'active', 'locked', 'disabled'))
);

-- The user list sorts by these, and its search is a substring match on name and email.
CREATE INDEX idx_usr_user_name ON usr.usr_user (name, id);
CREATE INDEX idx_usr_user_created ON usr.usr_user (created_at DESC, id);
CREATE INDEX idx_usr_user_name_trgm ON usr.usr_user USING gin (name gin_trgm_ops);
CREATE INDEX idx_usr_user_email_trgm ON usr.usr_user USING gin ((email::text) gin_trgm_ops);

-- password_hash is a PHC string, so it names its algorithm. NULL until activation.
CREATE TABLE usr.usr_credential (
    user_id               UUID        PRIMARY KEY REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    password_hash         TEXT        NULL,
    failed_login_attempts INT         NOT NULL DEFAULT 0,
    locked_until          TIMESTAMPTZ NULL,
    last_login_at         TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The unit of logout (sid in the JWT) and the SHA-256 of the live refresh jti.
CREATE TABLE usr.usr_session (
    id                          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    expires_at                  TIMESTAMPTZ NOT NULL,
    revoked_at                  TIMESTAMPTZ NULL,
    revocation_reason           TEXT        NULL,
    refresh_token_hash          BYTEA       NOT NULL,
    previous_refresh_token_hash BYTEA       NULL,
    ip_address                  INET        NULL,
    user_agent                  TEXT        NULL,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_usr_session_revocation_reason CHECK (
        revocation_reason IN ('user_logout', 'token_reuse', 'password_changed', 'expired', 'account_disabled', 'admin_revoked')
    ),
    CONSTRAINT chk_usr_session_revocation_consistency CHECK ((revoked_at IS NULL) = (revocation_reason IS NULL))
);

CREATE INDEX idx_usr_session_user_id ON usr.usr_session (user_id);

-- Secrets sent by email, stored as SHA-256. One live token per user and purpose.
CREATE TABLE usr.usr_token (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    purpose    TEXT        NOT NULL,
    token_hash BYTEA       NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_usr_token_purpose CHECK (purpose IN ('activation', 'password_reset'))
);

CREATE UNIQUE INDEX uni_usr_token_live ON usr.usr_token (user_id, purpose) WHERE used_at IS NULL;
CREATE INDEX idx_usr_token_issued ON usr.usr_token (user_id, purpose, created_at);

-- Every login, including unknown emails, so there is no FK to usr_user.
CREATE TABLE usr.usr_login_attempt (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    email        CITEXT      NOT NULL,
    ip_address   INET        NULL,
    user_agent   TEXT        NULL,
    success      BOOLEAN     NOT NULL,
    failure_kind TEXT        NULL,
    CONSTRAINT chk_usr_login_attempt_failure_kind CHECK (
        failure_kind IN (
            'bad_password', 'user_not_found', 'account_locked', 'account_disabled',
            'account_pending', 'rate_limited_ip', 'rate_limited_email'
        )
    ),
    CONSTRAINT chk_usr_login_attempt_success_consistency CHECK (success = (failure_kind IS NULL))
);

-- The predicate must equal the one in attempt/repository.go, or the planner
-- ignores these two indexes.
CREATE INDEX idx_usr_login_attempt_email_counted
    ON usr.usr_login_attempt (email, created_at)
    WHERE success = false
      AND failure_kind NOT IN ('rate_limited_email', 'rate_limited_ip', 'account_locked');
CREATE INDEX idx_usr_login_attempt_ip_counted
    ON usr.usr_login_attempt (ip_address, created_at)
    WHERE success = false
      AND failure_kind NOT IN ('rate_limited_email', 'rate_limited_ip', 'account_locked');
CREATE INDEX idx_usr_login_attempt_created ON usr.usr_login_attempt (created_at);

-- RBAC. actions is an INT4 mask: create=1 read=2 update=4 delete=8. Bits 4-30 are
-- free, bit 31 is the sign. Never reuse an index. The view does not expand the
-- sentinel '*' and leaves is_superuser out: the application applies both.
-- usr_resource.actions lists the actions its routes check, so a role cannot hold
-- a grant that does nothing (audit is read-only; users are disabled, never deleted).
CREATE TABLE usr.usr_resource (
    code        VARCHAR(100) PRIMARY KEY,
    description TEXT         NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    actions     INTEGER      NOT NULL DEFAULT 15,
    CONSTRAINT chk_usr_resource_code CHECK (code ~ '^[a-z0-9_-]{2,100}$')
);

CREATE TABLE usr.usr_role (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL UNIQUE,
    enabled      BOOLEAN     NOT NULL DEFAULT true,
    is_default   BOOLEAN     NOT NULL DEFAULT false,
    is_superuser BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_usr_role_name CHECK (name ~ '^[a-z0-9_-]{2,40}$')
);
-- Several roles may be default: activation gives a new account every enabled one.

CREATE TABLE usr.usr_role_permission (
    role_id      UUID         NOT NULL REFERENCES usr.usr_role (id) ON DELETE CASCADE,
    resource     VARCHAR(100) NOT NULL,
    actions      INT          NOT NULL,
    -- NULL for '*', which is how the FK skips the sentinel.
    catalog_code VARCHAR(100) GENERATED ALWAYS AS (NULLIF(resource, '*')) STORED
                 REFERENCES usr.usr_resource (code),
    PRIMARY KEY (role_id, resource),
    CONSTRAINT chk_usr_role_permission_actions CHECK (actions >= 0)
);

CREATE TABLE usr.usr_user_role (
    user_id    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    role_id    UUID        NOT NULL REFERENCES usr.usr_role (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_usr_user_role_role_id ON usr.usr_user_role (role_id);

CREATE VIEW usr.vw_user_effective_permissions AS
SELECT ur.user_id, rp.resource, bit_or(rp.actions) AS actions
FROM usr.usr_user_role ur
JOIN usr.usr_role r ON r.id = ur.role_id AND r.enabled
JOIN usr.usr_role_permission rp ON rp.role_id = r.id
GROUP BY ur.user_id, rp.resource;

-- Without a row, the application uses the default preferences.
CREATE TABLE usr.usr_preference (
    user_id    UUID        PRIMARY KEY REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    theme      VARCHAR(10) NOT NULL DEFAULT 'system',
    language   VARCHAR(5)  NOT NULL DEFAULT 'pt-BR',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_usr_preference_theme CHECK (theme IN ('system', 'light', 'dark'))
);

CREATE TABLE usr.usr_notification (
    id         UUID         PRIMARY KEY,
    user_id    UUID         NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    level      VARCHAR(10)  NOT NULL,
    title      VARCHAR(120) NOT NULL,
    message    TEXT         NOT NULL,
    -- kind and params let each client write the notice in the user's language;
    -- title and message stay as the English fallback.
    kind       TEXT         NULL,
    params     JSONB        NOT NULL DEFAULT '{}',
    read_at    TIMESTAMPTZ  NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_usr_notification_level CHECK (level IN ('info', 'warning', 'critical'))
);

CREATE INDEX idx_usr_notification_user_created ON usr.usr_notification (user_id, created_at DESC);

-- Administrative changes, written in the transaction of the change and readable
-- through the API (audit:read). No FK: the record outlives the user. A NULL actor is the operator CLI. The app may only
-- append; nothing in the schema cleans it up.
CREATE TABLE usr.usr_audit_log (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id   UUID        NULL,
    request_id TEXT        NULL,
    action     TEXT        NOT NULL,
    target_id  UUID        NULL,
    details    JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_usr_audit_log_created ON usr.usr_audit_log (created_at);
CREATE INDEX idx_usr_audit_log_target ON usr.usr_audit_log (target_id, created_at);
CREATE INDEX idx_usr_audit_log_actor ON usr.usr_audit_log (actor_id, created_at);
-- The action filter is a prefix (LIKE 'product.%'): without it a rare action reads the whole log.
CREATE INDEX idx_usr_audit_log_action ON usr.usr_audit_log (action text_pattern_ops, created_at DESC);
REVOKE UPDATE, DELETE ON usr.usr_audit_log FROM dataset_app;

CREATE TRIGGER trg_usr_user_set_updated_at BEFORE UPDATE ON usr.usr_user
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_credential_set_updated_at BEFORE UPDATE ON usr.usr_credential
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_role_set_updated_at BEFORE UPDATE ON usr.usr_role
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_preference_set_updated_at BEFORE UPDATE ON usr.usr_preference
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

INSERT INTO usr.usr_resource (code, description, actions) VALUES
    ('users', 'Accounts and profile', 7),
    ('roles', 'RBAC roles', 15),  -- reading the catalog is part of roles:read
    ('audit', 'Audit log of administrative changes', 2);

INSERT INTO usr.usr_role (name, is_default, is_superuser) VALUES
    ('user',  true,  false),
    ('admin', false, true);

INSERT INTO usr.usr_role_permission (role_id, resource, actions)
SELECT id, '*', 2147483647 FROM usr.usr_role WHERE name = 'admin';
