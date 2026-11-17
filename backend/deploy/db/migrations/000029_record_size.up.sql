-- One record may hold up to 1 MB (was 256 KB), enough for long-context
-- training examples; import refuses a larger record alone (Records.Oversized).
ALTER TABLE app.record
    DROP CONSTRAINT chk_app_record_data_size,
    ADD CONSTRAINT chk_app_record_data_size CHECK (pg_column_size(data) <= 1048576);
