# AI Gateway — Build Spec (for Claude Code)

> **What you're building:** a self-hostable **LLM gateway / proxy** that sits between
> applications and LLM providers (OpenAI, Anthropic) and gives teams the control plane
> they're missing: **cost & token analytics, semantic caching, rate limiting, virtual
> API keys, and multi-provider failover** — all behind a real-time dashboard.
>
> Think "Helicone / LiteLLM, built from scratch" — a production-shaped systems project.

**Author:** Sushil Harsana · **Codename:** `ai-gateway`
**Portfolio framing:** the evolution of an API-gateway / rate-limiting / Redis background into the AI era.

---

## 0. How to work through this (read first)

- Build in the **phases below, in order**. Each phase is independently runnable and demoable. Do not jump ahead.
- After each phase: ensure `docker compose up` works, write/run the phase's tests, and update the README's "Status" checklist.
- Prefer **standard library + small, well-known deps** over frameworks. This is a systems project — keep it lean and readable.
- Commit at the end of each phase with a clear message.
- Ask me before adding a heavy dependency or changing the architecture in section 2.

---

## 1. Tech stack (fixed — do not substitute)

| Layer | Choice | Notes |
|---|---|---|
| Gateway core | **Go** (1.22+), net/http, `httputil` | The proxy. Concurrency, streaming, low overhead. |
| Hot state / cache / rate-limit | **Redis** 7 | Response cache, token-bucket counters, live metrics. |
| Analytics store | **PostgreSQL** 16 | Append-only request log + aggregates. |
| Dashboard | **Next.js 14** (App Router, TypeScript) + Recharts | Reads the gateway's admin API. |
| Local infra | **Docker Compose** | Redis + Postgres + gateway + dashboard. |
| Config | env vars + a `pricing.yaml` | 12-factor; no secrets in code. |

Go module path: `github.com/sushil23harsana/ai-gateway`.

---

## 2. Architecture

```
                ┌────────────────────────────────────────────────┐
   client       │                 AI GATEWAY (Go)                 │
 (virtual key)  │                                                 │
  ───────────►  │  1 auth virtual key   2 rate limit (Redis TB)   │
                │  3 cache lookup (Redis)                         │     ┌──────────┐
                │  4 on miss → provider router ───────────────────┼───► │ OpenAI / │
                │  5 parse usage → cost → async log               │ ◄───┤ Anthropic│
                │  6 store cache   7 stream/return response       │     └──────────┘
                └───────┬───────────────────────────┬─────────────┘
                        │ writes                     │ reads
                        ▼                            ▼
                   PostgreSQL                     Redis  ◄──── /admin/stats
                 (request_logs)               (counters, cache)        │
                        ▲                                              │
                        └──────────── aggregates ──────────► Next.js dashboard
```

**Request lifecycle (the heart of the project):**
1. **Authenticate** the caller's *virtual key* (gateway-issued, not the provider key).
2. **Rate limit** per key using a Redis token-bucket.
3. **Cache lookup** — return a cached response on hit (record a hit).
4. **On miss**, route to the provider, **injecting the real provider key** server-side.
5. **Relay** the response (support **streaming/SSE** passthrough).
6. **Parse token usage**, compute **cost** from `pricing.yaml`, write an async **request log** (never block the response on logging).
7. **Store** the response in cache.

Keep the proxy path non-blocking: logging and metric writes go through a buffered channel + worker pool, never inline on the response.

---

## 3. Data model

**Postgres — `request_logs`** (append-only):
```
id            uuid pk
created_at    timestamptz
api_key_id    uuid        -- the virtual key used
provider      text        -- openai | anthropic
model         text
status        int         -- http status returned to client
cache_hit     bool
tokens_in     int
tokens_out    int
cost_usd      numeric(12,6)
latency_ms    int
error         text null
```

**Postgres — `api_keys`** (virtual keys):
```
id            uuid pk
name          text        -- "team-frontend", "dev-sushil"
key_hash      text        -- sha256 of the virtual key (store hash, never raw)
monthly_budget_usd numeric null
rate_limit_rpm int        -- requests/min, drives the token bucket
created_at    timestamptz
disabled      bool
```

**Redis keys:**
- `ratelimit:{api_key_id}` — token-bucket state.
- `cache:{sha256(normalized_request)}` — cached response JSON (TTL configurable).
- `live:cost:{api_key_id}:{yyyy-mm}` — running monthly spend counter (fast budget checks).
- `live:reqs:{minute}` — rolling request counter for the dashboard's live tile.

---

## 4. Phases (build in this order)

### Phase 0 — Scaffold
- Repo layout (section 6), `docker-compose.yml` (redis, postgres), `.env.example`, Makefile (`make dev`, `make test`, `make migrate`).
- Go config loader (env), structured logging (`slog`), graceful shutdown.
- DB migrations (use `golang-migrate` or plain SQL files + a tiny runner).
- **Done when:** `docker compose up` starts redis + postgres and the gateway serves `GET /healthz` → 200.

### Phase 1 — Pass-through proxy + observability (the core)
- `POST /v1/chat/completions` proxies to **OpenAI**, injecting the real key from env.
- Measure latency; parse `usage` from the provider response; compute cost via `pricing.yaml`.
- Write a `request_logs` row **asynchronously** (channel + worker).
- **Done when:** a curl through the gateway returns OpenAI's response *and* a row lands in `request_logs` with correct tokens, cost, latency.

### Phase 2 — Virtual keys + rate limiting
- `api_keys` table + an admin endpoint to create a key (returns the raw key once; store only the hash).
- Middleware: authenticate `Authorization: Bearer <virtual-key>`; reject unknown/disabled keys.
- **Redis token-bucket** rate limiter keyed by `api_key_id`, limit from `rate_limit_rpm`. Return `429` with `Retry-After` when exceeded.
- **Done when:** unknown key → 401; exceeding the limit → 429; valid key within limit → proxied.

