# Tooling

Comandos na raiz. Docker/migrate exigem `config/.env` (`make init` se faltar).

```sh
make check-system
make install-tools
make init
make tidy

make compose-services
make migrate-up
make dev-backend
make dev-mailer

make format
make lint
make audit
make test
make coverage

make build
make zip

make migrate-create name=descricao
make migrate-down
make migrate-reset
make migrate-force version=N
make migrate-version

make compose-up
make compose-build
make compose-down
make compose-log
make compose-exec
make compose-remove
```

Postgres local: `localhost:${POSTGRES_PORT}` (env.sh default `5444`). `DOCKER_SERVICE` default `backend`.
