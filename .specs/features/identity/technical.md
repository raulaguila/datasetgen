# Identity — technical

- Tabelas: ver `existing/infrastructure/schema.md`. RBAC: ADR-004.
- Config: `AuthConfig` (TTL access/refresh, registration, reset; lockout; janelas e-mail/IP/par; teto de e-mails; `AllowManualTokenDelivery`).
- Erros de login: `INVALID_CREDENTIALS`, `TOO_MANY_REQUESTS` (pending/expired/disabled/locked respondem como credencial invalida). Outros: `TOKEN_EXPIRED`; refresh perdedor de corrida: `CONFLICT` (409).
- Rate limit: janelas em `usr_login_attempt` no use case (recusas sem teste de senha nao contam) e middleware `API_RATE_LIMIT_*` por IP/minuto, em memória do processo. Login reserva a tentativa sob advisory lock antes do Argon2. IPv6 conta por `/64` nos dois.
- JWT: chaves RSA distintas access/refresh; claims `sub`/`sid`/`jti`; **sem** permissions.
- Authz: load effective map (view); `is_superuser` ou `*`; senão `Allows(have, need)` CRUD.
- Rotas: `POST /v1/auth/{register,activate,login,refresh,logout,password-reset/request,password-reset/confirm}`, `/v1/me` (perfil, `password`, `sessions`, `preferences`, `notifications`).
- Conta própria: use case `account`; `user_id` sempre do token verificado, toda query filtra por ele.
- Notificações: troca de senha, conta bloqueada, reuso de refresh e cliente novo. Só a troca de senha é transacional; as demais não derrubam o fluxo.
- Primeiro admin: `admin bootstrap`, idempotente sob advisory lock; senha nunca como argumento.
- Persistência: repos Postgres; `shared.WrapError`. Pool `POSTGRES_MAX_CONNS` por processo.
- Argon2: no máximo 4 hashes simultâneos, sempre fora de transação; quem espera vaga desiste no timeout (`TIMED_OUT_ERROR`, sem contar falha de senha).
- E-mail: INSERT `sys.outbox` com token cifrado (`SMTP_MASTER_KEY`); mailer abre, envia e remove o token ao terminar.
