# Gaps

Restante. Runtime: `ARCHITECTURE.md`. Surfaces: `.keel/surfaces/`.

1. Quem tem `roles:update` pode ampliar o próprio acesso em qualquer recurso, menos `*`.
2. Observability: sem traces; auditoria sem endpoint de leitura.
3. Rate limit HTTP no Postgres: uma escrita por request.
4. `GO-2026-5932` (x/crypto): sem release com correção; código não alcança a função vulnerável.
5. CI no GitHub não verificado: repositório sem acesso pela API.

Não copiar multi-tenant SMTP nem dashboard do hexago.
