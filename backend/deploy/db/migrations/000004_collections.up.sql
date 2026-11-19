-- Collections and their records: members, addresses, versions, the trash, quality,
-- dashboards, saved views, files, comments and import batches.

-- A collection holds records that follow its JSON Schema (draft 2020-12). schema is
-- json, not jsonb: jsonb sorts object keys by length, and the key order of
-- properties is the field order of the generated form.
-- owner_id is the account in its address (/<username>/<slug>); it stays an owner
-- member (the service refuses removing or demoting it) until a transfer hands the
-- collection to another owner. A deleted collection keeps its records and members
-- until the purge, and gives its address up.
CREATE TABLE app.collection (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    UUID        NOT NULL REFERENCES usr.usr_user (id),
    slug        TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    -- A curated icon instead of the initials ('' = initials); domain.ValidCollectionIcon.
    icon        TEXT        NOT NULL DEFAULT '',
    schema      JSON        NOT NULL,
    -- Optimistic concurrency: every update bumps it, and an update that names the
    -- version it read is refused when someone else changed the row since.
    version     INTEGER     NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ NULL,
    deleted_by  UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    CONSTRAINT chk_app_collection_name_len CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT chk_app_collection_description_len CHECK (char_length(description) <= 2000),
    CONSTRAINT chk_app_collection_schema_object CHECK (json_typeof(schema) = 'object'),
    CONSTRAINT chk_app_collection_slug CHECK (char_length(slug) <= 100 AND slug ~ '^[a-z0-9](?:[a-z0-9]|-(?=[a-z0-9])){0,99}$')
);

-- Names are not unique: a hidden collection must neither block a name nor reveal it exists.
CREATE INDEX idx_app_collection_name ON app.collection (name, id);
CREATE INDEX idx_app_collection_name_trgm ON app.collection USING gin (name gin_trgm_ops);
CREATE INDEX idx_app_collection_created ON app.collection (created_at DESC, id DESC);
CREATE INDEX idx_app_collection_owner ON app.collection (owner_id);
CREATE INDEX idx_app_collection_deleted ON app.collection (deleted_at) WHERE deleted_at IS NOT NULL;
-- A deleted collection gives its path up; a restore picks a free one (trigger below).
CREATE UNIQUE INDEX uq_app_collection_owner_slug ON app.collection (owner_id, slug) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_app_collection_set_updated_at BEFORE UPDATE ON app.collection
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- Who works on each collection. Access to a collection and its records comes from
-- this table, not from roles: owner manages fields, settings and members; editor
-- writes records; viewer reads. A platform admin (superuser role or '*') sees all.
-- A member may be a pending account: inviting by email creates it and adds it here.
CREATE TABLE app.collection_member (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    role          TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, user_id),
    CONSTRAINT chk_app_collection_member_role CHECK (role IN ('owner', 'editor', 'viewer'))
);

-- The sidebar lists a person's collections.
CREATE INDEX idx_app_collection_member_user ON app.collection_member (user_id);

-- Old paths that still lead somewhere: renames, transfers, restores and username
-- changes keep the previous one. The newest move of a path wins.
CREATE TABLE app.collection_path (
    username      TEXT        NOT NULL,
    slug          TEXT        NOT NULL,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (username, slug)
);
CREATE INDEX idx_app_collection_path_collection ON app.collection_path (collection_id);

-- The first free slug from name under owner, skipping the collection itself.
CREATE FUNCTION app.fn_free_slug(owner UUID, name TEXT, except_id UUID) RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE
  root TEXT := coalesce(nullif(public.fn_slugify(name, 90), ''), 'collection');
  candidate TEXT := root;
  n INTEGER := 1;
BEGIN
  LOOP
    EXIT WHEN NOT EXISTS (SELECT 1 FROM app.collection
      WHERE owner_id = owner AND slug = candidate AND deleted_at IS NULL AND id IS DISTINCT FROM except_id);
    n := n + 1;
    candidate := root || '-' || n;
  END LOOP;
  RETURN candidate;
