# Overview

Plataforma de coleções de dados com campos validados por JSON Schema: backend Go (Fiber, Postgres), mailer, CLI de operador e web Next.js (BFF). API `/v1` com Bearer, chaves de API, MCP e links públicos.

Fonte de verdade de runtime: `ARCHITECTURE.md`. Rotas: Swagger (`/swagger/index.html` fora de produção), gerado das anotações e conferido por `swagger_test.go`.

## Binários

- `cmd/backend`: API Fiber, jobs em segundo plano e automações.
- `cmd/mailer`: consome `sys.outbox` e envia por SMTP.
- `cmd/admin`: CLI de operador (`bootstrap`, `promote`, `disable`, `enable`, `mfa-reset`).
