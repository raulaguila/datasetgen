-- A key goes back to one collection: its first; keys that reach none are dropped.
DROP INDEX app.idx_app_api_key_creator;
ALTER TABLE app.api_key DROP CONSTRAINT api_key_share_collection;
UPDATE app.api_key k SET collection_id = s.collection_id, can_write = s.can_write
FROM (SELECT DISTINCT ON (key_id) key_id, collection_id, can_write FROM app.api_key_collection ORDER BY key_id, collection_id) s
WHERE s.key_id = k.id;
DELETE FROM app.api_key WHERE collection_id IS NULL;
ALTER TABLE app.api_key ALTER COLUMN collection_id SET NOT NULL;
DROP TABLE app.api_key_collection;
