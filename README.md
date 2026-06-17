# Janus

> *Janus — the Roman god of gateways and transitions, with two faces. One
> gateway between two providers.*

**Janus** is a self-hostable **LLM gateway / proxy** that sits between your
applications and LLM providers (OpenAI, Anthropic). It's a drop-in replacement
for the OpenAI API that adds the control plane teams are missing: **virtual API
keys, rate limiting, exact + semantic caching, cost & token analytics, monthly
budgets, and multi-provider routing with retry/failover** — all behind a
real-time dashboard.

Point any OpenAI-compatible client (the OpenAI SDK, LangChain, LlamaIndex, …) at
Janus instead of `api.openai.com`, and you get cost tracking, caching, and key
management for free — without changing your app code.

> 📖 **Full guide, live demo & examples:** <https://sushil23harsana.github.io/ai-gateway/>
>
> **Author:** Sushil Harsana · **Module:** `github.com/sushil23harsana/ai-gateway`

---

## What you get

| | Feature | What it does |
|---|---------|--------------|
| 🔑 | **Virtual API keys** | Mint scoped keys per team/service. Stored as SHA-256 hashes; raw key shown once. |
| ⚡ | **Rate limiting** | Redis token-bucket per key. `429` + `Retry-After` on breach. |
| 🗄️ | **Response caching** | Exact-match Redis cache (1.8s → ~4ms) + optional pgvector semantic cache for near-duplicate prompts. |
| 💸 | **Cost & token analytics** | Per-model pricing config; tokens and USD tracked on every request, including SSE streams. |
| 🔄 | **Multi-provider routing** | Native OpenAI + Anthropic. Routed by model name, with automatic cross-provider failover. |
| 🛡️ | **Resilience** | Bounded retry with backoff + a per-provider circuit breaker (Closed → Open → Half-open). |
| 💰 | **Monthly budgets** | Per-key spend caps enforced at the gateway (`402` when exceeded). |
| 📊 | **Real-time dashboard** | Next.js console: spend, cache hit-rate, latency, per-key/model/provider breakdowns, live req/min, key management. |
| ✅ | **Health & readiness** | `GET /healthz` (liveness) and `GET /readyz` (pings Redis + Postgres) for orchestrators. |

---

## How a request flows

Every call to `POST /v1/chat/completions` passes this pipeline:

1. **Auth** — Bearer virtual key is SHA-256 hashed and matched against `api_keys`. Unknown → `401`.
2. **Budget gate** — month's spend (Redis) vs the key's `monthly_budget_usd`. Over budget → `402`.
3. **Rate limit** — Redis token-bucket per key. Exceeded → `429` + `Retry-After`.
4. **Cache lookup** — exact Redis match (`X-Cache: HIT`), then semantic pgvector match (`X-Cache: SEMANTIC`). Hits skip the provider and cost $0.
5. **Provider routing + resilience** — model name selects the provider; transient failures are retried; a tripped circuit breaker triggers failover.
6. **Async logging** — tokens, cost, latency, model, cache status written via a buffered channel + worker pool, off the response path.

---

## Architecture

```
                ┌────────────────────────────────────────────────┐
   client       │                 JANUS GATEWAY (Go)              │
 (virtual key)  │                                                 │
  ───────────►  │  1 auth   2 budget   3 rate limit (Redis TB)    │
                │  4 cache lookup (Redis + pgvector)              │     ┌──────────┐
                │  5 router + retry/breaker ──────────────────────┼───► │ OpenAI / │
                │  6 parse usage → cost → async log               │ ◄───┤ Anthropic│
                │  7 stream / return response                     │     └──────────┘
                └───────┬───────────────────────────┬─────────────┘
                        │ writes                     │ reads
                        ▼                            ▼
                   PostgreSQL                     Redis  ◄──── /admin/stats
            (request_logs, api_keys,        (counters, cache,
                  pgvector cache)            spend tracking)
```

The proxy path is non-blocking: logging and metric writes go through a buffered
channel + worker pool, never inline on the response.

---

## Quickstart

### Run from pre-built images (just Docker — no clone, no build)

Images are published to GHCR. Grab the prod compose file + an env template:

```bash
curl -O https://raw.githubusercontent.com/sushil23harsana/ai-gateway/main/docker-compose.prod.yml
curl -o .env https://raw.githubusercontent.com/sushil23harsana/ai-gateway/main/.env.example
# edit .env — add OPENAI_API_KEY and/or ANTHROPIC_API_KEY, set a strong ADMIN_TOKEN

docker compose -f docker-compose.prod.yml up -d
```

This pulls `ghcr.io/sushil23harsana/ai-gateway` (+ the dashboard image), runs DB
migrations once, and starts everything. Pin a version with
`GATEWAY_IMAGE=ghcr.io/sushil23harsana/ai-gateway:vX.Y.Z`; the default tracks `latest`.

### Build from source (developers)

```bash
git clone https://github.com/sushil23harsana/ai-gateway.git && cd ai-gateway
cp .env.example .env        # then fill in provider key(s)
docker compose up -d --build
```

