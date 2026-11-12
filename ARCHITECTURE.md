# Architecture

<!-- keel:architecture-schema 1 -->

## Overview

The Go service lives in `backend/` (its own `go.mod`, `Makefile`, config and Docker files) and the Next.js app in `frontend/`; paths such as `cmd/backend` or `config/.env` below are relative to `backend/`.

**Confirmed.** Dataset runs as two long-lived processes plus Postgres: `cmd/backend` serves Fiber HTTP (health, auth, sessions, preferences, notifications, permissions, roles, users, collections, records and products CRUD); `cmd/mailer` claims `sys.outbox` rows under a 10-minute lease (`SKIP LOCKED`), opens the sealed token and sends SMTP. `cmd/admin` handles operator actions. `frontend/` is a Next.js BFF: it keeps the tokens in a signed httpOnly cookie, forwards API calls with the bearer and sends the client IP as `X-Real-IP`, which the backend accepts only from the web container's fixed address (`API_TRUSTED_PROXIES`). Composition wires use cases to Postgres. Product constraints live in `PRODUCT.md`; surface briefs in `.keel/surfaces/`.

## Boundaries

**Confirmed dependency direction (as coded):**

```text
cmd → composition / infra / adapter/inbound / config / pkg
infra → adapter/outbound/persistence/postgres (opens the connection)
composition → core/usecase + adapter/outbound
adapter/inbound → port/input (+ pkg/apperror)
adapter/outbound → port/output + domain
core/usecase → domain + policy + ports + pkg/{apperror,password,tokenhash,validate,clientip}
core/domain → stdlib only
```

| Boundary | Owns writes | Notes |
|----------|-------------|--------|
| `internal/core` | — | Domain + ports + use cases; no Fiber/GORM/config |
| `adapter/inbound/http/rest` | — | DTOs, route map, `apperror`→HTTP |
| `adapter/outbound/persistence/postgres` | `usr.*`, `sys.outbox`, `app.product`, `app.collection`, `app.collection_member`, `app.record` | GORM/`Exec`; `Store` = TxManager + UnitOfWork |
| `frontend/` | session cookie only | Next.js; never reaches Postgres, holds no signing key |
| `cmd/mailer` | outbox claim → SMTP | Poll 2s, batch 10; terminal fail after 14 attempts or smtp_disabled |

**Sacred (product + ADR):** JWT must not carry permission claims; authz loads live from the DB on each check. Plaintext passwords / refresh JTIs / email tokens must not be stored.

**Authorization (confirmed):** Authorizer is applied on `/v1/products`, `/v1/permissions`, `/v1/roles`, `/v1/users` and `/v1/audit`; own-account routes live under `/v1/me`, use Bearer and queries scoped to the authenticated user; `/v1/auth` keeps only the token and sign-up flows. Roles are managed at runtime under `/v1/roles`. Protected roles (`is_superuser` or a `*` grant) cannot be changed there and `GET /v1/roles` does not list them: full access changes only through `cmd/admin`, and the API never grants `*`. Holding `roles:update` lets someone change any unprotected role, including their own. Users with a protected role get the same treatment under `/v1/users`: not listed, 403 by id, changed only through `cmd/admin`. Nobody disables their own account. Enable puts an account back to `active`, or `pending` if it never set a password; sessions do not come back.

**Collection membership (confirmed):** collections and records are not governed by RBAC but by `app.collection_member` (fixed roles: `owner` changes fields, settings and members; `editor` writes records; `viewer` reads). RBAC keeps only `collections:create`. `/v1/collections` and `/v1/records` run behind Bearer and the use cases call `policy.CollectionRole` per request: a non-member gets 404 (existence does not leak), a member with too small a role 403. A platform admin (enabled `is_superuser` role or a `*` grant) sees every collection as owner. The creator becomes owner in the create transaction; a collection keeps at least one owner (member changes take a per-collection advisory lock). `POST /v1/collections/{id}/members` adds an account by email or, when none exists, invites it (`auth.Invite`, pending account) and adds it at once; the invite and the membership are two transactions (marked `shortcut:`). Collection names are no longer unique across the instance, so a hidden collection neither blocks nor reveals a name.

