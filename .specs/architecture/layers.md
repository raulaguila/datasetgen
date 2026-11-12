# Layers

## Pretendido

1. **Inbound** — `internal/adapter/inbound/http/rest` (Fiber). DTOs e mappers no adapter. Handlers chamam input ports via composition/`Application`, nunca repositórios.
2. **Core** — `internal/core/domain`, `port/input`, `port/output`, `usecase`. Sem tags de transporte. Use case devolve entidade; REST DTO fica no inbound.
3. **Outbound** — Postgres implementa output ports. Models e mappers no adapter. Erros via `shared.WrapError` → `apperror`. SMTP no mailer.
4. **Runtime** — `internal/infra` recursos; `internal/composition` monta concretos; `cmd/*` só bootstrap.

## Atual

- Camadas 1–4: health, auth, conta, products CRUD; Bearer + `Require` nas rotas protegidas.
- Mailer: outbox claim (`SKIP LOCKED`) + SMTP send.
- Pendente: API roles/permissions.