END $$;

CREATE FUNCTION app.fn_remember_path(username TEXT, slug TEXT, collection_id UUID) RETURNS VOID
LANGUAGE sql AS $$
  INSERT INTO app.collection_path (username, slug, collection_id) VALUES (username, slug, collection_id)
  ON CONFLICT (username, slug) DO UPDATE SET collection_id = EXCLUDED.collection_id, created_at = now()
$$;

-- A new collection, a rename and a restore take a free slug; the old path is remembered.
CREATE FUNCTION app.fn_collection_path() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    NEW.slug := app.fn_free_slug(NEW.owner_id, NEW.name, NEW.id);
    RETURN NEW;
  END IF;
  IF NEW.name IS DISTINCT FROM OLD.name OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
     OR (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL) THEN
    NEW.slug := app.fn_free_slug(NEW.owner_id, NEW.name, NEW.id);
  END IF;
  IF NEW.slug IS DISTINCT FROM OLD.slug OR NEW.owner_id IS DISTINCT FROM OLD.owner_id THEN
    PERFORM app.fn_remember_path((SELECT username FROM usr.usr_user WHERE id = OLD.owner_id), OLD.slug, OLD.id);
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_app_collection_path BEFORE INSERT OR UPDATE OF name, owner_id, deleted_at ON app.collection
    FOR EACH ROW EXECUTE FUNCTION app.fn_collection_path();

-- A new username keeps the old paths of every collection the account owns.
CREATE FUNCTION app.fn_user_username_moved() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO app.collection_path (username, slug, collection_id)
  SELECT OLD.username, c.slug, c.id FROM app.collection c WHERE c.owner_id = NEW.id
  ON CONFLICT (username, slug) DO UPDATE SET collection_id = EXCLUDED.collection_id, created_at = now();
  RETURN NEW;
END $$;
CREATE TRIGGER trg_usr_user_username_moved AFTER UPDATE OF username ON usr.usr_user
    FOR EACH ROW WHEN (NEW.username IS DISTINCT FROM OLD.username) EXECUTE FUNCTION app.fn_user_username_moved();

-- One record of a collection; data is validated against the collection schema by the
-- backend. Up to 1 MB (domain.RecordMaxBytes); import refuses a larger record alone.
CREATE TABLE app.record (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- RESTRICT: a collection with records cannot be deleted by accident.
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE RESTRICT,
    data          JSONB       NOT NULL,
    version       INTEGER     NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_app_record_data_size CHECK (pg_column_size(data) <= 1048576)
);

-- The list sorts by creation or by last change; the quality check walks by id.
CREATE INDEX idx_app_record_collection_created ON app.record (collection_id, created_at DESC, id DESC);
CREATE INDEX idx_app_record_collection_updated ON app.record (collection_id, updated_at DESC, id DESC);
CREATE INDEX idx_app_record_collection_id ON app.record (collection_id, id);
-- Search is a substring match over the whole record. shortcut: the biggest write cost
-- of the table (every trigram of every value); narrow it to text fields if writes slow down.
CREATE INDEX idx_app_record_data_trgm ON app.record USING gin ((data::text) gin_trgm_ops);
-- Per-field filters use containment (data @> '{"field": value}').
CREATE INDEX idx_app_record_data_path ON app.record USING gin (data jsonb_path_ops);

CREATE TRIGGER trg_app_record_set_updated_at BEFORE UPDATE ON app.record
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- Earlier versions of each record: every change that replaces data saves the data it
-- replaces here first. No FK: versions outlive the move to the trash; the purge drops them.
-- shortcut: kept forever; prune to the last N per record when the table grows.
CREATE TABLE app.record_version (
    record_id UUID        NOT NULL,
    version   INTEGER     NOT NULL,
    data      JSONB       NOT NULL,
    -- When this version was written: the record's updated_at at that time.
    saved_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (record_id, version)
);

