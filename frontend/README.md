# Web

Next.js frontend for the Dataset API. It works as a BFF (backend for frontend): the browser talks only to it, and it calls the API. Everything it needs lives in this folder: code, config, Docker files and Makefile.

## Run

```bash
make init   # creates config/.env with a fresh SESSION_SECRET
make dev    # http://localhost:3006, calls the API on http://localhost:API_PORT
```

With Docker, start the backend stack first (`make compose-build` in `backend/`), then:

```bash
make compose-build
```

The container joins `dataset_network` at `WEB_IP` (default `172.30.97.10`). The backend trusts `X-Real-IP` only from that address, so keep `WEB_IP` equal in both stacks.

`make check` runs the typecheck, lint and tests. Other targets: `make help`.

## Configuration

`config/env.sh` writes every option to `config/.env`, each with a comment. The file holds only web settings; the backend's signing keys and database passwords never reach this process.

## How it works

- The access and refresh tokens live in an httpOnly cookie signed with `SESSION_SECRET`; browser JavaScript never sees them. The user and permissions come from `/v1/me`.
- `/api/auth/*` handles login, logout, refresh and the public account routes; `/api/proxy/*` forwards everything else with the bearer and refreshes once on 401.
- The client IP goes to the API as `X-Real-IP`, read from `CLIENT_IP_HEADER` (default `x-forwarded-for`). A client can forge that header, so in production put a reverse proxy in front that overwrites it.
- Menus and buttons follow `permissions` and `is_superuser`, for display only: the API still checks every call.
- Theme and language follow the saved `preferences` and are written back when changed.

Screens: login, register, activation, password reset, profile (name, password, sessions, preferences), notifications, users, roles with the permission matrix and members, and products.

## Add a screen

1. Types and one function per route in `src/lib/api/types.ts` and `src/lib/api/endpoints.ts`.
2. The page under `src/app/(app)/`, following `products/page.tsx`.
3. A menu entry in `src/lib/navigation.ts`, gated by `needs: [resource, action]`.
4. The texts in `src/lib/i18n/messages.ts`; a key missing in one language fails the typecheck.

The visible name lives in `src/lib/brand.ts` and the mark in `src/components/brand-mark.tsx`.
