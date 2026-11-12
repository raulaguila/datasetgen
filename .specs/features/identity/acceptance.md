# Identity — acceptance

Critérios derivados do schema e dos fluxos `/v1/auth/*`. Cobertura: testes de integração em `internal/adapter/outbound/persistence/postgres` (exigem `TEST_POSTGRES_URL`).

- Não autentica `pending`, `expired`, `disabled` nem `locked` com bloqueio vigente; bloqueio vencido volta a `active`.
- Hash de senha nunca NULL em conta `active` (exceto janela de bootstrap admin).
- Refresh plaintext não aparece em log nem no banco.
- Reuse de refresh (qualquer geração antiga) revoga a sessão; novo refresh não vale.
- Dois refresh simultâneos: um vence, o outro recebe `409` sem token e sem revogar. Após 10 s, o token anterior conta como reuse.
- Conta bloqueada mantém sessão aberta: refresh segue valendo. Só novo login é recusado.
- Bloqueio responde `INVALID_CREDENTIALS`, igual a conta inexistente.
- Novo e-mail de ativação ou reset para a mesma conta respeita `AUTH_EMAIL_TOKEN_COOLDOWN` e `AUTH_EMAIL_TOKEN_HOURLY_MAX`, com a mesma resposta HTTP; acima do teto o link vivo continua válido.
- Um cliente que erra a senha 5 vezes é recusado só para aquela conta; o dono em outro IP continua entrando.
- Logins paralelos não ultrapassam limite: 30 simultâneos no mesmo par dão 5 `INVALID_CREDENTIALS` e 25 `TOO_MANY_REQUESTS`.
- E-mail acima de 254 bytes ou com NUL é recusado no cadastro e não gera erro 500 no login.
- `admin disable` revoga as sessões: access token para na requisição seguinte.
- Cadastro `pending`/`expired` sem token vivo sai da limpeza após 30 dias.
- Dois tokens `activation` unused para o mesmo user são impossíveis (índice único parcial).
- `failure_kind` não aparece no JSON de erro.
- Papel `user` é o único default; activate associa esse papel.
- `admin` tem `is_superuser` e `*`; a view pode devolver `*`; autorização trata superuser/`*` como todas as permissões.
- DELETE de role com usuários vinculados falha.
- Quinta tentativa inválida no mesmo token SMTP marca `used_at`.
- Access JWT não carrega mapa de permissões; authz é ao vivo.
- `admin bootstrap` cria um admin ativo; com admin existente nada faz; e-mail em uso é erro, nunca promove conta alheia; execuções simultâneas criam um só.
- Sessão ou notificação de outro usuário responde `404` e nada muda.
- `permissions` no corpo da resposta é só para a interface: a autorização continua lendo o banco a cada requisição.
