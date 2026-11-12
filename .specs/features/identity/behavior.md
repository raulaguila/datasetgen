# Identity — behavior

Comportamento implícito nos CHECKs e COMMENT ON. Sem HTTP até o adapter existir.

## Ciclo da conta

1. Cadastro cria `usr_user` `pending` + `usr_credential` sem hash + `usr_token` `activation` + `sys.outbox` `pending` (quando mailer ligado).
2. Activate válido: hash Argon2id, status `active`, papel default, token `used_at`.
3. Token de ativação expira sem uso: status `expired`.
4. Senha errada incrementa `failed_login_attempts`; no limite, `locked_until` e status `locked`.
5. Sucesso zera tentativas; atualiza `last_login_at`.

## Sessão

- Login cria sessão com hash do `jti` atual.
- Refresh grava o `jti` antigo em `previous_refresh_token_hash` e o novo em `refresh_token_hash`.
- Apresentar o hash anterior com sessão viva → revoga `token_reuse`.
- Logout / troca de senha / expiração preenchem `revoked_at` + reason.

## Token SMTP

- Lookup por hash. Expirado ou já usado incrementa `attempts`. Na 5ª, `used_at`.
- Password reset não usa `session_id`.

## Login attempt

- Sempre insert. `success=true` ⇒ `failure_kind` NULL. Falha ⇒ um dos kinds do CHECK.
- HTTP responde só `invalid_credentials` ou `account_locked`, não o kind.
