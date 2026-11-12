# Surface: mailer-outbox

<!-- keel:surface mailer-outbox -->

Accepted shape: poll consumer in `cmd/mailer` (option A).

## Decisions

- Claim: `FOR UPDATE SKIP LOCKED`, batch 10, poll every 2s
- SMTP disabled → mark `failed` with `last_error=smtp_disabled` (no infinite requeue)
- Backoff: `available_at = now() + min(2^attempts, 15m)`; after 8 attempts → terminal `failed`
- Never log payload `token`

## Ownership

- Writer: backend auth use cases (already)
- Reader/sender: `cmd/mailer` + outbox repo claim/mark APIs + SMTP adapter
