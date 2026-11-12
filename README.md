# Dataset

Platform for building datasets to train and fine-tune AI models. Each collection defines its own record format as a JSON Schema; records are checked against it in the browser and again on the server, so the data that reaches training is valid. Login, sessions, role-based permissions and audit come from the identity base this project started from; the products CRUD stays as the template `make module` copies. `cmd/backend` serves HTTP, `cmd/mailer` sends email from the outbox, `cmd/admin` is the operator CLI, all under `backend/`, and `frontend/` is the Next.js app. See [ARCHITECTURE.md](ARCHITECTURE.md) for how it fits together, [PRODUCT.md](PRODUCT.md) for its constraints and [TEMPLATE.md](TEMPLATE.md) to start a new project from it or add a module.

## Layout

- `backend/`: the Go service, with its own `go.mod`, `Makefile`, `config/`, migrations, Docker files and tests. Run every `make` target and `go` command from here.
- `frontend/`: the Next.js app, with its own config, Docker files and Makefile.
- At the root: documentation, `.specs/`, CI and `scripts/rename.sh`.

## Run with Docker

```bash
cd backend
make init            # creates config/.env with fresh secrets
make compose-build   # builds and starts postgres, backend and mailer
make migrate-up      # applies the schema and configures both database logins
make admin-bootstrap # asks name, email and password of the first admin
```

The API answers on `API_PORT` (9977 in the generated file). Swagger UI is at `/swagger/index.html` outside protected environments.

The network has a fixed range (`DOCKER_SUBNET`, default `172.30.97.0/24`) and the backend trusts `X-Real-IP` only from `WEB_IP` (default `172.30.97.10`), the address of the web container. Moving an existing stack to the fixed range needs `make compose-down` first, since Docker cannot change the range of a live network.

`make migrate-down` reverses one migration.

## Develop

From `backend/`:

```bash
make dev-backend
```

```bash
make dev-mailer
```

Tests against Postgres need `TEST_POSTGRES_URL` pointing at an empty database; without it they skip:

```bash
TEST_POSTGRES_URL='postgres://postgres:test@localhost:5432/dataset_test?sslmode=disable' go test -race ./...
```

Other targets: `make help`. After changing a handler annotation run `make swagger`.

## Configuration

`config/env.sh` writes every option to `config/.env`, each with a comment. Any `SYS_ENVIRONMENT` other than development, dev, local or test gets the production rules, enforced at boot.

## Frontend

`frontend/` is a separate Next.js app with its own config, Docker files and Makefile. See [frontend/README.md](frontend/README.md).

## Diagnostics and pagination

JSON logging is the default; use `LOG_FORMAT=text` for an interactive console. Internal diagnostics listen on `DIAGNOSTICS_ADDRESS` (default `127.0.0.1:9091`); `/metrics` is JSON, not Prometheus exposition. Inspect it from inside a container, for example `docker exec dataset_backend wget -qO- http://127.0.0.1:9091/metrics`. Do not publish this port without access controls. `make dev-mailer` defaults its diagnostics to port 9092 so it can run beside the backend on the same host. When launching binaries directly, choose another diagnostics port for one process. An empty address disables diagnostics; Compose healthchecks expect the default address.

For large product lists, request `/v1/products?cursor=start&limit=20`, then pass the returned `pagination.next_cursor` as `cursor`; the field is absent on the last page. This mode omits totals and supports `created_at desc` only. Existing `page`, `limit` and sorting behavior remains available.

The integration suite is in `tests/integration`. Regression tests cover login/reset ordering, concurrent email limits, absolute session lifetime, stale outbox claims, numeric price ordering, cursor ties, restricted mailer grants and HTTP ownership/authorization. Use a new empty database on each test run. The CI and Makefile both pin govulncheck v1.9.0; refresh local tooling with `make install-tools`.
