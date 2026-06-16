# Janus

> *Janus — the Roman god of gateways and transitions, with two faces. One
> gateway between two providers.*

**Janus** is a self-hostable **LLM gateway / proxy** that sits between your
applications and LLM providers (OpenAI, Anthropic) and gives teams the control
plane they're missing: **cost & token analytics, exact + semantic caching, rate
limiting, virtual API keys, and multi-provider routing/failover** — all behind a
real-time dashboard.

Think *Helicone / LiteLLM, built from scratch* — a production-shaped systems
project in Go.

> **Author:** Sushil Harsana · **Module:** `github.com/sushil23harsana/ai-gateway`
> (the Go module path keeps the original `ai-gateway` slug; the product is **Janus**)
>
> **Live site:** <https://sushil23harsana.github.io/ai-gateway/>

---

## Status

Built in independently runnable phases (see [BUILD.md](BUILD.md)). Per-phase
notes — what each phase actually changed — live in [docs/](docs/README.md).

- [x] **Phase 0 — Scaffold:** repo layout, Docker Compose (redis + postgres),
      config loader (`slog`, graceful shutdown), DB migrations, `GET /healthz`.
- [x] **Phase 1 — Pass-through proxy + observability:** `POST /v1/chat/completions`
      → OpenAI (key injected server-side), async `request_logs` with tokens, cost, latency.
- [x] **Phase 2 — Virtual keys + rate limiting:** admin API mints keys (hash stored,
      raw shown once), Bearer auth on the proxy, Redis token-bucket limiter (429 + `Retry-After`).
- [x] **Phase 3 — Response caching:** Redis exact-match cache (per-key scope, TTL,
      per-key toggle); hits skip the provider (`cache_hit=true`, cost 0) — verified 1.8s → 4.8ms.
- [x] **Phase 4 — Multi-provider + routing/failover:** `Provider` interface, native
      Anthropic (OpenAI⇄Messages translation), model routing, 5xx/timeout failover.
      *(Live-verified 2026-06-16: real Claude completion + correct cost, and forced-outage failover OpenAI→Anthropic.)*
- [x] **Phase 5 — Next.js dashboard:** `/admin/stats/*` Go API + Redis live counter,
      and a Next.js 14 + Recharts console (overview tiles, spend/cost charts, per-key
      budgets, live req/min) at `:3000` — renders real traffic.
- [x] **Phase 6 — Semantic caching:** pgvector + OpenAI embeddings serve near-duplicate
      prompts (`X-Cache: SEMANTIC`), with a calibrated threshold that rejects different-answer
      lookalikes. *(Stretch since done: per-key budget enforcement + a k6 cache-hit load test. SSE token accounting still remains.)*
- [x] **Phase 7 — Control plane:** create/edit/disable/delete virtual keys from the dashboard
      via token-safe, same-origin write routes (admin token never reaches the browser) + a
      gateway write-guard; secure-by-default packaging (loopback-only ports). See [docs/security.md](docs/security.md).

---

## Architecture

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
                 (request_logs)               (counters, cache)
```

The proxy path is kept non-blocking: logging and metric writes go through a
buffered channel + worker pool (Phase 1), never inline on the response.

---

## Quickstart

```bash
cp .env.example .env        # then fill in provider keys as you reach Phase 1+
docker compose up -d --build
```

This starts Redis, Postgres, the gateway, and the dashboard (and runs DB
migrations once). Verify the health endpoint:

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
# {"status":"ok"}
```

Then open the **dashboard** at <http://localhost:3000> for cost, cache, latency,
and per-key analytics.

Tear down with `docker compose down` (add `-v` to also drop the Postgres volume).

### Running locally (without Docker for the app)

You still need Redis and Postgres. Start just the infra, then run the gateway on
the host:

```bash
docker compose up -d redis postgres
go run ./cmd/migrate     # apply migrations
go run ./cmd/gateway     # serve on :8080
```

> **Deploying beyond your laptop?** See [docs/deploy.md](docs/deploy.md) and read
> [docs/security.md](docs/security.md) first.

---

## Dashboard

A Next.js console at <http://localhost:3000>: overview tiles (spend, cache
hit-rate, latency), spend/cost charts, per-provider and per-model breakdowns,
live requests/min, recent requests, and an **API Keys** page to create, edit,
disable, and delete virtual keys — with the admin token kept server-side
(see [docs/security.md](docs/security.md)).

> 📸 **Screenshot:** capture your running dashboard, save it to
> `docs/assets/dashboard.png`, and add `![Janus dashboard](docs/assets/dashboard.png)`
> here. It needs your own traffic, so it's a quick step on your machine — see
> [docs/assets/README.md](docs/assets/README.md). For a visual right now, the
> [live site](https://sushil23harsana.github.io/ai-gateway/) shows the product.

---

## Developer commands

A `Makefile` wraps the common tasks (`make help` lists them):

| Make target  | What it does                                   | Raw command |
|--------------|------------------------------------------------|-------------|
| `make dev` / `make up` | Bring up the full stack            | `docker compose up -d --build` |
| `make down`  | Stop everything                                | `docker compose down` |
| `make migrate` | Apply DB migrations                          | `go run ./cmd/migrate` |
| `make test`  | Run unit tests                                 | `go test ./...` |
| `make build` | Build `gateway` + `migrate` binaries           | `go build ./cmd/...` |
| `make run`   | Run the gateway locally                        | `go run ./cmd/gateway` |

> **Windows note:** `make` isn't installed by default. Use the raw commands in
> the right-hand column (e.g. `docker compose up -d --build`, `go test ./...`).

---

## Project layout

```
ai-gateway/
├── cmd/
│   ├── gateway/      # HTTP server entrypoint (config, slog, graceful shutdown, /healthz)
│   └── migrate/      # tiny forward-only SQL migration runner
├── internal/
│   └── config/       # env + pricing.yaml loader
├── migrations/       # *.up.sql / *.down.sql
├── pricing.yaml      # per-model $/1K tokens (verify against current pricing!)
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── Makefile
└── BUILD.md          # full build spec
```

Later phases fill in `internal/proxy`, `internal/providers`, `internal/cache`,
`internal/ratelimit`, `internal/keys`, `internal/store`, `internal/metrics`,
`internal/api`, and `dashboard/`.

---

## Configuration

All config comes from the environment (12-factor); see [.env.example](.env.example).
Model pricing lives in [pricing.yaml](pricing.yaml) so it can change without a
rebuild. Secrets (provider keys) are server-side only and are never logged or
returned to callers; virtual keys are stored as SHA-256 hashes.
