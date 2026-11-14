# Template

This repository is meant to be copied: a new project starts from it with
identity, RBAC, sessions, email, audit log, rate limiting and diagnostics
already in place, and grows its own modules next to `collections`.

## Start a new project

```bash
git clone https://github.com/raulaguila/dataset.git my-app
cd my-app/backend
make rename MODULE=github.com/acme/my_app NAME=my_app
make init
```

`make rename` rewrites the Go module path and the name `dataset` in every
tracked file: imports, Docker images and containers, the database roles
`<name>_app` and `<name>_mailer`, the pg_cron jobs and titles. `NAME` must be
lowercase letters, digits and `_`, since it becomes Postgres role names. Then
point the `origin` remote at the new repository. Backend paths below are relative to
`backend/`. The frontend follows (`cd ../frontend && make init`): the visible name
lives in `frontend/src/lib/brand.ts` and the mark in `frontend/src/components/brand-mark.tsx`.

## Add a module

The `products` example module was removed (migration 000020) and `make module` with it. To add a module, copy the products files from git history (`git show eab7529:backend/internal/core/usecase/product/service.go` and the paths below) and follow the steps; they still describe the pattern. The table below is what a module touches. The architecture test (`tests/architecture`) fails
if the core imports an adapter, GORM, Fiber or the config.

| Step | File (products) | What goes there |
|------|-----------------|-----------------|
| 1. Schema | `deploy/db/migrations/000004_products.*.sql` | Table in schema `app`, indexes, `updated_at` trigger, and the RBAC resource row in `usr.usr_resource`. Add a new numbered pair; never edit a released one. |
| 2. Resource | `internal/core/domain/action.go` | Constant with the resource code, equal to the row in `usr.usr_resource`. |
| 3. Domain | `internal/core/domain/product.go` | Entity, filter and patch types. No tags, no imports from adapters. |
| 4. Ports | `internal/core/port/input/product.go`, `internal/core/port/output/product.go` | The service the handler calls and the repository the service needs. |
| 5. Use case | `internal/core/usecase/product/service.go` | Validation and rules. Errors are `apperror` codes. Administrative changes call `policy.Audit` inside the transaction; record `"name"` in the details (create, rename, delete) so the audit log can label the record, even after it is deleted. |
| 6. Repository | `internal/adapter/outbound/persistence/postgres/product/repository.go` | SQL with GORM; wrap errors with `shared.WrapError`. |
| 7. Handler | `internal/adapter/inbound/http/rest/handler/product/handler.go` | Routes with `middleware.Require(authz, resource, action)`, request and response types, `swag` annotations. |
| 8. Wiring | `internal/core/port/input/application.go`, `internal/composition/wire.go`, `internal/adapter/inbound/http/rest/server.go` | Field in `Application`, construction, and the route group behind `bearer`. |
| 9. Tests | `service_test.go`, `handler_test.go`, `tests/integration/product_test.go` | Unit tests for rules and HTTP mapping; integration tests against Postgres. |
| 10. Docs | `make swagger` | Regenerates the Swagger spec from the annotations. |
| 11. Web types | `frontend/src/lib/api/types.ts`, `frontend/src/lib/api/endpoints.ts` | Response types and one function per route. |
| 12. Web screen | `frontend/src/app/(app)/products/page.tsx`, `frontend/src/lib/navigation.ts`, `frontend/src/lib/i18n/messages.ts` | The page, a menu entry gated by `needs: [resource, action]`, and the texts in both languages. |

The app role gets read and write rights on new tables in `usr`, `sys` and
`app` by default privileges. A table in a new schema needs its own `GRANT`.

What a module gets from the products pattern:

- **Edit conflicts.** Each row has a `version`; an update that sends the version
  it read is refused with 409 (`CONFLICT`) when someone saved in between, and
  the screen asks to discard and redo instead of overwriting.
- **Background jobs.** `policy.EnqueueJob(ctx, uow, kind, payload)` inside the
  use case's transaction queues a job in `sys.job`; the module's handlers
  (`product.Jobs`) are registered in `composition.NewJobs`, and every backend
  instance runs the worker. Failed runs retry with backoff up to `max_attempts`;
  `policy.PermanentJobError` stops at once. Handlers must be safe to repeat.
- **Files.** `output.FileStore` keeps uploads on disk (`STORAGE_DRIVER=local`,
  one instance) or in an S3-compatible bucket (`s3`: AWS S3, MinIO, R2). The
  profile picture (`/v1/me/avatar`) is the example. Request bodies are capped at
  about 100 KB for every route; larger uploads need their own listener or a
  streamed body.

A request already carries its correlation id and actor in the context
(`pkg/reqctx`), so logs, outbox messages and audit entries of a new module pick
them up with nothing to wire.

## What a copy inherits

- Hexagonal layout with an import check, and the `shortcut:` comments that name known limits (`grep -rn "shortcut:"`).
- Migrations that run up and down, `dataset_app` and `dataset_mailer` roles without owner rights.
- `X-Request-ID` correlation, administrative audit log, rate limits shared by every instance.
- Private diagnostics server with `/metrics` and `/health`.
- A Next.js frontend with the session in an httpOnly cookie, light and dark themes, pt-BR and en-US.
