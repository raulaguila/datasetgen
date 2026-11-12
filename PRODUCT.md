# Product

<!-- keel:product-schema 1 -->

## Platform

`web-api` + `worker` (evidence: `cmd/backend` Fiber HTTP API; `cmd/mailer` SMTP outbox consumer), with `cmd/admin` for operator actions.

## Stack

- Language / runtime: Go (`go.mod` module `github.com/raulaguila/dataset`, Go 1.26.x)
- HTTP: Fiber v3
- Datastore: PostgreSQL 17 (schemas `usr`, `sys`, `app`; golang-migrate)
- Auth crypto: Ed25519 JWT (access/refresh), Argon2id passwords (PHC), SHA-256 for refresh `jti` and email tokens
- SMTP: configured through environment variables; email tokens sealed with AES-GCM (`pkg/aescrypt`); mailer consumes the Postgres outbox with fenced leases, retries and a restricted database role
- Deploy: Docker Compose (`dataset_backend`, `dataset_mailer`, `dataset_postgres`)

## Users

Unknown — confirm before designing to that constraint.

Hypothesis from purpose: applications and operators that need identity (register/login/session) and RBAC; not confirmed as named customers or first-party apps.

## Product Purpose

Provide identity and authentication for clients: account lifecycle (pending → activate → active), session-based JWT access/refresh with rotation and reuse detection, session management by the user, password reset, live RBAC (resource × CRUD), role/permission/member management, user administration, user preferences, in-app security notifications, and email delivery via Postgres outbox.

Operational success (not measured SLOs): password login requires an active account or an expired lock that can be cleared; a lock does not invalidate existing sessions or prevent refresh. Disabling an account revokes its sessions. Passwords and token plaintext are never stored; HTTP never echoes login `failure_kind`; authz does not trust JWT permission claims.

## Positioning

**Is:** a focused dataset/identity service with hexagonal layout, identity schema `usr`, system outbox `sys`, and a small example domain module `app.product` to exercise RBAC.

**Is not (v1):** multi-tenant SMTP catalog, product commerce platform, ABAC/instance-level authz, or a general dashboard — explicitly out of scope vs copying “hexago” extras without a decision.

## Operating Context

- Local/dev via Compose and `config/.env` (`SYS_ENVIRONMENT=development` allows `AUTH_MANUAL_TOKEN_DELIVERY`)
- Protected envs (anything except `development`, `dev`, `local` or `test`): CORS cannot be empty or `*`; manual token delivery forbidden; SMTP is required with TLS and real settings
- Tenancy: single-tenant identity store (no org/tenant tables in migrations)
- Regions / on-call / HA topology: Unknown — confirm before designing to that constraint.

## Scale & SLOs

Unknown — confirm before designing to that constraint.

(Config exposes rate-limit knobs and lockout windows; no measured QPS, latency targets, or availability SLOs in-repo.)

## Cost Posture

Unknown — confirm before designing to that constraint.

Known expensive-by-design paths (qualitative): Argon2id verify on login; authz Postgres view on each check; mailer/SMTP when enabled.

## Capabilities and Constraints

**Must**

- Hexagonal dependency direction (core ≠ Fiber/GORM)
- Password Argon2id only; refresh/`usr_token` stored as SHA-256
- Password login gated by account status and lock expiry; existing sessions remain usable during a login lock
- RBAC B+C (ADR-004): `usr_resource` catalog; actions CRUD bits; admin `is_superuser` and/or `resource='*'`; live authz via view
- HTTP role administration excludes changes to protected roles; full-access grants and management of users with protected roles go through `cmd/admin`. `roles:update` can change any unprotected role, including the caller's own
- Email side effects via `sys.outbox` (backend enqueue; mailer consume)

**Must not (v1)**

- Put permissions in access JWT
- Echo internal `failure_kind` on HTTP
- Multi-tenant SMTP / hexago dashboard copy without explicit decision
- Instance-level (UUID) authorization — type-level RBAC only

## Evidence on Hand

- `.specs/project/overview.md`, `constraints.md`, `glossary.md`
- `.specs/architecture/decisions/ADR-001` … `ADR-004` (hexagonal, identity schema, secrets, RBAC)
- `.specs/features/identity/*`, `mailer/outbox.md`, `products/requirements.md`
- `.specs/existing/overview.md`, `existing/infrastructure/schema.md`
- Code: `cmd/backend`, `internal/core/usecase/auth`, `internal/adapter/inbound/http/rest`, migrations `000001`–`000004`

## Product Principles

1. **Secrets stay hashed or wrapped** — never store password, refresh `jti`, or email token plaintext.
2. **Session is the logout unit** — access carries `sid`; refresh rotates and detects reuse.
3. **Authz is live and typed** — load effective permissions at request time; authorize resource types with CRUD bits (or superuser/`*`).
4. **Side effects are durable first** — email goes through outbox, then the mailer.
5. **Specs and SQL constrain the product** — CHECKs and ADRs beat ad-hoc feature creep; example `products` exists only to prove RBAC.
6. **Enforce account state** — pending and disabled accounts cannot log in; active locks block password login while preserving existing sessions. HTTP responses stay coarse for credential failures.

Operational safeguards include atomic login/reset coordination, serialized per-account email limits, absolute session expiry from initial login, and stale-worker rejection for outbox acknowledgements. Product prices sort numerically. Cursor pagination is available for chronological traversal without totals. JSON logs and private process diagnostics support operational measurements without introducing a cache for live authorization.
