DROP OWNED BY dataset_app, dataset_mailer;
DROP ROLE dataset_app, dataset_mailer;

DROP FUNCTION public.fn_set_updated_at();
DROP SCHEMA app, sys, usr;
DROP EXTENSION pg_trgm, citext;
