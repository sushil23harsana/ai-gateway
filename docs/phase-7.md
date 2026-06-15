# Phase 7 — Control plane (key management + secure self-host packaging)

**Goal:** turn the dashboard from a read-only report into a **control surface** —
create, edit, disable, and delete virtual keys from the UI — without ever
exposing the admin secret to the browser, and package the whole thing to be
**secure by default** on a self-hosted machine.

**Status:** ✅ done & verified — `tsc` clean, `next build` passes, the admin
token is absent from the client bundle, and the gateway refuses writes until a
real token is set.

---

## What was built

Three layers, committed in order:

1. **Backend control plane** (`91bcde4`) — full CRUD on keys behind a stricter guard.
2. **Dashboard UI** (`0b63814`) — token-safe write routes + an interactive Keys page.
3. **Packaging hardening** (this commit) — loopback-only ports, a token generator, and `docs/security.md`.

## Files

| File | What it does |
|------|--------------|
| [internal/api/admin.go](../internal/api/admin.go) | `Create` / `List` / `Update` / `Delete` handlers; `AdminAuth` (token, constant-time); **`WriteGuard`** — rejects writes when the token is unset or the default `change-me`. |
| [internal/store/store.go](../internal/store/store.go) | `KeyUpdate` (nil = unchanged) + `UpdateAPIKey` (partial update) + `DeleteAPIKey` (`found=false` on unknown id). |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Wires `POST` + `PATCH /admin/keys/{id}` + `DELETE /admin/keys/{id}` through `adminWrite` (`AdminAuth` + `WriteGuard`); `List` stays read-only auth. |
| [dashboard/lib/api.ts](../dashboard/lib/api.ts) | `getKeys()` (management list), `gatewayWrite()` (server-side forwarder that attaches the token), `writesEnabled()`. |
| [dashboard/lib/security.ts](../dashboard/lib/security.ts) | `assertLocalRequest()` — Host allowlist (DNS-rebinding) + same-origin Origin check (CSRF) on every write. |
| [dashboard/app/api/keys/route.ts](../dashboard/app/api/keys/route.ts) · [/[id]/route.ts](../dashboard/app/api/keys/[id]/route.ts) | `POST` / `PATCH` / `DELETE` route handlers: guard → forward → pass status/body back. |
| [dashboard/components/KeysManager.tsx](../dashboard/components/KeysManager.tsx) | Client UI: create modal with **one-time raw-key reveal** + copy, enable/disable, edit, delete-with-confirm; controls disabled + hint banner when no strong token. |
| [dashboard/app/(dash)/keys/page.tsx](../dashboard/app/(dash)/keys/page.tsx) | Merges the management list with per-key spend; renders `KeysManager`. |
| [docker-compose.yml](../docker-compose.yml) | All published ports bind to **127.0.0.1**; `SEMANTIC_THRESHOLD` default aligned to `0.25`. |
| [Makefile](../Makefile) | `make gen-token` — prints a strong random `ADMIN_TOKEN`. |
| [docs/security.md](security.md) | The self-host security model, checklist, and reverse-proxy recipe. |

## How a write flows

```
browser (Keys page)
  └─ fetch POST /api/keys           (same-origin; NO token in the browser)
       └─ Next route handler
            1. assertLocalRequest()  → Host allowlist + Origin == Host, else 403
            2. gatewayWrite()        → attaches Authorization: Bearer <ADMIN_TOKEN>
                 └─ gateway PATCH/POST/DELETE /admin/keys[/{id}]
                      └─ AdminAuth (token) → WriteGuard (reject change-me) → handler
       ◄─ gateway status/body passed straight back (incl. one-time raw key on create)
```

Two independent guards mean a forged cross-site request is blocked at the
dashboard (no valid Origin), **and** a write with a weak/absent token is blocked
at the gateway.

## How to run / verify

```bash
make gen-token                 # copy the output into ADMIN_TOKEN in .env
docker compose up -d --build   # ports bound to 127.0.0.1 only
# open http://localhost:3000 → API Keys → "New key"
```

- With a real token: create a key → the raw `sk-gw-…` is shown once → disable/edit/delete work.
- With `ADMIN_TOKEN=change-me`: the UI controls are disabled with a hint, and a
  direct write returns `403` from the gateway's `WriteGuard`.
- Cross-origin check: a `POST /api/keys` with a foreign `Origin` header gets `403`.

## Key decisions

- **No hand-rolled login.** For an open-source self-hosted tool, the right model
  is *secure-by-default on localhost* + *delegate team auth to a reverse proxy*
  (documented in [security.md](security.md)) — not a bespoke auth system in front
  of a tool that holds provider keys.
- **Token never reaches the browser.** Same pattern as the live tile: the
  dashboard's server side holds it; the browser only calls same-origin routes.
- **Loopback by default.** Datastore ports stay published (so the host
  `go run ./cmd/gateway` dev flow keeps working) but only on `127.0.0.1`.

## Deferred

- A pluggable forward-auth / OIDC layer (the code is structured to allow it later).
- Auditable history of admin actions beyond the existing request logs.
- `docs/deploy.md` — shipping the gateway itself to a remote host.