Either path starts Redis, Postgres, the gateway (`:8080`), and the dashboard
(`:3000`). Verify health:

```bash
curl http://localhost:8080/healthz     # → {"status":"ok"}
curl http://localhost:8080/readyz      # → {"status":"ready","checks":{...}}
```

Open the dashboard at <http://localhost:3000>. Tear down with
`docker compose down` (add `-v` to drop the Postgres volume).

### Mint a virtual key

From the dashboard's **API Keys** page, or via the admin API:

```bash
curl -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-app","rate_limit_rpm":60,"monthly_budget_usd":10}'
```

The response includes the raw key (`sk-gw-…`) — save it, it's shown only once.

---

## Supported models

Pass any of these as the `model` field — Janus routes to the right provider
automatically. Prices live in [`pricing.yaml`](pricing.yaml) and can change
without a rebuild.

| Model | Provider | $/1M tokens (in / out) | Note |
|-------|----------|------------------------|------|
| `gpt-4o-mini` | OpenAI | $0.15 / $0.60 | Cheapest — ideal for testing |
| `gpt-4o` | OpenAI | $2.50 / $10.00 | |
| `claude-haiku-4-5` | Anthropic | $1.00 / $5.00 | Cheapest Anthropic |
| `claude-sonnet-4-6` | Anthropic | $3.00 / $15.00 | |
| `claude-opus-4-8` | Anthropic | $5.00 / $25.00 | Most capable |

OpenAI models need `OPENAI_API_KEY`; Anthropic models need `ANTHROPIC_API_KEY`.

---

## Usage

The gateway is a drop-in OpenAI API — same endpoint shape, virtual key as the
Bearer token. Switch providers by changing only the `model` name.

### curl

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-your-virtual-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello!"}]}'
```

### Python (openai SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="sk-gw-your-virtual-key",
)

# OpenAI
print(client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Explain backpressure in 2 sentences."}],
).choices[0].message.content)

# Anthropic — same client, just change the model name
print(client.chat.completions.create(
    model="claude-sonnet-4-6",
    messages=[{"role": "user", "content": "Explain backpressure in 2 sentences."}],
).choices[0].message.content)
```

### Streaming (Python)

```python
stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Write a haiku about distributed systems."}],
    stream=True,
)
for chunk in stream:
    delta = chunk.choices[0].delta.content
    if delta:
        print(delta, end="", flush=True)
```

### LangChain (Python)

Any LangChain integration that accepts a `base_url` works unchanged — Janus
speaks the OpenAI format.

```python
# pip install langchain-openai
from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage

llm = ChatOpenAI(
    model="gpt-4o-mini",
    openai_api_key="sk-gw-your-virtual-key",
    openai_api_base="http://localhost:8080/v1",   # ← point at Janus
)
print(llm.invoke([HumanMessage(content="What is a circuit breaker in software?")]).content)
```

LCEL chain (the second identical call returns from the Janus cache, `$0`):

```python
from langchain_openai import ChatOpenAI
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser

llm = ChatOpenAI(model="gpt-4o-mini", temperature=0,
                 openai_api_key="sk-gw-your-virtual-key",
                 openai_api_base="http://localhost:8080/v1")
prompt = ChatPromptTemplate.from_messages([
    ("system", "You are a concise technical writer."),
    ("human",  "Summarise {topic} in one paragraph."),
])
chain = prompt | llm | StrOutputParser()
print(chain.invoke({"topic": "Redis Streams"}))
print(chain.invoke({"topic": "Redis Streams"}))   # → cache hit
```

### LangChain (JavaScript / TypeScript)

```typescript
// npm install @langchain/openai @langchain/core
import { ChatOpenAI } from "@langchain/openai";
import { HumanMessage } from "@langchain/core/messages";

const llm = new ChatOpenAI({
  modelName:    "gpt-4o-mini",
  openAIApiKey: "sk-gw-your-virtual-key",
  configuration: { baseURL: "http://localhost:8080/v1" },
});
const res = await llm.invoke([new HumanMessage("What is the capital of France?")]);
console.log(res.content);
```

> Any OpenAI-compatible client works the same way — openai-python, openai-node,
> LangChain, LlamaIndex, AutoGen, CrewAI — just point `base_url` at
> `http://<gateway-host>:8080/v1` and use a virtual key.

---

## Dashboard

A Next.js console at <http://localhost:3000> with five pages:

- **Overview** — spend (month/today), requests + tokens, cache hit-rate, P95/P50 latency, spend-over-time chart, by-provider / by-model breakdowns, live req/min, per-key spend vs budget.
- **Routing** — model → provider mapping and failover configuration.
- **Live Logs** — individual requests as they happen.
- **Caches** — exact + semantic hit-rate, stored embeddings, recent semantic entries.
- **API Keys** — create, edit, disable, and delete virtual keys (budget, RPM, cache toggle).

The admin token is kept server-side, so the browser never sees it.

---

## Configuration