**Collection monitoring (confirmed):** `GET /v1/collections/{id}/audit` (owners) is the audit log filtered to the collection: entries whose target is the collection (its changes and member changes) or whose `details.collection_id` names it (record create, update and delete); `idx_usr_audit_log_collection` serves the second. `GET /v1/collections/{id}/quality` (editors and owners) walks the records by id (`idx_app_record_collection_key`) in batches of 500 and checks each against the current schema with `schema.Check`, returning the count and the first 100 invalid records with their issues. It runs in the request and stops at 50 000 records (marked `shortcut:`).

**Collection overview, icon and leave (confirmed):** `GET /v1/collections/{id}/stats?tz=` (any member) returns the record and member counts, the last record update, records created per day over the last 30 days in the caller's time zone (an IANA name, default UTC) and the top contributors by `record.create`/`record.update` audit entries in the same window. `POST /v1/collections/{id}/leave` removes the caller's own membership and is refused (409) for the last owner. `app.collection.icon` holds a slug from a fixed allowlist (`domain.ValidCollectionIcon`, mirrored by `frontend/src/lib/collection-icons.ts`); empty shows the initials. **Record tools (confirmed):** `GET /v1/records/export?collection_id=&format=jsonl|csv&valid=` (viewers) writes every record (up to 100 000, built in memory, marked `shortcut:`); CSV columns follow the schema's property order (`schema.PropertyOrder`) and every cell goes through `mapper.CSVCell`. `POST /v1/records/import` (editors) takes one batch of up to 1000 records under the 100 KB body limit, which stays as it is: the browser splits a JSON Lines or JSON array file into batches, checks them all with `dry_run` first, then sends them; batches commit one by one. `GET /v1/records?filter={...}` matches exact top-level values with `data @> filter` on `idx_app_record_data_path` (GIN `jsonb_path_ops`). Every change that replaces a record's data (edit, restore, bulk fix) first saves the old data in `app.record_version` (kept forever, marked `shortcut:`); `GET /v1/records/{id}/versions` and `POST /v1/records/{id}/restore` read and restore them, and a restore must match the current schema. `POST /v1/collections/{id}/fix` (editors) fills, renames or removes one top-level field in every record without checking the schema. The quality check is a background job (`collection.quality`) storing its result in `app.collection_quality`: `POST /v1/collections/{id}/quality` starts it unless one started less than 15 minutes ago is running, `GET` reads it, and a result of an older collection version is stale; `POST /v1/collections/{id}/delete-invalid` deletes the records that result found invalid, except those changed after it started. `POST /v1/collections/{id}/duplicate` needs the right to create collections and read access to the source. Adding a member notifies them (`collection_added`).

`GET /v1/collections?details=true` adds `member_count`, `field_count` and `last_activity` (newest change to the collection or its records); only the collections screen asks for them, which groups up to 100 collections in the browser into favorites, the caller's own (owner), shared with them and, for a platform admin, the others. Pinned collections and keyboard shortcuts (`g`+key to a section of the selected collection, `?` for the list) live only in the browser: pins in the UI store (localStorage).

## Data

**Confirmed stores:**

| Store | Role | Consistency notes |
|-------|------|-------------------|
| PostgreSQL schemas `usr`, `sys`, `app` | Source of truth | Single DB; migrations 000001–000007 |

**`usr` ownership (backend auth and account use cases):** users, credentials, sessions, email tokens, login attempts, role assignment, preferences, notifications; RBAC reader uses `vw_user_effective_permissions` + `is_superuser`.

**`sys.outbox`:** backend INSERT on register / password-reset; mailer claims `pending` → `processing` (lease in `available_at`, ownership in `claim_id`; a crashed claim returns and stale confirmations are rejected) → `sent`/`failed`. Tokens stay AES-GCM sealed with `SMTP_MASTER_KEY` and leave the row when done. `sys.fn_cleanup()` (daily via pg_cron) drops history older than 30 days.

**`app.product`:** owned by backend product use case (CRUD); kept as the template `make module` copies.

**`app.collection` / `app.record`:** a collection holds a JSON Schema in a `json` column, not `jsonb`: jsonb sorts object keys by length, and the key order of `properties` is the field order of the generated form. A record's `data` is `jsonb` (≤ 256 KB), checked by `pkg/schema` inside the write transaction after reading the schema `FOR SHARE`, so a concurrent schema change waits. A schema change does not revalidate stored records (marked `shortcut:`). Deleting a collection with records fails on the foreign key (`RESOURCE_IN_USE`). The record list returns a preview (top-level plain values, strings cut to 120 characters); `GET /v1/records/{id}` returns the full data. Access to both follows collection membership (see Authorization). In the browser, `frontend/src/lib/schema-validator.ts` validates with `@cfworker/json-schema`: ajv compiles with `new Function`, which the production CSP blocks.

