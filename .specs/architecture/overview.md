# Architecture overview

Hexagonal. O core não importa adapters, Fiber, GORM nem SQL concreto. Core define contratos; adapters traduzem o mundo externo.

## Hoje

```text
cmd/{backend,mailer,admin}
config
internal/infra
internal/composition
internal/core/{domain,port,usecase}
internal/adapter/inbound/http/rest
internal/adapter/outbound/{persistence/postgres,smtp,tokens}
pkg/{logger,apperror,aescrypt,smtpsecurity,pointer,password,tokenhash,tokens}
```

`cmd/backend`: Fiber listen + auth/health. `cmd/mailer`: infra sem consumer. RBAC: ADR-004 (Authorize wired; middleware HTTP pendente). Outbox: `sys.outbox` (API escreve). Detalhe: `ARCHITECTURE.md` e `layers.md`.