-- Deleted records wait here 30 days; restoring puts them back with the same id.
CREATE TABLE app.record_trash (
    id            UUID        PRIMARY KEY,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    data          JSONB       NOT NULL,
    version       INTEGER     NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    deleted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_by    UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL
);
CREATE INDEX idx_app_record_trash_collection ON app.record_trash (collection_id, deleted_at DESC);
-- The purge empties what passed the retention, across collections.
CREATE INDEX idx_app_record_trash_deleted ON app.record_trash (deleted_at);

-- Each schema a collection had, saved when a change replaces it (json: key order is field order).
CREATE TABLE app.collection_schema_version (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    version       INTEGER     NOT NULL,
    schema        JSON        NOT NULL,
    saved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    saved_by      UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    PRIMARY KEY (collection_id, version)
);

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
    -- The invalid records "delete the invalid records" removes, at most
    -- domain.QualityMaxInvalidIDs; past it, a new check finds the rest.
    invalid_ids    UUID[]      NOT NULL DEFAULT '{}',
    error          TEXT        NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ NULL,
    CONSTRAINT chk_app_collection_quality_status CHECK (status IN ('running', 'done', 'failed')),
    CONSTRAINT chk_app_collection_quality_invalid_ids CHECK (cardinality(invalid_ids) <= 10000)
);

-- A collection's dashboard: the owners' chosen widgets, shown to every member.
CREATE TABLE app.collection_dashboard (
    collection_id UUID        PRIMARY KEY REFERENCES app.collection (id) ON DELETE CASCADE,
    widgets       JSON        NOT NULL DEFAULT '[]',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by    UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL
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
    created_by    UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_file_collection ON app.file (collection_id);

-- Ids of deleted file rows whose stored bytes are still to be removed. The trigger
-- also catches rows a collection purge cascades away.
CREATE TABLE app.file_gone (
    id         UUID        PRIMARY KEY,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE FUNCTION app.fn_file_gone() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO app.file_gone (id) VALUES (OLD.id) ON CONFLICT DO NOTHING;
    RETURN OLD;
END $$;

CREATE TRIGGER trg_app_file_gone AFTER DELETE ON app.file
    FOR EACH ROW EXECUTE FUNCTION app.fn_file_gone();

-- Comments on a record: a note any member can leave, removable by its author or an owner.
CREATE TABLE app.record_comment (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id  UUID        NOT NULL REFERENCES app.record (id) ON DELETE CASCADE,
    user_id    UUID        NULL REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_app_record_comment_body_len CHECK (char_length(body) BETWEEN 1 AND 2000)
);
CREATE INDEX idx_app_record_comment_record ON app.record_comment (record_id, created_at);

-- An import batch already saved, by the key its client gave it: a retry after a lost
-- answer gets the first result back instead of saving the records twice. Kept a week.
CREATE TABLE app.import_batch (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    batch_key     TEXT        NOT NULL,
    created       BIGINT      NOT NULL DEFAULT 0,
    updated       BIGINT      NOT NULL DEFAULT 0,
    saved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, batch_key)
);
CREATE INDEX idx_app_import_batch_saved ON app.import_batch (saved_at);

-- Empties the trash: records and collections deleted more than retention ago
-- (domain.TrashRetention). The backend runs it every hour.
CREATE FUNCTION app.fn_purge_trash(retention INTERVAL DEFAULT INTERVAL '30 days')
RETURNS void
LANGUAGE sql
AS $$
  DELETE FROM app.record_version v USING app.record_trash t
   WHERE t.id = v.record_id AND t.deleted_at < now() - retention;
  DELETE FROM app.record_trash WHERE deleted_at < now() - retention;
  DELETE FROM app.record_version v USING app.record r, app.collection c
   WHERE v.record_id = r.id AND r.collection_id = c.id AND c.deleted_at < now() - retention;
  DELETE FROM app.record r USING app.collection c WHERE r.collection_id = c.id AND c.deleted_at < now() - retention;
  DELETE FROM app.collection WHERE deleted_at < now() - retention;
$$;
