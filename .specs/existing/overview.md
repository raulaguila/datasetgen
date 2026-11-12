# Existing overview

O que o código faz hoje.

1. `config.MustLoad` + logger + `infra.New` (Postgres).
2. `cmd/backend`: Fiber — health, `/v1/auth/*`, `/v1/me/*` (perfil, senha, sessões, preferências, notificações), `/v1/products` CRUD + Bearer/Require.
3. Auth: register → outbox → activate → login → refresh → logout; password reset + outbox.
4. `cmd/mailer`: poll `sys.outbox` (SKIP LOCKED), SMTP send, backoff / terminal fail.
5. Products: full CRUD on `app.product` with RBAC bits.

Repos: user, credential, session, token, attempt, role, outbox (enqueue+claim), rbac, product.

Testes unitários: aescrypt, password, domain, authorize, middleware, product validation.
