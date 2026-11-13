DELETE FROM app.api_key WHERE share IS NOT NULL;
ALTER TABLE app.api_key DROP CONSTRAINT api_key_share_sealed, DROP COLUMN sealed, DROP COLUMN share;