**Database roles:** migrations run as the owner (`POSTGRES_USER`). Backend and `cmd/admin` connect as `dataset_app` (`POSTGRES_APP_USER`), which reads and writes rows in `usr`, `sys` and `app` and cannot change the schema. The mailer uses `dataset_mailer` (`POSTGRES_MAILER_USER`): SELECT on outbox and UPDATE on delivery columns only. It cannot access identity or products, insert messages, delete them, or change their purpose. Migrations create the roles and grants. After successful migration, `make migrate-up` enables both logins and sets their passwords from `config/.env` in a single transaction; secrets are not stored in versioned SQL.

**Tenancy:** single identity database; no org/tenant tables. **Inferred** as single-tenant until PRODUCT users say otherwise.

**Consistency:** auth multi-step writes use `WithinTx` (register, activate, reset confirm). `RecordFailure` counts in SQL. After hashing outside the transaction, login updates the credential conditionally and inserts its session in one short transaction. The credential row lock serializes this with password reset, which changes the password before revoking sessions. Email cooldown/cap checks and issuance share a transaction lock per user and purpose. Initial sessions and refresh both honor the absolute lifetime.

### Query rules

Every query reads only what its caller uses. A hot path pays for each extra column and each extra row in every request.

- **Name the columns.** No `SELECT *` and no GORM `Find`/`Take` into a full row struct when the caller reads only part of it. A narrower need gets its own method that names its columns (`Sessions().FindLive` for Bearer, next to `FindByID` for refresh). Secrets such as hashes and tokens leave the database only on the path that compares them.
- **Expensive values stay opt-in.** Aggregates that visit many rows (`member_count` on roles) are computed only for the screen that shows them (`FindByID(ctx, id, members)`). `ForUser`, permission checks and batch loops never pay for them.
- **One round trip where one statement can answer.** Several counts over one table use `count(*) FILTER (...)` in one query (`CountRecentFailures`). This matters most inside a transaction that holds locks.
- **Check the cheap thing before the expensive one.** Refuse on a count before loading the rows (audit CSV export).
- **Filters must match the indexes.** A partial index is used only when the query repeats its predicate literally (`shared.CountedFailure`).
- **Read raw SQL with `Find`, `Take` or `Row().Scan`, never `Raw(...).Scan`.** GORM's `Scan` swaps in a recorder logger that formats the full SQL with regular expressions on every call, even with logging off.
- **Request checks share one query.** Routes behind `Require` read the session and the permission bits for that one resource together (`rbac.Access`); the token group middleware (`Token`) only verifies the JWT. Every route in those groups must run `Require` (`TestEveryTokenRouteChecksTheSession`).
- **Bounded caches only.** GORM `PrepareStmt` stays off: its cache has no size limit, and every `IN` list length is a new statement. pgx already caches 512 statements per connection.
- **Every list has a limit.** OFFSET pages and leading-wildcard `ILIKE` are accepted shortcuts, marked `shortcut:` with the size where they stop being enough.

## Contracts

**HTTP (confirmed):** JSON; base path `/v1` for API; health stays at `/health`, outside the rate limit.

