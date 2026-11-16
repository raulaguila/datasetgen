-- A hook that keeps failing pauses itself, so a dead receiver stops filling the job queue.
ALTER TABLE app.webhook
    ADD COLUMN failures  INTEGER     NOT NULL DEFAULT 0,
    ADD COLUMN paused_at TIMESTAMPTZ;

-- A key's own MCP log (the "usage" link on the API keys page).
CREATE INDEX idx_app_mcp_call_key ON app.mcp_call (key_id, created_at DESC);
