-- json keeps the schema text as written; jsonb sorts object keys by length,
-- which reordered the fields of every generated form.
ALTER TABLE app.collection DROP CONSTRAINT chk_app_collection_schema_object;
ALTER TABLE app.collection ALTER COLUMN schema TYPE JSON USING schema::json;
ALTER TABLE app.collection ADD CONSTRAINT chk_app_collection_schema_object CHECK (json_typeof(schema) = 'object');
