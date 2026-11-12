# Postgres

Imagem custom `deploy/docker/postgres/Dockerfile` (Postgres 17 Alpine + `pg_cron`). Volume `dataset_pg_volume`. Healthcheck `pg_isready`. Database default `dataset`.

Migrations: golang-migrate em `deploy/db/migrations`. Ver `existing/infrastructure/schema.md`.
