-- A collection holds records that follow its JSON Schema (draft 2020-12).

CREATE TABLE app.collection (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    schema      JSONB       NOT NULL,
    -- Optimistic concurrency: every update bumps it, and an update that names the
    -- version it read is refused when someone else changed the row since.
    version     INTEGER     NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_app_collection_name UNIQUE (name),
    CONSTRAINT chk_app_collection_name_len CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT chk_app_collection_description_len CHECK (char_length(description) <= 2000),
    CONSTRAINT chk_app_collection_schema_object CHECK (jsonb_typeof(schema) = 'object')
);

CREATE INDEX idx_app_collection_name_trgm ON app.collection USING gin (name gin_trgm_ops);
CREATE INDEX idx_app_collection_created ON app.collection (created_at DESC, id DESC);

CREATE TRIGGER trg_app_collection_set_updated_at BEFORE UPDATE ON app.collection
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

INSERT INTO usr.usr_resource (code, description) VALUES ('collections', 'Collections and their schemas');
