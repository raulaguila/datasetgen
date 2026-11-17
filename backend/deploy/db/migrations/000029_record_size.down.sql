-- Fails while any record is over 256 KB.
ALTER TABLE app.record
    DROP CONSTRAINT chk_app_record_data_size,
    ADD CONSTRAINT chk_app_record_data_size CHECK (pg_column_size(data) <= 262144);
