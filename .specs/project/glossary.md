# Glossary

- **activate**: `POST /v1/auth/activate` — prova a caixa, grava senha Argon2id e associa o papel default.
- **CRUD actions**: bits `create=1`, `read=2`, `update=4`, `delete=8` em `usr_role_permission.actions`.
- **KEK / DEK**: chave mestra (`SMTP_MASTER_KEY`) que envolve chaves de dados usadas para senhas SMTP em rest.
- **`failure_kind`**: motivo interno de login falho em `usr_login_attempt`; não vai na resposta HTTP.
- **`is_superuser`**: flag em `usr_role`; bypass de autorização no Go (fora da view de permissions).
- **`jti`**: id do refresh JWT; no banco só o SHA-256.
- **outbox**: `sys.outbox`; backend grava evento de e-mail; mailer consome.
- **reuse**: apresentar `previous_refresh_token_hash` com sessão viva; revoga com `token_reuse`.
- **sid**: id da sessão no access JWT; unidade de logout. JWT não carrega permissões.
- **sentinela `*`**: `usr_role_permission.resource` grant-all; a view não expande; a aplicação aplica.
- **schema `app`**: domínio de aplicação (products exemplo).
- **schema `sys`**: infraestrutura (outbox de e-mail).
- **schema `usr`**: identidade (users, credenciais, sessões, tokens, tentativas, RBAC).
- **`usr_resource`**: catálogo de nomes de resource para a API.
