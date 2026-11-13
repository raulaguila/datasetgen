-- The record list sorts by last change as well as by creation.
CREATE INDEX idx_app_record_collection_updated ON app.record (collection_id, updated_at DESC, id DESC);
