# Phase 1 — Pass-through proxy + observability

**Goal (from BUILD.md):** `POST /v1/chat/completions` proxies to OpenAI
(injecting the real key server-side), measures latency, parses `usage`, computes
cost from `pricing.yaml`, and writes a `request_logs` row **asynchronously** —
without ever blocking the response. Done when a curl through the gateway returns
OpenAI's response *and* a correct row lands in `request_logs`.

**Status:** ✅ done & verified end-to-end against the real OpenAI API.

---

## What was built

The core request lifecycle for a single provider (OpenAI). A request comes in,
gets forwarded to OpenAI with the real key attached server-side, the response is
relayed back verbatim, and a log row (tokens, cost, latency, status) is written
on a background worker pool.

## Files

| File | What it does |
|------|--------------|
| [internal/proxy/handler.go](../internal/proxy/handler.go) | The `POST /v1/chat/completions` handler: reads the body, forwards to OpenAI, relays the response, parses usage, computes cost, enqueues the log. Handles non-streaming fully; streams are passed through (token accounting deferred to Phase 6). |
| [internal/providers/openai.go](../internal/providers/openai.go) | OpenAI specifics: endpoint URL, holds the API key, `ParseUsage()` to read `prompt_tokens`/`completion_tokens`/`cached_tokens` + the resolved model from the response. |
| [internal/store/store.go](../internal/store/store.go) | Postgres layer: a `pgxpool` connection + `InsertRequestLog()`. Thin — no HTTP/provider knowledge. |
| [internal/metrics/logger.go](../internal/metrics/logger.go) | Async log writer: a buffered channel + worker pool. `Enqueue()` never blocks (drops + warns if the buffer is full); `Stop()` drains on shutdown. |
| [internal/config/config.go](../internal/config/config.go) | Added `OPENAI_BASE_URL` (default `https://api.openai.com/v1`) and `UPSTREAM_TIMEOUT_SECONDS` (default 120). |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Wires Postgres → async logger → proxy handler; registers the route; drains the logger before closing the DB on shutdown. |

Tests: [providers/openai_test.go](../internal/providers/openai_test.go),
[proxy/handler_test.go](../internal/proxy/handler_test.go) (uses an `httptest`
mock OpenAI — no network or DB needed).

## Request flow

```
client ──POST /v1/chat/completions──► gateway
                                        │ 1. read body, peek model + stream flag
                                        │ 2. build upstream req; set Authorization: Bearer <OPENAI_API_KEY>
                                        ▼
                                      OpenAI  ◄── real key injected here, never from the client
                                        │
                                        │ 3. read response, measure latency
                                        │ 4. relay status + body to client  ◄── response returns now
                                        │ 5. parse usage → compute cost
                                        ▼
                                   metrics.Logger.Enqueue()  (non-blocking)
                                        │  buffered channel
                                        ▼
                                   worker pool ──INSERT──► request_logs (Postgres)
```

Step 4 happens before steps 5–6 don't block it — the client gets its answer
immediately; logging is fire-and-forget on the worker pool.

## How to run / verify

```bash
# .env must have a real OPENAI_API_KEY (gitignored). Then:
docker compose up -d --build

curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Reply: gateway works"}],"max_tokens":20}'

# Confirm the row:
docker compose exec postgres psql -U gw -d aigateway -x -c \
  "SELECT provider, model, status, tokens_in, tokens_out, cost_usd, latency_ms FROM request_logs ORDER BY created_at DESC LIMIT 1;"
```

Verified result: `200` with OpenAI's reply, and a row like
`openai | gpt-4o-mini-2024-07-18 | 200 | 13 | 2 | 0.000003 | 788`.

Unit tests: `go test ./...`.

## Key decisions & a bug caught

- **Logging is off the hot path.** The proxy `Enqueue()`s onto a buffered channel drained by a worker pool; the response never waits on Postgres. If the buffer fills, entries are dropped with a warning rather than back-pressuring requests (BUILD.md §2).
- **The real key is injected server-side only.** The gateway builds a fresh upstream request and sets `Authorization` itself; it never forwards the client's auth header. (Virtual keys arrive in Phase 2.)
- **Cost pricing falls back from snapshot → requested alias.** OpenAI resolves `gpt-4o-mini` to a dated snapshot (`gpt-4o-mini-2024-07-18`) that isn't a key in `pricing.yaml`. The first live test logged `cost_usd = 0` because of this. Fix: price by the resolved model, and if that snapshot isn't priced, fall back to the model the client requested. Covered by a regression test.
- **`WriteTimeout` is disabled (0) on the server.** LLM responses (and future SSE streams) routinely run longer than a normal HTTP response; the upstream HTTP client has its own timeout, and `ReadHeaderTimeout` still guards against slow-loris.
- **`cached_tokens` is parsed but not yet priced.** We read OpenAI's prompt-cache hit count now; pricing it at the discounted rate and surfacing cache savings on the dashboard is a later analytics enhancement.

## Deferred to later phases

- **Auth / virtual keys** → Phase 2 (`request_logs.api_key_id` stays NULL until then).
- **Rate limiting** → Phase 2.
- **Response caching** (`cache_hit` is always false) → Phase 3.
- **Streaming token accounting** — streams pass through, but tokens log as 0 → Phase 6.
- **Anthropic + routing/failover** → Phase 4.