### Phase 3 — Response caching
- Build a stable cache key: `sha256` of normalized request (model + messages + temperature, etc.; ignore volatile fields).
- On hit: return cached response, set `cache_hit=true`, **skip the provider call**. On miss: proxy, then store with TTL.
- Add a per-key toggle + configurable TTL. Track cache-hit metrics.
- **Done when:** identical requests show a measurable latency drop and `cache_hit=true` in logs; cache-hit rate is queryable.

### Phase 4 — Multi-provider + routing/failover
- Define a `Provider` interface (`Name()`, `Translate(req) -> providerReq`, `ParseUsage(resp)`, `Endpoint()`).
- Implement **OpenAI** and **Anthropic**. Map a unified request shape to each.
- Routing: choose provider by requested model; **failover** to a configured fallback provider on 5xx/timeout.
- **Done when:** the same gateway request can be served by either provider, and a forced provider outage fails over cleanly (and the failover is logged).

### Phase 5 — Next.js dashboard
- App Router + TypeScript. Pages/sections:
  - **Overview:** total spend (today / month), requests, cache-hit rate, p50/p95 latency.
  - **Spend over time** (line chart), **cost by model** (bar), **requests by provider**.
  - **Keys** table: per-key spend vs. budget, RPM, status.
  - **Live** tile: requests/min from Redis.
- Data comes from gateway **`/admin/stats/*`** JSON endpoints (add them in Go: aggregate from Postgres, live counters from Redis).
- **Done when:** dashboard renders real numbers from real traffic generated through the gateway.

### Phase 6 — Stretch (pick what impresses)
- **Semantic caching:** embed the request, cosine-match against recent requests (Redis vector search / `pgvector`); serve near-duplicate answers from cache. *(This is the headline feature — do it if time allows.)*
- **Streaming (SSE) passthrough** with mid-stream token accounting.
- **Budget alerts:** block or warn when a key passes `monthly_budget_usd`.
- **Load test** with `k6` or `vegeta`; put the p99 numbers in the README (great résumé fuel).

---

## 5. Public API (gateway)

```
GET  /healthz                         -> 200
POST /v1/chat/completions             -> proxied LLM call (OpenAI-compatible shape)
POST /admin/keys                      -> create virtual key (returns raw key once)
GET  /admin/keys                      -> list keys (no raw keys)
GET  /admin/stats/overview            -> totals: spend, reqs, cache-hit %, latency p50/p95
GET  /admin/stats/timeseries?range=   -> spend & requests bucketed by time
GET  /admin/stats/by-model            -> cost/tokens grouped by model
GET  /admin/stats/by-key              -> per-key usage vs budget
```
Admin endpoints are protected by a separate `ADMIN_TOKEN` (env).

---

## 6. Repo layout

```
ai-gateway/
├── cmd/gateway/main.go         # entrypoint: wire config, redis, pg, http server
├── internal/
│   ├── config/                 # env + pricing.yaml loader
│   ├── proxy/                  # the request lifecycle handler
│   ├── providers/              # provider interface + openai.go + anthropic.go
│   ├── cache/                  # redis cache (exact; phase6 semantic)
│   ├── ratelimit/              # redis token bucket
│   ├── keys/                   # virtual key auth + admin
│   ├── store/                  # postgres: request_logs, aggregates
│   ├── metrics/                # async log worker + redis live counters
│   └── api/                    # admin/stats handlers
├── migrations/                 # *.sql
├── pricing.yaml                # per-model $/1K tokens (in & out)
├── dashboard/                  # Next.js 14 app (TypeScript)
├── docker-compose.yml
├── .env.example
├── Makefile
└── README.md
```

---

## 7. Config (`.env.example`)

```
# Gateway
PORT=8080
ADMIN_TOKEN=change-me
# Providers (real keys, server-side only)
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
# Infra
REDIS_URL=redis://localhost:6379
DATABASE_URL=postgres://gw:gw@localhost:5432/aigateway?sslmode=disable
# Cache
CACHE_TTL_SECONDS=3600
```

`pricing.yaml` (example shape):
```yaml
models:
  gpt-4o-mini:      { provider: openai,    in_per_1k: 0.00015, out_per_1k: 0.0006 }
  claude-haiku-4-5: { provider: anthropic, in_per_1k: 0.0008,  out_per_1k: 0.004 }
```
> Keep pricing in config, not code — note in the README that figures must be verified against current provider pricing.

---

## 8. Quality bar / acceptance

- `make test` passes; core logic (cost calc, cache key, token bucket, provider parsing) has unit tests.
- Proxy adds **< 5 ms** overhead on a cache miss (excluding provider time); cache hits return in **< 10 ms**.
- No secret ever logged or returned. Virtual keys stored as hashes only.
- Response path never blocks on Postgres/metrics writes.
- `README.md` has: what it is, architecture diagram, `docker compose up` quickstart, a screenshot/GIF of the dashboard, and (if Phase 6 load test done) the p99 numbers.

---

## 9. Out of scope (don't build)

Billing/payments, user signup/SSO, multi-tenant orgs, a hosted control plane, fine-tuning. This is a focused, self-hostable gateway — keep it sharp.

---

## 10. First message to Claude Code

> "Read `BUILD.md`. Start with **Phase 0** only: scaffold the repo per section 6, write
> `docker-compose.yml` (redis + postgres), `.env.example`, a `Makefile`, the Go config
> loader with `slog` + graceful shutdown, DB migrations for `request_logs` and `api_keys`,
> and a `GET /healthz` endpoint. Make `docker compose up` work, then stop and show me."