| Method | Path |
|--------|------|
| GET | `/health` |
| POST | `/v1/auth/register`, `/v1/auth/activate`, `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout` |
| POST | `/v1/auth/password-reset/request`, `/v1/auth/password-reset/confirm` |
| GET/PATCH | `/v1/me` (Bearer; PATCH renames) |
| POST | `/v1/me/password` (Bearer; current password required, ends the other sessions) |
| GET | `/v1/me/sessions` (Bearer) |
| DELETE | `/v1/me/sessions/others`, `/v1/me/sessions/:id` (Bearer) |
| GET/PUT | `/v1/me/preferences` (Bearer) |
| GET | `/v1/me/notifications`, `/v1/me/notifications/count` (Bearer) |
| PUT | `/v1/me/notifications/read`, body `{"ids": [...]}` (1 to 100, all or none) or no body for every one (Bearer) |
| GET | `/v1/permissions/resources` (`roles:read`); each resource lists the `actions` its routes check, and `PUT /v1/roles/:id/permissions` refuses any other (audit is read-only, users have no delete) |
| GET/POST | `/v1/roles`, `search`, `sort` (`name`, `created_at`), `order` and pagination (`roles:read`, `roles:create`) |
| GET | `/v1/roles/:id` (`roles:read`) |
| POST | `/v1/roles/delete`, body `{"ids": [...]}` (1 to 100), all or none; roles without members, not default (`roles:delete`) |
| PATCH | `/v1/roles/:id`; `is_default` marks or unmarks a default role; several may be default (`roles:update`) |
| PUT | `/v1/roles/:id/permissions`, full map in one transaction (`roles:update`) |
| GET | `/v1/roles/:id/members`, `search` and pagination (`roles:read`) |
| PUT/DELETE | `/v1/roles/:id/members/:user_id` (`roles:update`) |
| PUT | `/v1/users/:id/roles` (`roles:update`; replaces every role of the user) |
| GET | `/v1/users`, `search`, `status`, `role_id`, `sort` (`name`, `email`, `status`, `created_at`), `order` and pagination (`users:read`) |
| POST | `/v1/users`, invite: creates a pending account and sends the activation email; a taken email is 409 (`users:create`) |
| POST | `/v1/users/:id/invite`, sends a pending account a new activation email and ends the old link; 409 once activated, 429 inside the email cooldown (`users:update`) |
| POST | `/v1/users/:id/activation-link`, the same without the email: returns the token and `expires_at` in any delivery mode, for the admin to hand over; audited as `user.activation_link`, shares the email throttle (`users:update` and `roles:update`: the account may already hold roles, and the link signs in as it). `GET /v1/users/:id` shows `access.activation_expires_at` |
| DELETE | `/v1/users/:id/sessions`, ends every session of another user (`users:update`) |
| GET | `/v1/users/:id`, with roles and `access`: last login, counted failures in the email window, lock end, live sessions (`users:read`) |
| POST | `/v1/users/disable`, `/v1/users/enable`, body `{"ids": [...]}` (1 to 100), all or none (`users:update`) |
| POST | `/v1/users/unlock`, body `{"ids": [...]}` (1 to 100), all or none: ends the lock, puts a `locked` account back to `active` and drops the counted failures of the email window (`users:update`) |
| GET | `/v1/audit`, newest first; `action` prefix, `user_id` (actor or target), `from`/`to` (RFC 3339) and pagination (`audit:read`) |
| GET | `/v1/audit/export`, the same filters as CSV, up to 10 000 records (more is 400); cells that start like a formula get a `'` (`audit:read`) |
| GET/POST | `/v1/products` |
| GET | `/v1/products/list` |
| GET/PATCH | `/v1/products/:id` |
| POST | `/v1/products/delete`, body `{"ids": [...]}` (1 to 100), all or none |

**Auth tokens:** EdDSA (Ed25519), a base64 PEM from `openssl genpkey -algorithm ed25519`; access claims `sub`, `sid`, `jti`, `iat`, `exp`; refresh same with `jti` hashed at rest. Defaults: access 15m, refresh 60m. **Key rotation:** set the new key in `ACCESS_TOKEN`/`REFRESH_TOKEN` and the old one in `ACCESS_TOKEN_PREVIOUS`/`REFRESH_TOKEN_PREVIOUS`; the old key is only verified, never used to sign, and is removed after one refresh lifetime. An access key may never equal a refresh key, current or previous.

**Errors (confirmed):** `pkg/apperror` → JSON `{code,message,field?,details?}` via `rest/mapper`; errors raised by Fiber itself (404, 405, 413) use the same shape. Login failures are always `INVALID_CREDENTIALS`: pending, disabled, locked and unknown accounts answer identically, in the same time. Register answers `201` for known emails too; internal `failure_kind` not echoed.

**User object:** login, refresh and `/v1/me` return the same `user`: identity, `is_superuser`, `permissions` (resource → actions) and `preferences`, loaded live. They are response data for the interface to adapt; the server still decides each request from the database, and the JWT still carries none. If the profile fails to load, login and refresh answer the tokens without `user`, since a refresh has already spent the old token, and the request log records `Request degraded` with the cause.

