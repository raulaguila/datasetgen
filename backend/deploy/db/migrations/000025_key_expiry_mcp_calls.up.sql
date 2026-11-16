-- A key may stop working on a date its maker picks; NULL never expires.
ALTER TABLE app.api_key ADD COLUMN expires_at TIMESTAMPTZ;

-- What AI assistants did in a collection through MCP: which key, which tool,
-- whether it worked. Kept 30 days (the hourly sweep deletes older rows).
CREATE TABLE app.mcp_call (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    key_id        UUID        REFERENCES app.api_key (id) ON DELETE SET NULL,
    key_name      TEXT        NOT NULL,
    user_id       UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    tool          TEXT        NOT NULL,
    ok            BOOLEAN     NOT NULL,
    error         TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_mcp_call_collection ON app.mcp_call (collection_id, created_at DESC);
CREATE INDEX idx_app_mcp_call_created ON app.mcp_call (created_at);
