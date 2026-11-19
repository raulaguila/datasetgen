# Postgres

Imagem oficial `postgres:17.2-alpine`, sem extensão de agendamento: a limpeza (`sys.fn_cleanup`, `app.fn_purge_trash`) roda de hora em hora no worker de jobs do backend. Volume `dataset_pg_volume`. Healthcheck `pg_isready`. Database default `dataset`.

Migrations: golang-migrate em `deploy/db/migrations`. Ver `existing/infrastructure/schema.md`.
