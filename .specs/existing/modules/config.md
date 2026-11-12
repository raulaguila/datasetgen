# config

`config.Load` / `MustLoad` tentam `config/.env` e o path relativo ao arquivo fonte.

Structs: log, API (porta, prefork, CORS, proxies confiáveis, rate limits, timeouts Fiber), Auth (RSA access/refresh, TTLs, lockout, janelas de tentativa, cooldown de e-mail), Postgres, SMTP (`SMTP_MASTER_KEY`, obrigatória, + host/user/pass).

Variáveis reais do ambiente vencem o `.env`, e a falta do arquivo é tolerada (containers). Timezone default `America/Manaus`.

Validação em `validate.go` (ver `project/constraints.md`).
