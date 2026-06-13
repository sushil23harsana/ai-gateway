# Phase 0 — Scaffold

**Goal (from BUILD.md):** stand up the project skeleton so `docker compose up`
starts Redis + Postgres and the gateway serves `GET /healthz` → 200.

**Status:** ✅ done & verified.

---

## What was built

- A Go module (`github.com/sushil23harsana/ai-gateway`) with the repo layout from BUILD.md §6.
- A minimal HTTP server (config + structured logging + graceful shutdown + healthcheck).
- A tiny SQL migration runner and the initial schema.
- Docker Compose for the full local stack, plus a Dockerfile, Makefile, and `.env.example`.

## Files

| File | What it does |
|------|--------------|
| [go.mod](../go.mod) | Module definition. Deps: `pgx/v5` (Postgres), `yaml.v3` (pricing). |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Server entrypoint: loads config, sets up `slog` JSON logging, serves `GET /healthz`, graceful shutdown on SIGINT/SIGTERM. |
| [cmd/migrate/main.go](../cmd/migrate/main.go) | Forward-only SQL migration runner. Applies `migrations/*.up.sql` not yet recorded in `schema_migrations`, each in its own transaction. No migration framework. |
| [internal/config/config.go](../internal/config/config.go) | 12-factor env loader + `pricing.yaml` parser, with a per-model `Cost()` helper. |
| [migrations/0001_init.up.sql](../migrations/0001_init.up.sql) | Creates `api_keys` and `request_logs` (+ indexes). |
| [pricing.yaml](../pricing.yaml) | Per-model $/1K tokens (in & out). **Figures are placeholders — verify against live pricing.** |
| [docker-compose.yml](../docker-compose.yml) | redis + postgres + one-shot `migrate` + gateway, all health-checked. |
| [Dockerfile](../Dockerfile) | Multi-stage (Go 1.25 → alpine); builds both `gateway` and `migrate` binaries. |
| [Makefile](../Makefile) / [.env.example](../.env.example) | Dev tasks and config template. |

## Data model

`api_keys` (virtual keys) and `request_logs` (append-only request log). See the
[migration](../migrations/0001_init.up.sql) for exact columns. `request_logs.api_key_id`
is nullable — it stays NULL until Phase 2 introduces auth.

## How to run / verify

```bash
docker compose up -d --build
curl -i http://localhost:8080/healthz      # → 200 {"status":"ok"}
docker compose exec postgres psql -U gw -d aigateway -c "\dt"   # api_keys, request_logs, schema_migrations
docker compose down                        # stop (add -v to drop the pg volume)
```

## Key decisions

- **Plain SQL + a tiny runner** instead of `golang-migrate` — keeps the systems project lean (BUILD.md §0).
- **Host ports are configurable** (`REDIS_HOST_PORT`, `POSTGRES_HOST_PORT`) because 6379 was already taken on this machine; the gateway talks to Redis/Postgres over the compose network regardless. We run Redis on host port **6380** locally.
- **`go` directive is 1.25** (`go mod tidy` set it); the Dockerfile build stage matches (`golang:1.25-alpine`).

## Deferred to later phases

Everything beyond the healthcheck: proxying (Phase 1), auth + rate limiting
(Phase 2), caching (Phase 3), multi-provider (Phase 4), dashboard (Phase 5).
The `internal/proxy`, `providers`, `cache`, `ratelimit`, `keys`, `store`,
`metrics`, `api` packages are introduced as their phases land.
