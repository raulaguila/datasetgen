# Endpoints

Fonte: `Routes` de cada handler em `internal/adapter/inbound/http/rest/`, montados em `server.go`. Limites por IP e minuto: global `API_RATE_LIMIT_GLOBAL`, auth `API_RATE_LIMIT_AUTH`, senha `API_RATE_LIMIT_PASSWORD`.

| Método | Rota | Acesso | Limite |
|--------|------|--------|--------|
| GET | `/health` | público | nenhum |
| GET | `/swagger/*` | público, só fora de produção | nenhum |
| POST | `/v1/auth/register` | público | auth |
| POST | `/v1/auth/activate` | público | auth |
| POST | `/v1/auth/login` | público | auth |
| POST | `/v1/auth/refresh` | refresh token | global |
| POST | `/v1/auth/logout` | Bearer | global |
| POST | `/v1/auth/password-reset/request` | público | senha |
| POST | `/v1/auth/password-reset/confirm` | público | senha |
| GET/PATCH | `/v1/me` | Bearer; PATCH renomeia | global |
| POST | `/v1/me/password` | Bearer; senha atual; encerra as outras sessões | senha |
| GET | `/v1/me/sessions` | Bearer | global |
| DELETE | `/v1/me/sessions/others` | Bearer | global |
| DELETE | `/v1/me/sessions/:id` | Bearer, só sessão própria | global |
| GET | `/v1/me/preferences` | Bearer | global |
| PUT | `/v1/me/preferences` | Bearer | global |
| GET | `/v1/me/notifications` | Bearer | global |
| GET | `/v1/me/notifications/count` | Bearer | global |
| PUT | `/v1/me/notifications/read` | Bearer, só notificações próprias; `{"ids": [...]}` (1 a 100, tudo ou nada) ou sem body para todas | global |
| GET | `/v1/permissions/resources` | `roles:read` | global |
| GET | `/v1/roles` | `roles:read`; não lista `is_superuser` nem grant `*` | global |
| POST | `/v1/roles` | `roles:create` | global |
| GET | `/v1/roles/:id` | `roles:read`; não `is_superuser` | global |
| POST | `/v1/roles/delete` | `roles:delete`; `{"ids": [...]}` (1 a 100); tudo ou nada; sem membros; não `is_superuser` nem padrão | global |
| PATCH | `/v1/roles/:id` | `roles:update`; não `is_superuser`; padrão não desativa; `is_default` marca/desmarca (vários padrões) | global |
| PUT | `/v1/roles/:id/permissions` | `roles:update`; não `is_superuser`; mapa completo em uma transação | global |
| GET | `/v1/roles/:id/members` | `roles:read` | global |
| PUT | `/v1/roles/:id/members/:user_id` | `roles:update`; não `is_superuser` | global |
| DELETE | `/v1/roles/:id/members/:user_id` | `roles:update`; não `is_superuser` | global |
| PUT | `/v1/users/:id/roles` | `roles:update`; substitui todos os papéis; não `is_superuser` | global |
| GET | `/v1/products` | `products:read` | global |
| GET | `/v1/products/list` | `products:read` | global |
| GET | `/v1/products/:id` | `products:read` | global |
| POST | `/v1/products` | `products:create` | global |
| PATCH | `/v1/products/:id` | `products:update` | global |
| POST | `/v1/products/delete` | `products:delete`; `{"ids": [...]}` (1 a 100); tudo ou nada | global |
| POST | `/v1/users/disable` | `users:update`; `{"ids": [...]}` (1 a 100); tudo ou nada; não a própria conta | global |
| POST | `/v1/users/enable` | `users:update`; `{"ids": [...]}` (1 a 100); tudo ou nada | global |
| POST | `/v1/users/unlock` | `users:update`; `{"ids": [...]}` (1 a 100); tudo ou nada; encerra o bloqueio, `locked` volta a `active`, zera as falhas da janela | global |
| POST | `/v1/users` | `users:create`; convite: conta pendente + e-mail de ativação; e-mail já usado = 409 | global |
| POST | `/v1/users/:id/invite` | `users:update`; só conta pendente (senão 409); novo e-mail de ativação, o link antigo deixa de valer; 429 dentro do intervalo entre e-mails | global |
| POST | `/v1/users/:id/activation-link` | `users:update` e `roles:update` (a conta pendente pode já ter papéis, e o link entra como ela); como o anterior, mas sem e-mail: devolve o token e `expires_at` mesmo com entrega por e-mail, para o admin repassar; auditado como `user.activation_link` | global |
| DELETE | `/v1/users/:id/sessions` | `users:update`; não a própria conta; não `is_superuser` | global |
| GET | `/v1/audit` | `audit:read`; filtros `action` (prefixo), `user_id`, `from`, `to` | global |
| GET | `/v1/audit/export` | `audit:read`; CSV com os mesmos filtros, até 10 000 registros (acima disso 400); células com cara de fórmula ganham `'` | global |

`/v1/auth/refresh` responde `409` ao perdedor de dois refresh simultâneos: o cliente lê o token do vencedor e repete.

Login, refresh e `/v1/me` devolvem o mesmo objeto `user`, com `is_superuser`, `permissions` e `preferences`. Listas paginadas: `{items, pagination{page,limit,total_items,total_pages}}`.

`GET /v1/users/:id` traz `access` (último login, falhas na janela, fim do bloqueio, sessões vivas). Login acima de um limite de tentativas responde `429` com `Retry-After`. Notificações trazem `kind` e `params` para o front traduzir.
