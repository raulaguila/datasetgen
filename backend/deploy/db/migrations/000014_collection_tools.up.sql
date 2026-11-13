-- API keys: a program reads (or writes) one collection's records as the
-- member who made the key, never with more than that member's role.
CREATE TABLE app.api_key (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    -- The first characters of the key, shown to tell keys apart; the key itself is never stored.
    prefix        TEXT        NOT NULL,
    key_hash      BYTEA       NOT NULL UNIQUE,
    can_write     BOOLEAN     NOT NULL DEFAULT false,
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);
CREATE INDEX idx_app_api_key_collection ON app.api_key (collection_id, created_at);

-- Webhooks: record changes POSTed to a URL, signed with the hook's secret.
CREATE TABLE app.webhook (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    url           TEXT        NOT NULL,
    -- AES-GCM sealed; the server needs it back to sign each delivery.
    secret        TEXT        NOT NULL,
    events        TEXT[]      NOT NULL,
    created_by    UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status   INTEGER,
    last_error    TEXT        NOT NULL DEFAULT '',
    last_at       TIMESTAMPTZ
);
CREATE INDEX idx_app_webhook_collection ON app.webhook (collection_id);

-- Deleted records wait here 30 days; restoring puts them back with the same id.
CREATE TABLE app.record_trash (
    id            UUID        PRIMARY KEY,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    data          JSONB       NOT NULL,
    version       INTEGER     NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    deleted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_by    UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL
);
CREATE INDEX idx_app_record_trash_collection ON app.record_trash (collection_id, deleted_at DESC);
-- A record's versions outlive its move to the trash; the purge drops them.
ALTER TABLE app.record_version DROP CONSTRAINT record_version_record_id_fkey;

-- A deleted collection keeps its records and members until the purge.
ALTER TABLE app.collection ADD COLUMN deleted_at TIMESTAMPTZ, ADD COLUMN deleted_by UUID REFERENCES usr.usr_user (id) ON DELETE SET NULL;
CREATE INDEX idx_app_collection_deleted ON app.collection (deleted_at) WHERE deleted_at IS NOT NULL;

-- Each schema a collection had, saved when a change replaces it.
CREATE TABLE app.collection_schema_version (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    version       INTEGER     NOT NULL,
    -- json, not jsonb: the key order is the field order.
    schema        JSON        NOT NULL,
    saved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    saved_by      UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    PRIMARY KEY (collection_id, version)
);

-- Named record list queries: a member's own, or shared with the collection.
CREATE TABLE app.saved_view (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    -- The list's query string (filters, search, sort, columns).
    query         TEXT        NOT NULL,
    shared        BOOLEAN     NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_saved_view_collection ON app.saved_view (collection_id, name);

-- Files attached to records; the bytes live in the file store under files/<id>.
CREATE TABLE app.file (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    content_type  TEXT        NOT NULL,
    size          INTEGER     NOT NULL,
    created_by    UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_file_collection ON app.file (collection_id);

-- Empties the trash: records and collections deleted more than retention ago.
CREATE FUNCTION app.fn_purge_trash(retention INTERVAL DEFAULT INTERVAL '30 days')
RETURNS void
LANGUAGE sql
AS $$
  DELETE FROM app.record_version v WHERE EXISTS (SELECT 1 FROM app.record_trash t WHERE t.id = v.record_id AND t.deleted_at < now() - retention);
  DELETE FROM app.record_trash WHERE deleted_at < now() - retention;
  DELETE FROM app.record_version v USING app.record r, app.collection c
   WHERE v.record_id = r.id AND r.collection_id = c.id AND c.deleted_at < now() - retention;
  DELETE FROM app.record r USING app.collection c WHERE r.collection_id = c.id AND c.deleted_at < now() - retention;
  DELETE FROM app.collection WHERE deleted_at < now() - retention;
$$;

DO $$
BEGIN
  IF current_setting('shared_preload_libraries', true) LIKE '%pg_cron%'
     AND current_setting('cron.database_name', true) = current_database() THEN
    PERFORM cron.schedule('dataset_trash', '37 3 * * *', 'SELECT app.fn_purge_trash()');
  END IF;
END;
$$;
