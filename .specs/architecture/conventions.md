# Conventions

Regras do SQL, da config e de `pkg`. Aplicar ao implementar HTTP e domínio. RBAC: ADR-004.

## Segredos

- Plaintext nunca gravado: senha Argon2id; refresh `jti` e tokens SMTP SHA-256 (`BYTEA`).

## Identidade

- Só `status = active` autoriza login.
- HTTP não ecoa `failure_kind` (`invalid_credentials` / `account_locked` + `apperror`).
- `usr_login_attempt` também para e-mail inexistente.
- Reuse de refresh revoga (`token_reuse`).
- Papel `is_default` no activate; um default no banco.
- Authz: `is_superuser` **ou** effective `resource='*'`; senão `Allows(have, need)` por resource. View não expande `*`.
- Access JWT sem mapa de permissões; load ao vivo (view).
- Actions CRUD: `create`, `read`, `update`, `delete`. Resources dinâmicos via `usr_resource`.
- Grants granulares (incl. mutação em `users`) via API com superuser/`*`.

## E-mail

- Register/reset gravam `sys.outbox` (`pending`); `cmd/mailer` consome e envia SMTP.

## Tempo e erros

- Erros via `pkg/apperror`; adapters mapeiam protocolo.
- `updated_at` é trigger. Expiração, lockout e TTL são da aplicação.

## Migrations

- `deploy/db/migrations`, sequenciais, golang-migrate.
- Não misturar fundação, tabelas e seed no mesmo arquivo novo.
- Não renomear histórico aplicado sem plano. Local: `make migrate-reset` se o volume puder apagar.
