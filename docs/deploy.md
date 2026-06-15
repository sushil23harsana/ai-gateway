# Deploying Janus

Janus is **self-hosted** — you run it; there's no Janus cloud. This guide covers
shipping it beyond your laptop to a server you control. Read
[security.md](security.md) first: going off `localhost` is exactly when the
security checklist starts to matter.

There is no separate `deploy` artifact — the same `docker-compose.yml` you run
locally is the deploy unit. The differences are: real secrets, TLS + auth in
front, and (ideally) managed datastores.

## What runs

| Component | Image / source | Notes |
|---|---|---|
| gateway | `Dockerfile` (Go) | the proxy + control plane, `:8080` |
| dashboard | `dashboard/Dockerfile` (Next.js) | the console, `:3000` |
| migrate | gateway image, one-shot | applies `migrations/*.up.sql`, then exits |
| postgres | `pgvector/pgvector:pg16` | needs the `vector` extension (semantic cache) |
| redis | `redis:7` | rate-limit buckets, cache, live counters, spend |

## Option A — one VM with Docker (simplest)

On any small Linux VM (a 1 GB box is plenty to start):

```bash
git clone https://github.com/sushil23harsana/ai-gateway && cd ai-gateway
cp .env.example .env
make gen-token            # paste into ADMIN_TOKEN
# edit .env: set OPENAI_API_KEY (+ ANTHROPIC_API_KEY), ADMIN_TOKEN
docker compose up -d --build
```

Compose binds every port to `127.0.0.1`, so nothing is public yet. Put a reverse
proxy in front for TLS + auth — e.g. Caddy (full recipe in
[security.md](security.md)):

```caddyfile
gateway.example.com {
    basic_auth { admin <bcrypt-hash> }   # protects the dashboard/control plane
    reverse_proxy 127.0.0.1:3000
}
api.example.com {
    reverse_proxy 127.0.0.1:8080         # the data plane your apps call
}
```

Then point the dashboard's Host check at the public name:

```bash
# .env
DASHBOARD_ALLOWED_HOSTS=gateway.example.com
```

## Option B — PaaS + managed datastores

On Fly.io / Railway / Render, deploy the **gateway** and **dashboard** images and
use **managed Redis + Postgres** instead of the compose datastores:

1. Provision managed Postgres (must support `pgvector` — Neon, Supabase, and most
   managed PG do) and managed Redis.
2. Run migrations once against the managed DB: `DATABASE_URL=… go run ./cmd/migrate`
   (or run the `migrate` image as a one-off job).
3. Deploy the gateway with env: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`,
   `ADMIN_TOKEN`, `REDIS_URL`, `DATABASE_URL`, `PRICING_PATH=/app/pricing.yaml`.
4. Deploy the dashboard with `GATEWAY_URL` (internal URL of the gateway),
   `ADMIN_TOKEN`, and `DASHBOARD_ALLOWED_HOSTS=<your dashboard domain>`.
5. Most PaaS give you TLS automatically; add an auth layer in front of the
   dashboard (the platform's access control, an OIDC proxy, or Tailscale).

## Going-to-production checklist

- [ ] `ADMIN_TOKEN` is strong (`make gen-token`) — writes are refused otherwise.
- [ ] Provider keys set via the platform's **secrets**, never committed.
- [ ] TLS terminated in front (reverse proxy / platform).
- [ ] Dashboard/admin behind an auth layer; **admin port never public** unauthenticated.
- [ ] `DASHBOARD_ALLOWED_HOSTS` set to the public hostname(s).
- [ ] Datastores not publicly reachable; Postgres has `pgvector`.
- [ ] Postgres volume is backed up (it holds `request_logs` + api keys + semantic cache).

## Verify

```bash
curl -i https://api.example.com/healthz       # -> 200 {"status":"ok"}
```

Then mint a virtual key (dashboard → API Keys, or `POST /admin/keys`) and point an
app's `base_url` at `https://api.example.com/v1`.

> Janus stays **self-hosted by design** (BUILD.md §9 rules out a hosted SaaS); this
> guide is about running *your own* instance well, not operating it for others.
