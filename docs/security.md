# Security model

Janus is **self-hosted**: it runs on *your* machine (or your own server) and
holds *your* provider API keys. There is no Janus-operated cloud, no multi-tenant
data store, and no account system. That removes a whole class of SaaS risks — but
it also means **the security boundary is yours to set**. This document explains
the model and the one thing you must get right.

## Threat model in one paragraph

The data plane (`POST /v1/chat/completions`) is protected by **virtual keys**.
The control plane (`/admin/*`: minting keys, changing limits/budgets, disabling
or deleting keys, reading stats) is protected by a single **admin secret**
(`ADMIN_TOKEN`). The dashboard is a thin client that holds `ADMIN_TOKEN`
**server-side only** and talks to the gateway for you — the browser never sees
the token. The realistic attacker is therefore not "someone on the internet"
(if you follow the defaults below); it's **a malicious web page open in your own
browser** trying to reach `localhost` (CSRF / DNS-rebinding). Janus defends
against that on every write route.

## What's already enforced

| Control | Where |
|---|---|
| Provider keys are server-side only — never logged or returned to callers | gateway config |
| Virtual keys are stored as a **SHA-256 hash**; the raw key is shown exactly once | `internal/keys`, dashboard create-key modal |
| Admin/stats/control endpoints require `ADMIN_TOKEN` (constant-time compare) | `internal/api` |
| Write endpoints (`POST`/`PATCH`/`DELETE /admin/keys`) refuse the default `change-me` token | gateway WriteGuard |
| Dashboard keeps `ADMIN_TOKEN` server-side; browser calls same-origin `/api/keys` only | `dashboard/lib/api.ts`, route handlers |
| Write routes validate **Host** (blocks DNS-rebinding) + **Origin** (blocks CSRF) | `dashboard/lib/security.ts` |
| All published Docker ports bind to **127.0.0.1** (loopback) | `docker-compose.yml` |

## Your checklist (single-user, localhost — the default)

1. **Set a strong `ADMIN_TOKEN`.** Not `change-me`. Generate one:
   - macOS/Linux: `make gen-token` (or `openssl rand -hex 32`)
   - Windows PowerShell: `-join ((48..57+97..102) | Get-Random -Count 64 | % {[char]$_})`

   Put it in `.env` (which is git-ignored). The gateway refuses control-plane
   **writes** until this is set to a real value, and the dashboard disables its
   management controls with a hint until then.
2. **Keep the defaults.** `docker compose up` already binds everything to
   `127.0.0.1`. Nothing is reachable from your LAN or the internet.
3. **Never commit `.env`.** Only `.env.example` (blank placeholders) is tracked.

That's it. On your own machine the OS login is your authentication boundary; the
Host/Origin checks handle the browser-borne attacks.

## Exposing it to a team (or any non-localhost use)

The moment a **second person** needs the dashboard, a shared `ADMIN_TOKEN` is no
longer real authentication. **Do not build a login into Janus and do not publish
the admin port directly.** Instead, put it behind a reverse proxy that adds auth
— the standard pattern for self-hosted tools. Example with [Caddy](https://caddyserver.com/):

```caddyfile
# Caddyfile — fronts the dashboard with HTTPS + basic auth.
gateway.internal.example.com {
    # generate the hash with: caddy hash-password
    basic_auth {
        admin $2a$14$...your-bcrypt-hash...
    }
    reverse_proxy 127.0.0.1:3000
}
```

Then tell the dashboard which host it's now served under, so its Host check
keeps working:

```bash
# in .env / the dashboard's environment
DASHBOARD_ALLOWED_HOSTS=gateway.internal.example.com
```

Other equally good options: a VPN / Tailscale, Cloudflare Access, or any
OIDC forward-auth proxy (oauth2-proxy, Authelia). Pick one — just don't expose
the raw admin port.

## Rules of thumb

- 🔒 **Never** expose `/admin/*` (the gateway's `:8080` admin routes or the
  dashboard) to the public internet without an auth proxy in front.
- 🔑 Treat `ADMIN_TOKEN` like a root password. Rotate it if it leaks.
- 🧾 A leaked **virtual key** is contained: disable or delete it from the Keys
  page — apps using it immediately get `401`.
- 🗝️ A leaked **provider key** is on you to rotate at the provider; Janus never
  exposes it, but it lives in your `.env`.
