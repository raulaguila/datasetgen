DROP FUNCTION IF EXISTS public.fn_slugify(TEXT, INTEGER);
DROP FUNCTION IF EXISTS public.fn_set_updated_at();
DROP SCHEMA IF EXISTS app CASCADE;
DROP SCHEMA IF EXISTS sys CASCADE;
DROP SCHEMA IF EXISTS usr CASCADE;
-- The roles are shared by every database of the cluster; they stay.
