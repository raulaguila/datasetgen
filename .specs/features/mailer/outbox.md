# Mailer — outbox

Backend registra eventos; `cmd/mailer` consome e envia SMTP.

## Tabela

`sys.outbox` (migration 000003):

- `purpose`: `activation` | `password_reset`
- `payload`: JSONB (destinatário, refs de token, locale, …) — sem plaintext de senha
- `status`: `pending` → `processing` → `sent` | `failed`
- `attempts`, `available_at` (backoff), `last_error`

## Fluxo

1. Register / password-reset (API): em transação com o domínio, INSERT `pending`.
2. Mailer: `SELECT … FOR UPDATE SKIP LOCKED` em `pending` com `available_at <= now()`.
3. Marca `processing`, envia SMTP, marca `sent` ou `failed` (+ incrementa `attempts`, agenda `available_at`).

## Schemas

- `sys` — infraestrutura (outbox); não é resource RBAC na v1
- `app` — domínio exemplo (`product`), separado do outbox

## Fora de escopo

Fila Redis dedicada.
