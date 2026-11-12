# Docker

`deploy/docker/compose.yml` inclui `services.compose.yml`. Serviços `backend` e `mailer` dependem de `postgres` healthy. Dockerfiles `bin` e `source` em `deploy/docker/backend` e `deploy/docker/mailer`.

Makefile default `BASE=bin`, mas os Dockerfiles chamam-se `built.Dockerfile` e `source.Dockerfile` — gap em `audits/implementation.md`.
