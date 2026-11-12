# ADR-003 Segredos em rest

Status: aceito. Implementação: constraints SQL + `pkg/aescrypt`; hashing de senha/token ainda não no Go.

- Senha: Argon2id, nunca plaintext.
- Refresh e tokens de e-mail: SHA-256 no banco.
- SMTP: KEK (`SMTP_MASTER_KEY`) envolve DEKs; bundle por passphrase para export.

Rotação da KEK só via job de re-wrap. Alias `SMTP_CREDENTIAL_KEY` deprecado, ainda no struct.
