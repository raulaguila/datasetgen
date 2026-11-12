# infra

`internal/infra.Resources` expõe `PostgresDB`. `New` loga host/database e conecta via `postgres.Connect`. `Shutdown` fecha o `sql.DB`.

Não inicializa SMTP, Fiber nem health server.
