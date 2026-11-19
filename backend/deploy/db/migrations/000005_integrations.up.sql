-- How programs and people outside the app reach a collection: personal API keys,
-- share links, webhooks, automations and the MCP tools with their call log.

-- A personal API key: a program reads (or writes) the collections its maker picks
-- (api_key_collection), as that member, never with more than the member's role.
-- The key itself is never stored, only its SHA-256.
CREATE TABLE app.api_key (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    -- The first characters of the key, shown to tell keys apart.
    prefix       TEXT        NOT NULL,
    key_hash     BYTEA       NOT NULL,
    created_by   UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NULL,
    -- NULL never expires.
    expires_at   TIMESTAMPTZ NULL,
    CONSTRAINT uq_app_api_key_hash UNIQUE (key_hash)
);
CREATE INDEX idx_app_api_key_creator ON app.api_key (created_by, created_at);

CREATE TABLE app.api_key_collection (
    key_id        UUID    NOT NULL REFERENCES app.api_key (id) ON DELETE CASCADE,
    collection_id UUID    NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    can_write     BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (key_id, collection_id)
);
CREATE INDEX idx_app_api_key_collection_collection ON app.api_key_collection (collection_id);

-- A share link: a public form that adds records, or a read-only list, acting as its
-- maker within one collection. The token is sealed (AES-GCM) so the owner can copy
-- it again; its SHA-256 finds the link.
CREATE TABLE app.share_link (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    kind          TEXT        NOT NULL,
    token_hash    BYTEA       NOT NULL,
    sealed        TEXT        NOT NULL,
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ NULL,
    expires_at    TIMESTAMPTZ NULL,
    CONSTRAINT uq_app_share_link_hash UNIQUE (token_hash),
    CONSTRAINT chk_app_share_link_kind CHECK (kind IN ('form', 'view'))
);
CREATE INDEX idx_app_share_link_collection ON app.share_link (collection_id, created_at DESC);

-- Webhooks: record changes POSTed to a URL, signed with the hook's secret. A hook
-- that keeps failing pauses itself, so a dead receiver stops filling the job queue.
CREATE TABLE app.webhook (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    url           TEXT        NOT NULL,
    -- AES-GCM sealed; the server needs it back to sign each delivery.
    secret        TEXT        NOT NULL,
    events        TEXT[]      NOT NULL,
    created_by    UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status   INTEGER     NULL,
    last_error    TEXT        NOT NULL DEFAULT '',
    last_at       TIMESTAMPTZ NULL,
    failures      INTEGER     NOT NULL DEFAULT 0,
    paused_at     TIMESTAMPTZ NULL
);
CREATE INDEX idx_app_webhook_collection ON app.webhook (collection_id);

-- Each webhook call and its answer, newest 50 per hook, so a failed one can be sent again.
CREATE TABLE app.webhook_delivery (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID        NOT NULL REFERENCES app.webhook (id) ON DELETE CASCADE,
    event      TEXT        NOT NULL,
    body       JSONB       NOT NULL,
    status     INTEGER     NULL,
    error      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_webhook_delivery_hook ON app.webhook_delivery (webhook_id, created_at DESC);

-- Automations: when a record is created or changed (or once a day), records that pass
-- the conditions get an action: set a field, notify the maker, send an email, call one
-- of the collection's webhooks, or create a record in a collection.
CREATE TABLE app.automation (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    trigger       TEXT        NOT NULL,
    -- The daily run's hour (server time); NULL for record triggers.
    hour          SMALLINT    NULL,
    conditions    JSONB       NOT NULL DEFAULT '{"any": false, "rules": []}',
    action        TEXT        NOT NULL,
    config        JSONB       NOT NULL,
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run_at   TIMESTAMPTZ NULL,
    last_error    TEXT        NOT NULL DEFAULT '',
    runs          INTEGER     NOT NULL DEFAULT 0,
    CONSTRAINT chk_app_automation_trigger CHECK (trigger IN ('record.created', 'record.updated', 'daily')),
    CONSTRAINT chk_app_automation_hour CHECK (hour BETWEEN 0 AND 23),
    CONSTRAINT chk_app_automation_action CHECK (action IN ('set_field', 'notify', 'email', 'webhook', 'create_record'))
);
CREATE INDEX idx_app_automation_collection ON app.automation (collection_id, trigger) WHERE enabled;

-- Tools a collection owner adds to the MCP server: a described, read-only search with
-- fixed conditions, typed parameters bound to fields, and chosen fields back.
CREATE TABLE app.mcp_tool (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT        NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    conditions    JSONB       NOT NULL DEFAULT '{"any": false, "rules": []}',
    params        JSONB       NOT NULL DEFAULT '[]',
    fields        JSONB       NOT NULL DEFAULT '[]',
    sort          TEXT        NOT NULL DEFAULT 'updated_at',
    sort_order    TEXT        NOT NULL DEFAULT 'desc',
    max_records   SMALLINT    NOT NULL DEFAULT 20,
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_app_mcp_tool_name UNIQUE (collection_id, name),
    CONSTRAINT chk_app_mcp_tool_name CHECK (name ~ '^[a-z][a-z0-9_]{2,47}$'),
    CONSTRAINT chk_app_mcp_tool_sort_order CHECK (sort_order IN ('asc', 'desc')),
    CONSTRAINT chk_app_mcp_tool_max_records CHECK (max_records BETWEEN 1 AND 50)
);

CREATE TRIGGER trg_app_mcp_tool_set_updated_at BEFORE UPDATE ON app.mcp_tool
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- What AI assistants did in a collection through MCP: which key, which tool, whether
-- it worked, and its arguments cut to 500 characters. Kept 30 days (the hourly sweep).
CREATE TABLE app.mcp_call (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    key_id        UUID        NULL REFERENCES app.api_key (id) ON DELETE SET NULL,
    key_name      TEXT        NOT NULL,
    user_id       UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    tool          TEXT        NOT NULL,
    args          TEXT        NOT NULL DEFAULT '',
    ok            BOOLEAN     NOT NULL,
    error         TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_mcp_call_collection ON app.mcp_call (collection_id, created_at DESC);
-- A key's own log (the "usage" link on the API keys page).
CREATE INDEX idx_app_mcp_call_key ON app.mcp_call (key_id, created_at DESC);
CREATE INDEX idx_app_mcp_call_created ON app.mcp_call (created_at);
