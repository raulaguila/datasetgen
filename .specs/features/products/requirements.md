# Products — requirements

> Removido antes da primeira release; histórico no git (commit `eab7529`).

Módulo exemplo no schema `app` para exercitar RBAC (`resource=products`, actions CRUD). Não é catálogo comercial completo.

- Campos: `name` (1–120), `price` (>= 0), `enabled` (default true).
- CRUD completo via API quando o caller tiver os bits correspondentes (ou superuser/`*`).
- Sem multi-tenant, estoque ou variantes na v1.

Fonte (histórico): `git show eab7529:backend/deploy/db/migrations/000004_products.up.sql`, ADR-004.
