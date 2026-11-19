-- Identity: accounts and usernames, credentials and two-step login, sessions,
-- email tokens, login attempts, roles, preferences and notifications.

-- Lowercase letters, digits and inner single hyphens, 1-39 characters (GitHub's rule).
-- Reserved: the frontend's top-level routes and a few names kept for later.
-- domain.ReservedUsername holds the same list; both change together.
CREATE FUNCTION usr.fn_username_reserved(name TEXT) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
  SELECT name = ANY (ARRAY[
    'about', 'account', 'activate', 'admin', 'api', 'api-access', 'app', 'assets', 'c', 'collections',
    'dashboard', 'dataset', 'docs', 'explore', 'f', 'fonts', 'forgot-password', 'help', 'home', 'login',
    'logout', 'me', 'new', 'notifications', 'oauth', 'profile', 'public', 'r', 'records', 'register',
    'reset-password', 'root', 's', 'search', 'settings', 'signin', 'signup', 'static', 'system', 'u',
    'user', 'users', 'www'])
$$;

-- username is the account's part of collection addresses (/<username>/<slug>).
CREATE TABLE usr.usr_user (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    email      CITEXT      NOT NULL,
    username   TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'pending',
    -- When the user last set a profile picture; NULL means none. The image itself
    -- lives in the file store under avatars/<user id>.
    avatar_at  TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_usr_user_email UNIQUE (email),
    CONSTRAINT uq_usr_user_username UNIQUE (username),
    CONSTRAINT chk_usr_user_name_len CHECK (char_length(name) BETWEEN 2 AND 80),
    CONSTRAINT chk_usr_user_status CHECK (status IN ('pending', 'active', 'locked', 'disabled')),
    CONSTRAINT chk_usr_user_username CHECK (char_length(username) <= 39 AND
        username ~ '^[a-z0-9](?:[a-z0-9]|-(?=[a-z0-9])){0,38}$' AND NOT usr.fn_username_reserved(username))
);

-- The user list sorts by these, and its search is a substring match on name and email.
CREATE INDEX idx_usr_user_name ON usr.usr_user (name, id);
CREATE INDEX idx_usr_user_created ON usr.usr_user (created_at DESC, id);
CREATE INDEX idx_usr_user_name_trgm ON usr.usr_user USING gin (name gin_trgm_ops);
CREATE INDEX idx_usr_user_email_trgm ON usr.usr_user USING gin ((email::text) gin_trgm_ops);

-- The first free username from base: base, base-2, base-3...
CREATE FUNCTION usr.fn_free_username(base TEXT, except_id UUID) RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE
  root TEXT := coalesce(nullif(public.fn_slugify(base, 34), ''), 'user');
  candidate TEXT := root;
  n INTEGER := 1;
BEGIN
  LOOP
    EXIT WHEN NOT usr.fn_username_reserved(candidate)
      AND NOT EXISTS (SELECT 1 FROM usr.usr_user WHERE username = candidate AND id IS DISTINCT FROM except_id);
    n := n + 1;
    candidate := root || '-' || n;
  END LOOP;
  RETURN candidate;
END $$;

-- A new account without a username gets one from its email, on every insert path
-- (API, admin CLI, SSO); activation may choose another.
CREATE FUNCTION usr.fn_user_username() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.username IS NULL OR NEW.username = '' THEN
    NEW.username := usr.fn_free_username(split_part(NEW.email::text, '@', 1), NEW.id);
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_usr_user_username BEFORE INSERT ON usr.usr_user
    FOR EACH ROW EXECUTE FUNCTION usr.fn_user_username();

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

CREATE INDEX idx_usr_session_user ON usr.usr_session (user_id);

-- Secrets sent by email, stored as SHA-256. One live token per user and purpose.
CREATE TABLE usr.usr_token (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    purpose    TEXT        NOT NULL,
    token_hash BYTEA       NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_usr_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_usr_token_purpose CHECK (purpose IN ('activation', 'password_reset'))
);

CREATE UNIQUE INDEX uq_usr_token_live ON usr.usr_token (user_id, purpose) WHERE used_at IS NULL;
-- The cooldown between two emails of one purpose.
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
            'bad_password', 'bad_code', 'user_not_found', 'account_locked', 'account_disabled',
            'account_pending', 'rate_limited_ip', 'rate_limited_email'
        )
    ),
    CONSTRAINT chk_usr_login_attempt_success_consistency CHECK (success = (failure_kind IS NULL))
);

-- The predicate must equal shared.CountedFailure, or the planner ignores these two indexes.
CREATE INDEX idx_usr_login_attempt_email_counted
    ON usr.usr_login_attempt (email, created_at)
    WHERE success = false
      AND failure_kind NOT IN ('rate_limited_email', 'rate_limited_ip', 'account_locked');
CREATE INDEX idx_usr_login_attempt_ip_counted
    ON usr.usr_login_attempt (ip_address, created_at)
    WHERE success = false
      AND failure_kind NOT IN ('rate_limited_email', 'rate_limited_ip', 'account_locked');
CREATE INDEX idx_usr_login_attempt_created ON usr.usr_login_attempt (created_at);

-- Platform roles. actions is an INT4 mask: create=1 read=2 update=4 delete=8; bit 31
-- is the sign, never reuse an index. resource is one the routes check (domain.Resource*)
-- or the sentinel '*'. The view does not expand '*' and leaves is_superuser out: the
-- application applies both. Collections and records are governed by membership instead.
CREATE TABLE usr.usr_role (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    enabled      BOOLEAN     NOT NULL DEFAULT true,
    -- Activation gives a new account every enabled default role.
    is_default   BOOLEAN     NOT NULL DEFAULT false,
    is_superuser BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_usr_role_name UNIQUE (name),
    CONSTRAINT chk_usr_role_name CHECK (name ~ '^[a-z0-9_-]{2,40}$')
);

CREATE TABLE usr.usr_role_permission (
    role_id  UUID NOT NULL REFERENCES usr.usr_role (id) ON DELETE CASCADE,
    resource TEXT NOT NULL,
    actions  INT  NOT NULL,
    PRIMARY KEY (role_id, resource),
    CONSTRAINT chk_usr_role_permission_resource CHECK (resource IN ('*', 'audit', 'collections', 'roles', 'users')),
    CONSTRAINT chk_usr_role_permission_actions CHECK (actions >= 0)
);

CREATE TABLE usr.usr_user_role (
    user_id    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    role_id    UUID        NOT NULL REFERENCES usr.usr_role (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_usr_user_role_role ON usr.usr_user_role (role_id);

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
-- Every open tab asks for the unread count once a minute.
CREATE INDEX idx_usr_notification_unread ON usr.usr_notification (user_id) WHERE read_at IS NULL;

CREATE TRIGGER trg_usr_user_set_updated_at BEFORE UPDATE ON usr.usr_user
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_credential_set_updated_at BEFORE UPDATE ON usr.usr_credential
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_role_set_updated_at BEFORE UPDATE ON usr.usr_role
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE TRIGGER trg_usr_preference_set_updated_at BEFORE UPDATE ON usr.usr_preference
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- Every account may start its own collections (collections:create); admin holds everything.
INSERT INTO usr.usr_role (name, is_default, is_superuser) VALUES
    ('user',  true,  false),
    ('admin', false, true);

INSERT INTO usr.usr_role_permission (role_id, resource, actions)
SELECT id, '*', 2147483647 FROM usr.usr_role WHERE name = 'admin'
UNION ALL
SELECT id, 'collections', 1 FROM usr.usr_role WHERE name = 'user';
