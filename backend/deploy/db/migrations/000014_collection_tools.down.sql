DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_cron') THEN
    PERFORM cron.unschedule('dataset_trash');
  END IF;
EXCEPTION WHEN OTHERS THEN NULL;
END;
$$;
DROP FUNCTION IF EXISTS app.fn_purge_trash(INTERVAL);
DROP TABLE IF EXISTS app.file;
DROP TABLE IF EXISTS app.saved_view;
DROP TABLE IF EXISTS app.collection_schema_version;
-- Trashed collections and records cannot come back once these go.
DELETE FROM app.record_version v WHERE NOT EXISTS (SELECT 1 FROM app.record r WHERE r.id = v.record_id);
ALTER TABLE app.record_version ADD CONSTRAINT record_version_record_id_fkey FOREIGN KEY (record_id) REFERENCES app.record (id) ON DELETE CASCADE;
DROP TABLE IF EXISTS app.record_trash;
DROP INDEX IF EXISTS app.idx_app_collection_deleted;
ALTER TABLE app.collection DROP COLUMN IF EXISTS deleted_by, DROP COLUMN IF EXISTS deleted_at;
DROP TABLE IF EXISTS app.webhook;
DROP TABLE IF EXISTS app.api_key;
