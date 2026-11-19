# Schema

Fonte: `deploy/db/migrations`. Schema `usr` = identidade; `sys` = outbox, jobs, rate limit e auditoria; `app` = coleções, registros e integrações. `public` guarda extensões, `fn_set_updated_at` e `fn_slugify`. Clock de negócio na aplicação; o trigger só preenche `updated_at`. Volume local antigo: `make compose-remove`.

## Ordem

```text
000001_foundation    # citext/pg_trgm/unaccent, schemas usr/sys/app, fn_set_updated_at, fn_slugify, roles dataset_app/dataset_mailer
000002_identity      # tabelas usr (username), RBAC, view, papeis user/admin, preferências, notificações
000003_system        # sys.outbox, sys.job, sys.rate_limit, sys.audit_log, sys.fn_cleanup
000004_collections   # app.collection e endereços, membros, records, versões, lixeira, qualidade, views, arquivos, comentários
000005_integrations  # app.api_key (pessoais), app.share_link, webhooks, automações, MCP tools e log

Squash antes da primeira release: cada tabela nasce na forma final. Depois da release, só pares novos.
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

- Sem catálogo: `chk_usr_role_permission_resource` fixa os nomes (`*`, `audit`, `collections`, `roles`, `users`); grant de recurso desconhecido falha. Recurso novo amplia o CHECK numa migration nova. Sem tabela `usr_permission`.
- Uma linha `(role_id, resource)` com `actions INT4`.
- Bits CRUD: `create=1`, `read=2`, `update=4`, `delete=8` (4–30 livres). `CHECK (actions >= 0)`.
- `resource` é nome do CHECK ou sentinela `*`. Não é UUID de instância.
- `usr_role`: nome `^[a-z0-9_-]{2,40}$`; no máximo um `is_default`; `is_superuser` (bypass no Go).
- `usr_user_role`: DELETE de role com users falha (`RESTRICT`).
- `vw_user_effective_permissions`: `bit_or(actions)` por user+resource nos papéis `enabled`; não expande `*`. Superuser fora da view (`EXISTS` no Go).

Seed: papel `user` (default, `collections` = create, `is_superuser=false`). Papel `admin` com `is_superuser=true` e `resource='*'` / `actions=0x7FFFFFFF`. Sem rotas HTTP de papéis: `cmd/admin` promove ou rebaixa.

## Schema `sys`

- `sys.outbox`: eventos de e-mail (`purpose` activation|password_reset|automation; `status` pending|processing|sent|failed; `payload` JSONB). Backend INSERT; mailer consome com `SKIP LOCKED`. Índice parcial em `pending`.
- `sys.job`: fila de jobs do backend. `sys.rate_limit`: UNLOGGED, contadores por minuto.
- `sys.audit_log`: eventos de admin e de coleção; app role só INSERT (sem UPDATE/DELETE); sem limpeza.
- `sys.fn_cleanup()` e `app.fn_purge_trash()`: chamadas de hora em hora pelo worker do backend.

## Schema `app`

- `app.collection` (endereço `owner_id` + `slug`), `collection_member`, `collection_path` (endereços antigos), `record` (data JSONB, 1 MB), `record_version`, `record_trash`, `collection_quality` (`invalid_ids` até 10 000), views, dashboards, arquivos, comentários, `import_batch`.
- `app.api_key`: chaves pessoais (só hash); `app.share_link`: links públicos `form`|`view` com token selado.
