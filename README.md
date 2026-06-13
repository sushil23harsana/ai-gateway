# AI Gateway

A self-hostable **LLM gateway / proxy** that sits between your applications and
LLM providers (OpenAI, Anthropic) and gives teams the control plane they're
missing: **cost & token analytics, semantic caching, rate limiting, virtual API
keys, and multi-provider failover** — all behind a real-time dashboard.

Think *Helicone / LiteLLM, built from scratch* — a production-shaped systems
project in Go.

> **Author:** Sushil Harsana · **Module:** `github.com/sushil23harsana/ai-gateway`

---

## Status

Built in independently runnable phases (see [BUILD.md](BUILD.md)).

- [x] **Phase 0 — Scaffold:** repo layout, Docker Compose (redis + postgres),
      config loader (`slog`, graceful shutdown), DB migrations, `GET /healthz`.
- [ ] **Phase 1 — Pass-through proxy + observability** (async request logging).
- [ ] **Phase 2 — Virtual keys + rate limiting** (Redis token bucket).
- [ ] **Phase 3 — Response caching.**
- [ ] **Phase 4 — Multi-provider + routing/failover.**
- [ ] **Phase 5 — Next.js dashboard.**
- [ ] **Phase 6 — Stretch** (semantic cache, SSE streaming, budget alerts, load test).

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

This starts Redis and Postgres, runs DB migrations once, and starts the gateway.
Verify the health endpoint:

```bash
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
# {"status":"ok"}
```

Tear down with `docker compose down` (add `-v` to also drop the Postgres volume).

### Running locally (without Docker for the app)

You still need Redis and Postgres. Start just the infra, then run the gateway on
the host:

```bash
docker compose up -d redis postgres
go run ./cmd/migrate     # apply migrations
go run ./cmd/gateway     # serve on :8080
```

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
