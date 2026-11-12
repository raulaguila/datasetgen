# ADR-002 Schema usr

Status: aceito. Implementação: SQL nas migrations; Go usa as tabelas via repos (`user`, `credential`, `session`, `token`, `attempt`, `role`, `rbac` reader) nos fluxos de auth.

Identidade vive no schema `usr` (não `sys`/`entry`). Status é `CHECK` em `usr_user.status`; só `active` autentica. Credencial 1:1. Sessão guarda refresh rotacionado. Tokens SMTP separados da sessão. Login attempts append-only sem FK. RBAC: ver **ADR-004** (`(role_id, resource)` × `actions INT4` CRUD, catálogo `usr_resource`, `is_superuser` + sentinela `*`).

Fonte: `deploy/db/migrations`, `existing/infrastructure/schema.md`, `ARCHITECTURE.md`.
