ALTER TABLE app.webhook DROP COLUMN paused_at, DROP COLUMN failures;
DROP INDEX IF EXISTS app.idx_app_mcp_call_key;
