-- Extensions, schemas, shared functions and the roles the processes use.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE SCHEMA usr;  -- identity: accounts, sessions, roles, notifications
CREATE SCHEMA sys;  -- machinery: email outbox, jobs, rate limits, audit log
CREATE SCHEMA app;  -- the product: collections, records and their integrations

CREATE FUNCTION public.fn_set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

-- The part of a text that fits a username or slug: accents dropped, other runs of
-- characters turned into one hyphen, cut to max characters.
CREATE FUNCTION public.fn_slugify(input TEXT, max INTEGER) RETURNS TEXT
LANGUAGE sql STABLE AS $$
  SELECT trim(BOTH '-' FROM left(trim(BOTH '-' FROM regexp_replace(lower(public.unaccent(input)), '[^a-z0-9]+', '-', 'g')), max))
$$;

-- Only migrations keep the owner. `make migrate-up` gives these roles their
-- logins and the passwords from config/.env.
DO $$
DECLARE
  role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['dataset_app', 'dataset_mailer'] LOOP
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = role_name) THEN
      EXECUTE format('CREATE ROLE %I NOLOGIN', role_name);
    END IF;
  END LOOP;
END;
$$;

GRANT USAGE ON SCHEMA usr, sys, app TO dataset_app;
GRANT USAGE ON SCHEMA sys TO dataset_mailer;
ALTER DEFAULT PRIVILEGES IN SCHEMA usr, sys, app
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO dataset_app;
