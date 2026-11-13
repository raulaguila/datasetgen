# End-to-end tests

Playwright drives the real screens against a real backend. The flows are the
ones a regression would hurt most: sign-in, invite and activation, password
reset, saving a role's permissions, a collection whose records follow its schema,
and its tools (CSV import, bulk edit, views, trash, API keys, field history,
reference and file fields).

## What the run needs

- A backend with a migrated database, reachable at `E2E_API_URL`
  (default `http://127.0.0.1:9990`), started with
  `AUTH_MANUAL_TOKEN_DELIVERY=1` so it hands activation and reset codes back
  instead of mailing them. Never point this at production.
- Test-only limits on that backend, so a run is not throttled:
  `AUTH_EMAIL_TOKEN_COOLDOWN=1s` (a new activation link right after the invite)
  and `API_RATE_LIMIT_AUTH=100` (a run makes more sign-ins and activations than
  the default ten per minute allows), and `API_RATE_LIMIT_GLOBAL=2000` (the
  whole suite makes more than 300 requests a minute from one address).
- An admin account on it, made with `admin bootstrap`:
  `E2E_ADMIN_EMAIL` and `E2E_ADMIN_PASSWORD`.
- Chromium for Playwright, once per machine: `pnpm exec playwright install chromium`.

Every run invites new people with unique addresses, so a database can be reused.

## Running

```bash
E2E_ADMIN_EMAIL=admin@example.test E2E_ADMIN_PASSWORD='…' pnpm e2e
```

The config builds the app and starts `next start` on port 3101 (`E2E_PORT`)
pointed at the backend, or reuses a server already running there. The
production build matters: it sends the real Content-Security-Policy, which
`next dev` relaxes (a validator that needed `eval` broke only in production).
`E2E_DEV=1` runs `next dev` instead, for a faster loop while writing a test.
