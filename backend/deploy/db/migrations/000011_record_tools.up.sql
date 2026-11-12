-- Earlier versions of each record: every change that replaces data saves the
-- data it replaces here first, so a record can be compared and restored.
-- shortcut: kept forever; prune to the last N per record when the table grows.
CREATE TABLE app.record_version (
    record_id UUID        NOT NULL REFERENCES app.record (id) ON DELETE CASCADE,
    version   INTEGER     NOT NULL,
    data      JSONB       NOT NULL,
    -- When this version was written: the record's updated_at at that time.
    saved_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (record_id, version)
);

-- Per-field filters on the record list use containment (data @> '{"field": value}').
CREATE INDEX idx_app_record_data_path ON app.record USING gin (data jsonb_path_ops);

-- The latest quality check of each collection, written by a background job.
CREATE TABLE app.collection_quality (
    collection_id  UUID        PRIMARY KEY REFERENCES app.collection (id) ON DELETE CASCADE,
    schema_version INTEGER     NOT NULL,
    status         TEXT        NOT NULL,
    checked        INTEGER     NOT NULL DEFAULT 0,
    invalid_count  INTEGER     NOT NULL DEFAULT 0,
    truncated      BOOLEAN     NOT NULL DEFAULT false,
    -- The first invalid records with their issues, as the report shows them.
    invalid        JSONB       NOT NULL DEFAULT '[]',
    -- Every invalid record found, for "delete the invalid records".
    invalid_ids    UUID[]      NOT NULL DEFAULT '{}',
    error          TEXT        NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ,
    CONSTRAINT chk_app_collection_quality_status CHECK (status IN ('running', 'done', 'failed'))
);
