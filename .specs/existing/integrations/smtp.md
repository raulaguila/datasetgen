# SMTP

Não há cliente SMTP nem dispatcher no Go. Existe:

- Config env (`SMTP_*`, `SMTP_MASTER_KEY`, alias `SMTP_CREDENTIAL_KEY`)
- `pkg/smtpsecurity` para o modo
- `pkg/aescrypt` para cifrar senha e wrap de DEK; bundle de export por passphrase

Mailer (`cmd/mailer`) só faz o mesmo bootstrap do backend. Integração de envio: gap.