**Session list:** `last_seen_at` is the time of the last login or refresh, not of the last request: it is not written per request.

**Own-account routes:** the user id comes from the verified token, never from the request, and every query filters by it. Another user's session or notification answers `404`.

**Pagination / idempotency:** `GET /v1/products`, `GET /v1/users`, `GET /v1/roles` and `GET /v1/me/notifications` return `{items, pagination{page,limit,total_items,total_pages}}`; limit defaults to 20, up to 100. Products, users and roles also take `sort` (allowlist) and `order`. `GET /v1/products/list` returns enabled products as id and name, up to 1000. Register is idempotent on email: an unactivated account gets a fresh token, any other answers as new and sends nothing. Outbox rows are append-only inserts (no idempotency key column today).

**Contracts (authz on wire):** Bearer middleware (`/v1/me`, logout) calls `input.Authenticator`, which validates the access JWT + live session. Resource groups use `Token`, which validates the JWT alone; their `Require(resource, action)` calls Authorizer, which checks the live session and the permission in one query (fail closed).

## Control flow

**Sync (request path, confirmed):**

1. **Register** — tx: user `pending` + empty credential + activation token hash + outbox enqueue; optional plaintext token in response if `AUTH_MANUAL_TOKEN_DELIVERY`.
2. **Activate** — validate token and hash outside the transaction; tx: revalidate token → set password → `active` → every enabled default role → mark token used.
3. **Login** — rate-limit checks on attempts → status/lockout → verify password → atomic credential update → create session + issue JWT pair → record attempt.
4. **Refresh** — parse refresh → load session by `sid` → reuse detection / compare-and-swap rotate → new pair. Each refresh extends the session, never past `AUTH_SESSION_MAX_LIFETIME` (30 days) from login. A lock does not stop refresh. The token rotated in the last 10 s answers `409`, without revoking: the client reads the winner's token and retries.
5. **Logout** — Bearer resolves the session → revoke it (`user_logout`).
6. **Password reset** — request enqueues outbox; confirm hashes password, revokes all sessions, marks token used.

**Notifications:** written by the auth use case for password changed (same transaction as the reset), account locked, refresh token reuse and login from a client never seen on the account (best effort: a lost notice does not fail the login). Clients poll. First login notifies nothing; after it, a client with no session left on the account counts as new. Each one carries a `kind` (`new_login`, `account_locked`, `session_reused`, `password_reset`, `password_changed`) and string `params` (`ip`, `until`); the web app writes the text in the user's language from them. `title`/`message` stay as English fallbacks.

**Emails:** plain text in the user's saved language (Portuguese by default, English otherwise), chosen when the email is queued. The link carries the token in the URL fragment (`/activate#token=...`), which never reaches a server log or a `Referer` header; the web app reads it and clears the address bar.

**Async (confirmed):** mailer claims outbox with `SKIP LOCKED`, sends SMTP (or terminal `smtp_disabled`), backoff on retryable failure.

**Health:** Postgres ping (`ok` / `down`), reused for 1 s. One request probes at a time; the others answer the last report, `down` until the first probe ends.

## Failure domains

**Confirmed what fails together:**

| Domain | Behavior |
|--------|----------|
| Postgres down | Backend/mailer boot fails; health `postgres=fail` |
| Fiber timeouts | `API_READ/WRITE/IDLE_TIMEOUT` (30s/30s/120s); shutdown `API_SHUTDOWN_TIMEOUT` 30s |
| Login abuse | DB-backed attempt counters per client+account (5), client (`/64` on IPv6) and account (50) → `TOO_MANY_REQUESTS` with `Retry-After` (seconds until the oldest counted failure leaves the window); lockout at 50 answers `INVALID_CREDENTIALS`. One client cannot lock the owner out. Each attempt is reserved under an advisory lock before hashing, so parallel requests cannot outrun a limit. Passwords need 8 to 128 bytes and may not be a common first guess, one repeated character, or the account's own name or email. A password reset request answers no sooner than 300 ms, so an unknown email is not told apart by timing. `AUTH_ALLOW_REGISTRATION=0` closes public sign-up and leaves invitations only (register answers 403). |
| Memory | Body limit 16 KiB; at most 4 Argon2 hashes (256 MiB live); `GOMEMLIMIT` and container limits in compose; Argon2 runs outside transactions; waiting for a hashing slot is cancellable, but a running Argon2 calculation finishes. PHC version, costs, salt and digest lengths are bounded before allocation |
| Email flood | `AUTH_EMAIL_TOKEN_COOLDOWN` (60 s) and `AUTH_EMAIL_TOKEN_HOURLY_MAX` (5) per account and purpose; over the cap the live link survives |
| Refresh reuse | Session revoked `token_reuse` |

