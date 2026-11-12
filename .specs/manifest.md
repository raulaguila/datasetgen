# Manifest

Specs do dataset. Inventário e `existing/` descrevem o que corre. `ARCHITECTURE.md` / `PRODUCT.md` (Keel) são a visão de runtime/produto na raiz. Features e audits marcam gaps restantes.

Versão do sistema: `0.0.1` (`backend/config/version.txt`). Módulo: `github.com/raulaguila/dataset` (`backend/go.mod`). Caminhos de código nas specs (`internal/`, `config/`, `deploy/`, `tests/`) são relativos a `backend/`; o front fica em `frontend/`.

## Árvore

- `project/` — objetivo, restrições, glossário
- `inventory/` — módulos, endpoints, dependências
- `existing/` — o que já corre: módulos Go, infra, integrações
- `architecture/` — hexagonal, camadas, regras, ADRs; `permission-models.md` histórico
- `audits/` — aderência, implementação, gaps
- `features/` — identidade, products (exemplo), mailer/outbox
- `tasks/` — backlog

## Leitura

1. `../PRODUCT.md` e `../ARCHITECTURE.md` (Keel)
2. `project/overview.md` + `existing/overview.md`
3. `architecture/overview.md` + ADR-004
4. `audits/gaps.md` / `tasks/backlog.md` para o que falta
5. `features/identity/` para comportamento de conta
