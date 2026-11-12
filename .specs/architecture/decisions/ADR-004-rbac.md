# ADR-004 RBAC B+C, CRUD, authz ao vivo

Status: aceito. Implementação: SQL em `deploy/db/migrations` (000002 e catálogo; `products` em 000004); Go: use case `Authorize` + repo RBAC wired em `composition` — **ainda sem middleware HTTP** nas rotas de domínio.

## Contexto

Identidade precisa de papéis e permissões sem catálogo N:N de códigos compostos (`users:read`). Comparação em `permission-models.md` (histórico).

## Decisão

1. **Modelo B+C** — fonte da verdade: `(role_id, resource)` + `actions INT4`. Catálogo `usr.usr_resource` lista nomes de resource. Sem `usr_permission` / códigos A. Máscara global única na role rejeitada.
2. **Actions = CRUD** (vocabulário global no core):
   - `1 << 0` create = 1
   - `1 << 1` read = 2
   - `1 << 2` update = 4
   - `1 << 3` delete = 8
   - Bits 4–30 livres; não reutilizar índice. Check: `have & need == need`.
3. **Admin** — `usr_role.is_superuser` **ou** effective `resource='*'`. Seed do `admin`: `is_superuser=true` e linha `('*', 0x7FFFFFFF)`. A view não expande `*`; a aplicação trata superuser e sentinela.
4. **Catálogo** — `usr_resource` é a fonte dos nomes para a API (list resources / grants). `*` não entra no catálogo; a coluna gerada `catalog_code` tem FK para ele, então o banco recusa grant de recurso desconhecido. `GET /v1/permissions/resources` lista o catálogo. Grants granulares por papel via `/v1/roles`, nunca por pessoa; seed inicial do admin só com `*`. Papel protegido (`is_superuser` ou grant `*`) não aparece em `GET /v1/roles` e é somente leitura na API: acesso total muda só por `cmd/admin`, e a API não concede `*`.
5. **Authz ao vivo** — access JWT carrega identidade/`sid`, **não** o mapa de permissões. Middleware/core carrega effective permissions (view) a cada autorização; mudança de roles vale na hora.
6. **Escopo** — RBAC por **tipo** de resource (`users`, `roles`, `products`), não por UUID de instância.

## Consequências

- API pode expor `{ "users": ["create", "read"] }` expandindo bits no adapter.
- Resource novo: INSERT em `usr_resource` e constante em `domain`; admin com `*` / superuser já cobre; roles não-admin precisam de UPDATE no `actions`.
- `permission-models.md` fica supersedido por este ADR.
- Gap atual: ligar Authorizer às rotas (middleware).

Fonte: `deploy/db/migrations`, `existing/infrastructure/schema.md`, `features/identity/`, `ARCHITECTURE.md`.
