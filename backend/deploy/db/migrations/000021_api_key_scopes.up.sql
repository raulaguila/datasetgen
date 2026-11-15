-- A personal API key reaches the collections its maker picks, each read only or
-- read and write. A share link still belongs to one collection (api_key.collection_id).
CREATE TABLE app.api_key_collection (
    key_id        UUID    NOT NULL REFERENCES app.api_key (id) ON DELETE CASCADE,
    collection_id UUID    NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    can_write     BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (key_id, collection_id)
);
CREATE INDEX idx_app_api_key_collection_collection ON app.api_key_collection (collection_id);

INSERT INTO app.api_key_collection (key_id, collection_id, can_write)
SELECT id, collection_id, can_write FROM app.api_key WHERE share IS NULL;

ALTER TABLE app.api_key ALTER COLUMN collection_id DROP NOT NULL;
UPDATE app.api_key SET collection_id = NULL, can_write = false WHERE share IS NULL;
ALTER TABLE app.api_key ADD CONSTRAINT api_key_share_collection CHECK ((share IS NULL) = (collection_id IS NULL));
CREATE INDEX idx_app_api_key_creator ON app.api_key (created_by, created_at) WHERE share IS NULL;
