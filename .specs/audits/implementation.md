# Audit: implementation

| Área | Código | Banco / config |
|------|--------|----------------|
| Bootstrap | `MustLoad` + Postgres | ok |
| HTTP | Fiber listen; `/health`, `/v1/auth/*` | rate limit, log de requisição e timeout em `rest/middleware` |
| JWT | emissão + parse RSA | access/refresh |
| Argon2id | `pkg/password` | CHECK na credencial |
| Sessão / refresh reuse | use case auth | tabelas e CHECKs |
| Tokens SMTP | hash SHA-256 + outbox enqueue | `usr_token` |
| Mailer | outbox com lease + SMTP | token cifrado com `SMTP_MASTER_KEY` |
| Outbox / products | mailer + CRUD HTTP | `sys.outbox`, `app.product` |
| RBAC | Authorize + rbac reader + `Require` | B+C |
| Admin CLI | `promote`, `disable` | papel `admin` |
| Docker `BASE` | default `source` | `built.Dockerfile` / `source.Dockerfile`; serviço `migrate`; `USER nobody` |
| Testes | unitários + integração (`TEST_POSTGRES_URL`) | CI em `.github/workflows/ci.yml` |

Health use case usa `postgres.HealthChecker`.
