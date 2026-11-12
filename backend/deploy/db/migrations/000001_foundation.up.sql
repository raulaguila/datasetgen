-- Extensions, schemas, the updated_at trigger and the roles the processes use.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SCHEMA usr;  -- identity
CREATE SCHEMA sys;  -- email outbox and cleanup
CREATE SCHEMA app;  -- example CRUD

CREATE FUNCTION public.fn_set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
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
