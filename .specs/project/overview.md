# Overview

Serviço de identidade e autenticação em Go. API `/v1` com Bearer JWT. Inclui módulo exemplo `products` e outbox de e-mail.

## Estado

- **Postgres:** schemas `usr` (identidade), `sys` (outbox), `app` (product exemplo).
- **Backend:** Fiber + health + `/v1/auth/*` (register → activate → login → refresh → logout → password reset); grava `sys.outbox` nos fluxos de e-mail.
- **Authz:** use case Authorize (superuser / `*` / CRUD) wired; **ainda sem middleware** nas rotas de domínio.
- **Mailer:** bootstrap só — consumer de outbox / SMTP send pendente.
- **Admin CLI:** `main` vazio.

Fonte de verdade de runtime: `ARCHITECTURE.md` e `.specs/existing/overview.md`.

## Binários

- `cmd/backend`: API Fiber; composition (Health, Auth, Authorizer); graceful shutdown.
- `cmd/mailer`: mesma infra; bloqueia sem loop de outbox.
- `cmd/admin`: CLI de operador (`promote`, `disable`).

Compose: `dataset_backend`, `dataset_mailer`, `dataset_postgres`.