All config is via environment variables (12-factor); see
[.env.example](.env.example). Pricing lives in [pricing.yaml](pricing.yaml).

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | — | OpenAI secret (server-side only). |
| `ANTHROPIC_API_KEY` | — | Anthropic secret (server-side only). |
| `ADMIN_TOKEN` | `change-me` | Guards `/admin/*`. Set a strong value for production. |
| `PORT` | `8080` | Gateway listen port. |
| `DEFAULT_PROVIDER` | `openai` | Provider when not determinable by model name. |
| `FAILOVER_ENABLED` | `true` | Auto-failover to the other provider on 5xx / timeout. |
| `CACHE_TTL_SECONDS` | `3600` | Exact-match cache TTL. |
| `CACHE_SCOPE` | `key` | `key` (per-virtual-key) or `global` (shared). |
| `BUDGET_ENFORCED` | `true` | Enforce per-key monthly budget caps. |
| `RETRY_MAX_ATTEMPTS` | `3` | Attempts per provider (1 = no retry). |
| `BREAKER_ENABLED` | `true` | Per-provider circuit breaker. |
| `BREAKER_THRESHOLD` | `5` | Consecutive failures that open a breaker. |
| `SEMANTIC_CACHE_ENABLED` | `false` | pgvector semantic cache (needs `OPENAI_API_KEY` for embeddings). |
| `SEMANTIC_THRESHOLD` | `0.25` | Max cosine distance for a hit (smaller = stricter). |

---

## Security

- Provider keys are **server-side only** — never logged or returned.
- Virtual keys are stored as **SHA-256 hashes**; the raw key is shown once.
- `/admin/*` and stats are guarded by `ADMIN_TOKEN` (constant-time compare); the
  dashboard holds the token server-side.
- **Writes are blocked while `ADMIN_TOKEN` is the default `change-me`.**
- Docker ports bind to `127.0.0.1` only (loopback) by default.

**Before exposing beyond localhost:** set a strong `ADMIN_TOKEN`
(`openssl rand -hex 32`), and put the control plane behind an authenticating
reverse proxy with TLS. See [docs/security.md](docs/security.md).

---

## API reference

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/v1/chat/completions` | The proxy. OpenAI-compatible. Auth via virtual key. |
| `GET` | `/healthz` | Liveness (process up). |
| `GET` | `/readyz` | Readiness (pings Redis + Postgres). |
| `POST` | `/admin/keys` | Mint a key: `name`, `rate_limit_rpm`, `monthly_budget_usd`, `cache_enabled`. |
| `GET` | `/admin/keys` | List keys. |
| `PATCH` | `/admin/keys/{id}` | Update a key (rename, rpm, budget, cache, disable). |
| `DELETE` | `/admin/keys/{id}` | Delete a key. |
| `GET` | `/admin/stats/*` | overview · timeseries · by-model · by-provider · by-key · recent · cache · live. |

**Proxy status codes:** `200` ok · `401` invalid key · `402` budget exceeded ·
`429` rate limited (`Retry-After`) · `502` upstream error · `503` breaker open.
**Cache header:** `X-Cache: MISS | HIT | SEMANTIC`.

---

## Developer commands

A `Makefile` wraps the common tasks (`make help` lists them):

| Make target | What it does | Raw command |
|-------------|--------------|-------------|
| `make dev` / `make up` | Bring up the full stack | `docker compose up -d --build` |
| `make down` | Stop everything | `docker compose down` |
| `make migrate` | Apply DB migrations | `go run ./cmd/migrate` |
| `make test` | Run unit tests | `go test ./...` |
| `make build` | Build the binaries | `go build ./cmd/...` |
| `make run` | Run the gateway locally | `go run ./cmd/gateway` |

> **Windows:** `make` isn't installed by default — use the raw commands in the
> right-hand column.

---

## Project layout

```
ai-gateway/
├── cmd/gateway/      # HTTP server entrypoint
├── cmd/migrate/      # forward-only SQL migration runner
├── internal/
│   ├── proxy/        # the request pipeline
│   ├── providers/    # OpenAI + Anthropic adapters, router
│   ├── cache/        # exact (Redis) + semantic (pgvector) caches
│   ├── ratelimit/    # Redis token-bucket
│   ├── budget/       # per-key monthly spend caps
│   ├── resilience/   # retry + circuit breaker
│   ├── keys/         # virtual-key auth
│   ├── api/          # admin + stats endpoints
│   ├── metrics/      # async request logger, live counter
│   ├── store/        # PostgreSQL access
│   └── config/       # env + pricing.yaml loader
├── dashboard/        # Next.js 14 console
├── migrations/       # *.up.sql / *.down.sql
├── pricing.yaml      # per-model $/1K tokens
├── docker-compose.yml          # build-from-source
├── docker-compose.prod.yml     # run pre-built GHCR images
└── docs/             # per-area docs (security, resilience, deploy, …)
```

Built by [Sushil Harsana](https://github.com/sushil23harsana). The Go module
path keeps the original `ai-gateway` slug; the product is **Janus**.
