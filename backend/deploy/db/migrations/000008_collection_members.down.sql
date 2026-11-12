DROP INDEX app.idx_app_collection_name;
-- Fails when two collections now share a name; rename one first.
ALTER TABLE app.collection ADD CONSTRAINT uq_app_collection_name UNIQUE (name);
DROP TABLE app.collection_member;
