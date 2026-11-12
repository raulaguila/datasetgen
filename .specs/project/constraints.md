# Constraints

Derivado de `config/validate.go`, CHECKs SQL e comentários de tabela.

## Ambiente protegido

`SYS_ENVIRONMENT` em `production`, `prod`, `staging` ou `stage`:

- `CORS_ORIGINS` não pode ser vazio nem `*`
- `AUTH_MANUAL_TOKEN_DELIVERY` não pode estar ligado
- SMTP habilitado exige `SMTP_HOST`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` e `SMTP_APP_URL` (URL do front, usada nos links dos e-mails) reais (sem placeholder `example.com`)

## Auth

- `AUTH_MAX_FAILED_LOGIN_ATTEMPTS` e `AUTH_ACCOUNT_LOCK_DURATION` > 0

## Segredos

- Senha: só Argon2id; hash NULL até activate ou bootstrap admin
- Refresh `jti` e tokens SMTP: SHA-256; plaintext nunca gravado
- Regenerar `SMTP_MASTER_KEY` invalida senhas SMTP armazenadas; rotacionar só via re-wrap

## Produto

- Só `usr_user.status = active` autoriza login
- HTTP não ecoa `failure_kind`
- Authz: `is_superuser` ou `resource='*'`; bits CRUD; catálogo `usr_resource`
- Sentinela `*` não é linha de `usr_resource`
- Módulo exemplo `products` no schema `app`; sem multi-tenant SMTP / dashboard hexago na v1
