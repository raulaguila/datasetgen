-- A collection's audit log also holds its records' and members' entries, which
-- name the collection in details.collection_id.
CREATE INDEX idx_usr_audit_log_collection ON usr.usr_audit_log ((details->>'collection_id'), created_at);

-- The quality check walks a collection's records by id.
CREATE INDEX idx_app_record_collection_key ON app.record (collection_id, id);
