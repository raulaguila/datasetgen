ALTER TABLE app.collection DROP CONSTRAINT chk_app_collection_schema_object;
ALTER TABLE app.collection ALTER COLUMN schema TYPE JSONB USING schema::jsonb;
ALTER TABLE app.collection ADD CONSTRAINT chk_app_collection_schema_object CHECK (jsonb_typeof(schema) = 'object');
