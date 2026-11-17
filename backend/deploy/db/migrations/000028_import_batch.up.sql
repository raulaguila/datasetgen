-- An import batch already saved, by the key its client gave it: a retry after a
-- lost answer gets the first result back instead of saving the records twice.
-- Kept a week (the hourly sweep), long enough to resume a stopped import.
CREATE TABLE app.import_batch (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    batch_key     TEXT        NOT NULL,
    created       BIGINT      NOT NULL DEFAULT 0,
    updated       BIGINT      NOT NULL DEFAULT 0,
    saved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, batch_key)
);
CREATE INDEX idx_import_batch_saved_at ON app.import_batch (saved_at);
