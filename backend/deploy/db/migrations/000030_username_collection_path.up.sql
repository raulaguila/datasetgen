-- Collections live at /<owner username>/<slug>, as repositories do on GitHub.
-- A user gets a username from the email, a collection a slug from its name; both are
-- made here so every insert path (API, admin CLI, SSO) gets one. Renames, transfers and
-- username changes keep the old path in app.collection_path, so old links still open.

CREATE EXTENSION IF NOT EXISTS unaccent;

-- Lowercase letters, digits and inner single hyphens, 1-39 characters (GitHub's rule).
-- Reserved: the frontend's top-level routes and a few names kept for later.
-- domain.ReservedUsername holds the same list; both change together.
CREATE FUNCTION usr.fn_username_reserved(name TEXT) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
  SELECT name = ANY (ARRAY[
    'about', 'account', 'activate', 'admin', 'api', 'api-access', 'app', 'assets', 'c', 'collections',
    'dashboard', 'dataset', 'docs', 'explore', 'f', 'fonts', 'forgot-password', 'help', 'home', 'login',
    'logout', 'me', 'new', 'notifications', 'oauth', 'profile', 'public', 'r', 'records', 'register',
    'reset-password', 'root', 's', 'search', 'settings', 'signin', 'signup', 'static', 'system', 'u',
    'user', 'users', 'www'])
$$;

-- The part of a text that fits a username or slug: accents dropped, other runs of
-- characters turned into one hyphen, cut to max characters.
CREATE FUNCTION app.fn_slugify(input TEXT, max INTEGER) RETURNS TEXT
LANGUAGE sql STABLE AS $$
  SELECT trim(BOTH '-' FROM left(trim(BOTH '-' FROM regexp_replace(lower(public.unaccent(input)), '[^a-z0-9]+', '-', 'g')), max))
$$;

-- The first free username from base: base, base-2, base-3...
CREATE FUNCTION usr.fn_free_username(base TEXT, except_id UUID) RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE
  root TEXT := coalesce(nullif(app.fn_slugify(base, 34), ''), 'user');
  candidate TEXT := root;
  n INTEGER := 1;
BEGIN
  LOOP
    EXIT WHEN NOT usr.fn_username_reserved(candidate)
      AND NOT EXISTS (SELECT 1 FROM usr.usr_user WHERE username = candidate AND id IS DISTINCT FROM except_id);
    n := n + 1;
    candidate := root || '-' || n;
  END LOOP;
  RETURN candidate;
END $$;

-- The first free slug from name under owner, skipping the collection itself.
CREATE FUNCTION app.fn_free_slug(owner UUID, name TEXT, except_id UUID) RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE
  root TEXT := coalesce(nullif(app.fn_slugify(name, 90), ''), 'collection');
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

ALTER TABLE usr.usr_user ADD COLUMN username TEXT;

-- Oldest accounts keep the plain name when two emails share it.
DO $$
DECLARE u RECORD;
BEGIN
  FOR u IN SELECT id, email FROM usr.usr_user ORDER BY created_at, id LOOP
    UPDATE usr.usr_user SET username = usr.fn_free_username(split_part(u.email::text, '@', 1), u.id) WHERE id = u.id;
  END LOOP;
END $$;

ALTER TABLE usr.usr_user
  ALTER COLUMN username SET NOT NULL,
  ADD CONSTRAINT uq_usr_user_username UNIQUE (username),
  ADD CONSTRAINT chk_usr_user_username CHECK (char_length(username) <= 39 AND
    username ~ '^[a-z0-9](?:[a-z0-9]|-(?=[a-z0-9])){0,38}$' AND NOT usr.fn_username_reserved(username));

-- A new account without a username gets one from its email.
CREATE FUNCTION usr.fn_user_username() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.username IS NULL OR NEW.username = '' THEN
    NEW.username := usr.fn_free_username(split_part(NEW.email::text, '@', 1), NEW.id);
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_usr_user_username BEFORE INSERT ON usr.usr_user
  FOR EACH ROW EXECUTE FUNCTION usr.fn_user_username();

-- owner_id is the account in the URL. It stays an owner member (the service refuses
-- removing or demoting it) until a transfer hands the collection to another owner.
ALTER TABLE app.collection
  ADD COLUMN owner_id UUID REFERENCES usr.usr_user (id),
  ADD COLUMN slug TEXT;

-- Existing collections: the oldest owner member, else the oldest member, else the
-- oldest platform admin (a collection nobody belongs to).
UPDATE app.collection c SET owner_id = coalesce(
  (SELECT m.user_id FROM app.collection_member m WHERE m.collection_id = c.id
    ORDER BY m.role = 'owner' DESC, m.created_at, m.user_id LIMIT 1),
  (SELECT ur.user_id FROM usr.usr_user_role ur JOIN usr.usr_role ro ON ro.id = ur.role_id
    JOIN usr.usr_user u ON u.id = ur.user_id
    WHERE ro.is_superuser OR EXISTS (SELECT 1 FROM usr.usr_role_permission p WHERE p.role_id = ro.id AND p.resource = '*')
    ORDER BY u.created_at, u.id LIMIT 1),
  (SELECT u.id FROM usr.usr_user u ORDER BY u.created_at, u.id LIMIT 1));

DO $$
DECLARE c RECORD;
BEGIN
  FOR c IN SELECT id, owner_id, name FROM app.collection ORDER BY deleted_at NULLS FIRST, created_at, id LOOP
    UPDATE app.collection SET slug = app.fn_free_slug(c.owner_id, c.name, c.id) WHERE id = c.id;
  END LOOP;
END $$;

ALTER TABLE app.collection
  ALTER COLUMN owner_id SET NOT NULL,
  ALTER COLUMN slug SET NOT NULL,
  ADD CONSTRAINT chk_app_collection_slug CHECK (char_length(slug) <= 100 AND slug ~ '^[a-z0-9](?:[a-z0-9]|-(?=[a-z0-9])){0,99}$');

-- A deleted collection gives its path up; a restore picks a free one (trigger below).
CREATE UNIQUE INDEX uq_app_collection_owner_slug ON app.collection (owner_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_app_collection_owner ON app.collection (owner_id);

-- Old paths that still lead somewhere. The newest move of a path wins.
CREATE TABLE app.collection_path (
    username      TEXT        NOT NULL,
    slug          TEXT        NOT NULL,
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (username, slug)
);
CREATE INDEX idx_app_collection_path_collection ON app.collection_path (collection_id);

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
CREATE FUNCTION usr.fn_user_username_moved() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO app.collection_path (username, slug, collection_id)
  SELECT OLD.username, c.slug, c.id FROM app.collection c WHERE c.owner_id = NEW.id
  ON CONFLICT (username, slug) DO UPDATE SET collection_id = EXCLUDED.collection_id, created_at = now();
  RETURN NEW;
END $$;
CREATE TRIGGER trg_usr_user_username_moved AFTER UPDATE OF username ON usr.usr_user
  FOR EACH ROW WHEN (NEW.username IS DISTINCT FROM OLD.username) EXECUTE FUNCTION usr.fn_user_username_moved();