**HTTP limits:** `API_RATE_LIMIT_*` per client IP (IPv6 by `/64`) per minute, counted in the UNLOGGED table `sys.rate_limit` with one atomic upsert per request, so every instance shares the limit. pg_cron drops past minutes every 5 minutes. A counter failure answers 500. Login and email limits also live in Postgres.

**Not implemented (confirmed):** separate DLQ table (terminal fails stay on outbox as `failed`).

**Inferred:** no circuit breaker library; outbound HTTP none on request path today.

## Observability

**Confirmed:** zerolog via `pkg/logger`; base fields `version`, `service` (`backend`|`mailer`), `pid`. Boot logs host/port/env/errors.

**Confirmed:** `RequestLog` uses Fiber's zerolog integration for ordinary requests and returned errors, with a wrapper for internal causes stored in `mapper.LocalCause`. It records 5xx responses at error level and degraded answers at warning level even when `API_LOGGER=0`; enabling `API_LOGGER` also records ordinary requests. Each request produces at most one request-log entry. Fields include status, method, client IP, latency, PID, path and `request_id`; query strings, headers and bodies are omitted. Internal application-error causes stay in the log and are not added to HTTP responses. The auth use case logs a warning when a best-effort step fails (attempt settlement, notification).

**Correlation id:** `RequestID` gives each request an id, returned in `X-Request-ID`. An incoming id is kept only from a trusted proxy. `pkg/reqctx` carries it, with the authenticated user, through the context: into the request log, `sys.outbox.request_id` (logged by the mailer) and the audit log.

**Audit log:** `usr.usr_audit_log` records role, permission, membership, user status, unlock, invitation and its resend, session revocation and product changes from the API and `cmd/admin`, through `policy.Audit` in the transaction of the change. A NULL actor means the CLI. The app role may only insert. `GET /v1/audit` reads it (`audit:read`); there is no cleanup. The list total stops at 10 001 (one past the export limit), so a filter that matches more reads as "more than 10 000" instead of counting the whole log.

**Diagnostics:** `DIAGNOSTICS_ADDRESS` defaults to `127.0.0.1:9091` inside each container. `/metrics` returns JSON with bounded HTTP latency buckets, server errors, hashing queue/active counts and wait time, `database/sql` pool statistics, outbox count/oldest age and runtime heap/GC counters. `/health` checks PostgreSQL and, for the mailer, a successful worker heartbeat within one minute. No public API route exposes these diagnostics. Compose uses healthchecks and stop grace periods of 40s (backend) and 60s (mailer).

**Absent:** distributed tracing. Add OpenTelemetry when a second service joins the request path.

Do not log refresh tokens, passwords, or email token plaintext (payload in outbox is sensitive — treat as secret storage until mailed).

## Delivery

**Confirmed:**

- Schema: `golang-migrate` on `deploy/db/migrations`, applied by hand with `make migrate-up` (also configures database logins); no container runs them
- Runtime: Docker Compose (`deploy/docker/compose.yml` + `services.compose.yml`: postgres, backend, mailer; app containers run as `nobody`). The mailer gets throwaway JWT keys (`ACCESS_TOKEN=new`, `SYS_SERVICE=mailer`), and no inherited backend database password; its database role is restricted to outbox delivery
- Dev: Air (`make dev-backend` / `dev-mailer`); binaries `make build-*`
- Config: `config/.env` from `config/env.sh` / `make init`

**CI:** vet, tests with `-race` and Postgres, lint, `govulncheck`, and a check that `make swagger` changes nothing.

**Rollback:** SQL `*.down.sql` per migration; no feature-flag system in code. **Inferred:** expand/contract discipline is human/process, not automated.

**`cmd/admin`:** operator CLI in the backend image. `make admin-bootstrap`, or `./bin/admin` with no argument on the host, asks name, email and password and creates the first admin, active, with no email involved; it does nothing once any admin exists, and refuses an email already in use. The password is never an argument: hidden on a terminal, one line per value on a pipe. `make admin-promote` asks the email and gives that active user the `admin` role; `make admin-disable` asks the email, disables that user and revokes their sessions; `make admin-enable` lifts that. `cmd/admin` is the only way to change a user with a protected role.

