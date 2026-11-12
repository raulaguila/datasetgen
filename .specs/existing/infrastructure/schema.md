# Schema

Fonte: `deploy/db/migrations`. Schema `usr` = identidade; `sys` = outbox, jobs e rate limit; `app` = products. `public` guarda extensões e `fn_set_updated_at`. Clock de negócio na aplicação; o trigger só preenche `updated_at`. Volume local antigo: `make compose-remove`.

## Ordem

```text
000001_foundation  # citext, schemas usr/sys/app, fn_set_updated_at, role dataset_app
000002_identity    # tabelas usr, RBAC, view, usr_resource, papeis user/admin, notificações, auditoria
000003_system      # sys.outbox, sys.job, sys.rate_limit, sys.fn_cleanup, pg_cron
000004_products    # app.product (com version) e recurso products no catalogo
```

## Status (`usr_user.status`)

`CHECK` na coluna; os códigos são os de `domain.UserStatus*`. Só `active` autentica.

- `pending` — cadastro sem prova de caixa; sem senha
- `active` — ativado
- `expired` — token de ativação venceu sem uso; sem senha
- `locked` — lockout (`locked_until` na credencial)
- `disabled` — operador

## Tabelas `usr`

- `usr_user`: e-mail `CITEXT` único; `status` com `CHECK`; nome 2–80 chars.
- `usr_credential`: 1:1; Argon2id; NULL até activate; `failed_login_attempts` >= 0.
- `usr_session`: logout (`sid`) + refresh (SHA-256 do `jti`); rotação via `previous_refresh_token_hash`; revogação `user_logout` | `token_reuse` | `password_changed` | `expired` | `account_disabled` com `revoked_at`. IP/UA snapshot, não device tracking.
- `usr_token`: `activation` | `password_reset`; SHA-256; um ativo por user+purpose; 5ª recusa marca `used_at`.
- `usr_preference`: 1:1 com usuário, `theme` e `language`; sem linha valem os defaults.
- `usr_notification`: avisos de segurança por usuário; lidas saem após a retenção de `fn_cleanup`, não lidas após o triplo.
- `usr_login_attempt`: append-only, inclusive e-mail inexistente; sem FK de user.

## RBAC (ADR-004)

- Catálogo `usr_resource` (códigos seed: `users`, `roles`, `audit`, `products`; não há `permissions`: ler o catálogo faz parte de `roles:read`). Sem tabela `usr_permission`.
- Uma linha `(role_id, resource)` com `actions INT4`. `catalog_code` (`NULLIF(resource, '*')`, coluna gerada) tem FK para o catálogo: grant de recurso desconhecido falha e recurso com grant não sai do catálogo; `*` passa sem linha. `GET /v1/permissions/resources` lista o catálogo.
- Bits CRUD: `create=1`, `read=2`, `update=4`, `delete=8` (4–30 livres). `CHECK (actions >= 0)`.
- `resource` é tipo do catálogo ou sentinela `*`. Não é UUID de instância.
- `usr_role`: nome `^[a-z0-9_-]{2,40}$`; no máximo um `is_default`; `is_superuser` (bypass no Go).
- `usr_user_role`: DELETE de role com users falha (`RESTRICT`).
- `vw_user_effective_permissions`: `bit_or(actions)` por user+resource nos papéis `enabled`; não expande `*`. Superuser fora da view (`EXISTS` no Go).

Seed: papel `user` (default, zero linhas, `is_superuser=false`). Papel `admin` com `is_superuser=true` e `resource='*'` / `actions=0x7FFFFFFF`. Grants granulares via API.

## Schema `sys`

- `sys.outbox`: eventos de e-mail (`purpose` activation|password_reset; `status` pending|processing|sent|failed; `payload` JSONB). Backend INSERT; mailer consome com `SKIP LOCKED`. Índice parcial em `pending`.

## Schema `app`

- `app.product`: exemplo CRUD (`name`, `price`, `enabled`) para exercitar resource `products`.
