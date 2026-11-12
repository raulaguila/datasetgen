-- One record of a collection; data is validated against the collection schema by the backend.

CREATE TABLE app.record (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- RESTRICT: a collection with records cannot be deleted by accident.
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE RESTRICT,
    data          JSONB       NOT NULL,
    version       INTEGER     NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_app_record_data_size CHECK (pg_column_size(data) <= 262144)
);

CREATE INDEX idx_app_record_collection ON app.record (collection_id, created_at DESC, id DESC);
CREATE INDEX idx_app_record_data_trgm ON app.record USING gin ((data::text) gin_trgm_ops);

CREATE TRIGGER trg_app_record_set_updated_at BEFORE UPDATE ON app.record
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

INSERT INTO usr.usr_resource (code, description) VALUES ('records', 'Records inside collections');
