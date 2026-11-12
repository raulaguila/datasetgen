# Identity — requirements

Modelo no SQL (`usr`) e nos fluxos Go de auth. RBAC: ADR-004.

- Conta nasce `pending`, sem senha, até prova de caixa (activate).
- Só `active` autentica.
- Ativação vencida sem uso → `expired`, ainda sem senha.
- Lockout temporário → `locked` + `locked_until`; disable operacional → `disabled`.
- Senha Argon2id; lockout por tentativas falhas.
- Sessão é a unidade de logout; refresh rotaciona e detecta reuse.
- Tokens de e-mail: `activation` e `password_reset`, um ativo por propósito; entrega via outbox (mailer consumer pendente).
- Toda tentativa de login é gravada, inclusive e-mail desconhecido.
- RBAC B+C: resource × actions CRUD; catálogo `usr_resource`; `user` default sem linhas; `admin` com `is_superuser` e `resource='*'`.
- Authz ao vivo (JWT sem permissions); Authorizer no core — middleware HTTP nas rotas de domínio ainda pendente; grants granulares via API ainda pendente.
