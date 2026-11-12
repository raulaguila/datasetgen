# Audit: architecture

Aderência ao hexagonal declarado. Ver também `ARCHITECTURE.md` (Keel).

| Regra | Estado |
|-------|--------|
| Core sem adapters | Sim — `internal/core` não importa adapters |
| Inbound REST / Fiber | Sim — health + `/v1/auth/*` |
| Outbound Postgres | Sim — repos auth + RBAC reader + outbox enqueue |
| Composition root | Sim — Health, Auth, Authorizer |
| `cmd` fino | Sim — bootstrap + listen/shutdown |
| `pkg` sem domínio de produto | Sim |
| Dependências apontam para dentro | Sim no grafo principal |

Conclusão: hexagonal operacional para auth. Gaps: middleware authz, mailer consumer, products/roles API.