**Behind a reverse proxy:** set `API_TRUSTED_PROXIES` and keep `API_PROXY_HEADER` on a header the proxy overwrites (`X-Real-IP`), or use `X-Forwarded-For`, read from the right past the trusted proxies.

**External database:** protected environments refuse a non-local Postgres host unless `POSTGRES_URL` uses `sslmode=verify-full`.

## Cost hotspots

**Suspected (no measurements — see PRODUCT.md Unknown cost posture):**

- Argon2id verify on every login (`time=3`, `memory=64MiB`, 4 threads in `pkg/password`)
- Each permission check → one query, no cache
- Login attempt inserts + rate-limit counts
- Login, refresh and `/v1/me` each run user + permissions + preferences queries (roles without member counts)
- `search` is a leading-wildcard `ILIKE`, and lists use `OFFSET` plus a count: scans grow with the table

## ADR index

| ADR | Topic |
|-----|--------|
| [ADR-001](.specs/architecture/decisions/ADR-001-hexagonal.md) | Hexagonal + Fiber |
| [ADR-002](.specs/architecture/decisions/ADR-002-identity-schema.md) | Schema `usr` |
| [ADR-003](.specs/architecture/decisions/ADR-003-secrets-at-rest.md) | Argon2id / SHA-256 / SMTP KEK |
| [ADR-004](.specs/architecture/decisions/ADR-004-rbac.md) | RBAC B+C, CRUD, live authz |

## API docs

Swagger UI at `/swagger/index.html`, outside protected environments only. Handlers under `rest/handler` carry `swag` annotations and `make swagger` regenerates `internal/adapter/inbound/http/rest/docs/docs.go`. Each handler package mounts its own routes (`Routes`); every operation has one `@Tags` tag, by subject; `x-tagGroups` in `rest/swagger.go` groups the tags, rendered by the plugin in `rest/docs/swagger.js` as collapsible sections, closed by default. Custom styles live in `rest/docs/swagger.css`. `swagger_test.go` fails when a route and the docs disagree.

An icon-only theme toggle sits at the right of the API title, on the same line. The first visit follows the system theme; the selection is saved in browser local storage when available. The button has a tooltip, supports keyboard activation and exposes its accessible name and state with `aria-label` and `aria-pressed`.

## Known gaps vs intent

1. **Observability thin** — correlation id and audit log, but no traces.
2. **Scale and SLOs unmeasured** — no measured throughput, latency or availability targets; the shared rate limiter costs one database write per request.

## Boundaries and verification

Application services use a small logging port, a protected email payload codec and shared identity policies. AES and the JSON outbox format live in `adapter/outbound/emailpayload`. The admin CLI consumes `input.AdminService`. Non-transactional account/user/role dependencies expose only the needed repositories; transactions keep one UnitOfWork so operations still share a single database connection. Generic transaction locks belong to UnitOfWork, not login attempts.

Authentication is split into login, refresh, request authentication, session insertion, attempt limits and notifications. Integration scenarios live under `tests/integration`, while pure policies and handlers keep focused tests beside their code. `tests/architecture` guards dependency direction. SQL record maintenance timestamps use the database/runtime clock; login attempt and email token issuance times, expirations and session rules use the application Clock. Health TTLs and diagnostic heartbeats intentionally use elapsed real time.

Products keep numbered pagination. `cursor=start` selects a count-free cursor page ordered by `(created_at DESC, id DESC)`; subsequent requests send `next_cursor`. Cursor mode only supports that order, with `page=1`; search must remain unchanged while traversing. An exact-sized final page may return a cursor whose following page is empty. Migration 000004 adds the matching composite index and a trigram index for substring search.

Benchmarks: `go test -run '^$' -bench BenchmarkHashParallel -benchmem ./pkg/password` and, with an empty TEST_POSTGRES_URL, `go test -run '^$' -bench BenchmarkHTTPProducts -benchmem ./tests/integration`. The latter exercises Fiber, JWT/session validation, RBAC and SQL through app.Test; TCP/TLS and production traffic are outside its scope. Use deployment measurements before claiming capacity or changing cryptographic cost.
