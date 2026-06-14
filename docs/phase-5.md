# Phase 5 — Next.js dashboard

**Goal (from BUILD.md):** a Next.js 14 (App Router, TS) + Recharts dashboard that
reads the gateway's `/admin/stats/*` JSON and shows overview tiles, spend-over-time,
cost-by-model, requests-by-provider, a per-key budget table, and a live req/min
tile. Done when: the dashboard renders real numbers from real traffic.

**Status:** ✅ done & verified — the dashboard server-renders live values from
`request_logs` (including key names, request counts, cache-hit rate, spend) and
the live tile streams requests/min from Redis.

Built in two parts:

- **Part 1 — the data API** (committed separately): `/admin/stats/*` Go endpoints +
  the Redis live counter. See the dedicated section below.
- **Part 2 — the dashboard** (`dashboard/`): the Next.js app described here.

---

## Part 1 — stats API (Go)

| File | What it does |
|------|--------------|
| [internal/store/stats.go](../internal/store/stats.go) | Aggregate SQL over `request_logs`: overview, timeseries, by-model, by-provider, by-key. |
| [internal/metrics/live.go](../internal/metrics/live.go) | Redis per-minute request counter (`live:reqs:<minute>`, 2-min TTL) + middleware on the chat route. |
| [internal/api/stats.go](../internal/api/stats.go) | `GET /admin/stats/{overview,timeseries,by-model,by-provider,by-key,live}` (behind `ADMIN_TOKEN`). |

Notable: latency p50/p95 are computed over **non-cache-hit** rows (cache hits are
~0ms and would skew the percentiles); a cache hit logs `cost_usd = 0`, so the
overview's spend reflects real provider charges only.

## Part 2 — dashboard (Next.js)

A dark "control-room" console: IBM Plex Mono numerals, an acid-lime accent, a
hairline grid background, and a staggered page-load reveal.

| File | What it does |
|------|--------------|
| [dashboard/app/page.tsx](../dashboard/app/page.tsx) | Server component — fetches all stats in parallel (server-side, with the admin token) and lays out the page. `force-dynamic` so every load is fresh. |
| [dashboard/lib/api.ts](../dashboard/lib/api.ts) | Server-only gateway client; each call is `safe()`-wrapped so one failing endpoint can't blank the page. |
| [dashboard/app/api/live/route.ts](../dashboard/app/api/live/route.ts) | Route handler the browser polls for the live tile — proxies to the gateway so `ADMIN_TOKEN` never reaches the client. |
| [dashboard/components/](../dashboard/components) | `Stat`, `SpendChart` (area+line), `ByProviderChart` (donut), `ByModelChart` (bars), `KeysTable` (budget bars), `LiveTile` (polls 5s), `AutoRefresh` (20s). |
| [dashboard/Dockerfile](../dashboard/Dockerfile) | Multi-stage Next standalone build (node:22-alpine). |
| [docker-compose.yml](../docker-compose.yml) | `dashboard` service on `:3000`, depends on a healthy gateway. |

### Security model

The browser **never** sees `ADMIN_TOKEN`. All gateway calls happen server-side
(server components + the `/api/live` route handler); the dashboard reaches the
gateway over the compose network (`GATEWAY_URL=http://gateway:8080`).

## How to run / verify

```bash
docker compose up -d --build
# open the dashboard:
open http://localhost:3000      # (or just visit it in a browser)
```

Generate traffic (mint a key, send a few chat requests) and the tiles, charts,
keys table, and live tile populate. Verified: dashboard returns 200, the SSR HTML
contains real key names + request counts + cache-hit rate, and `/api/live`
reports the current requests/minute.

## Key decisions

- **Server-side data fetching** keeps the admin token off the client and makes the page work without a client API layer.
- **`safe()` per endpoint** + a connection banner → the page degrades gracefully if the gateway is down, rather than throwing.
- **Two charts, one composed:** spend & requests share a chart (requests as area, spend as a secondary line) since at low volume request counts read better than fractions-of-a-cent costs.
- **Live tile polls a Next route**, the rest refresh via `router.refresh()` every 20s — real-time feel without websockets.

## Deferred to later phases

- **Screenshot/GIF in the README** (BUILD.md §8) — capture from the running dashboard.
- **Range switcher** (24h/7d/30d) on the spend chart — the API already supports `?range=`.
- **Anthropic data** will appear automatically in by-provider/by-model once the Anthropic key is added and traffic flows.
- **Semantic caching, SSE streaming, budget alerts, load test** → Phase 6.
