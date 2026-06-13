# Phase 4 — Multi-provider + routing/failover

**Goal (from BUILD.md):** define a `Provider` interface, implement OpenAI **and**
Anthropic behind it, route by model, and fail over to a fallback provider on
5xx/timeout. Done when: the same gateway request can be served by either
provider, and a forced provider outage fails over cleanly (and is logged).

**Status:** ✅ code-complete & unit-tested; OpenAI path + Anthropic routing
live-verified. ⏳ **Live verification against the real Anthropic API (and the
forced-outage failover demo) is pending an `ANTHROPIC_API_KEY`** — see below.

---

## What was built

The gateway is now provider-agnostic. Clients always speak one shape — OpenAI
`chat/completions` — and each provider translates that to/from its native API.
A router picks the provider by model, and failover retries on a fallback.

## Files

| File | What it does |
|------|--------------|
| [internal/providers/provider.go](../internal/providers/provider.go) | The `Provider` interface (`Name`, `APIKey`, `SupportsStreaming`, `BuildUpstreamRequest`, `TranslateResponse`) + normalized `Usage`. |
| [internal/providers/openai.go](../internal/providers/openai.go) | OpenAI behind the interface — passthrough (the unified shape *is* OpenAI's). |
| [internal/providers/anthropic.go](../internal/providers/anthropic.go) | **The translation layer:** OpenAI `chat/completions` ⇄ Anthropic `/v1/messages`, both ways. Auth via `x-api-key` + `anthropic-version`. |
| [internal/providers/router.go](../internal/providers/router.go) | `ProviderFor(model)`: pricing-table registry → name-prefix heuristic → default provider. |
| [internal/proxy/handler.go](../internal/proxy/handler.go) | Routes, translates, and fails over on 5xx/timeout (both attempts logged). |
| [pricing.yaml](../pricing.yaml) | Anthropic models added (Haiku/Sonnet/Opus); `provider` doubles as the routing registry. |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Builds both providers + router + failover config. |
| [internal/config/config.go](../internal/config/config.go) | `ANTHROPIC_BASE_URL`, `ANTHROPIC_VERSION`, `ANTHROPIC_MAX_TOKENS`, `DEFAULT_PROVIDER`, `FAILOVER_*`; `Pricing.ProviderMap()`. |

Tests: [anthropic_test.go](../internal/providers/anthropic_test.go) (request
translation + system lifting + max_tokens backfill + sampling-strip for Opus 4.8;
response translation to OpenAI shape; error passthrough),
[router_test.go](../internal/providers/router_test.go),
and [handler_test.go](../internal/proxy/handler_test.go) `TestFailoverOnPrimary5xx`
which drives OpenAI(500) → Anthropic(200) through real translation and asserts
two `request_logs` rows + a translated body.

## The translation (the heart of this phase)

**Request — OpenAI → Anthropic:**
- `system`-role messages are lifted to Anthropic's top-level `system` field.
- `max_tokens` is backfilled (`ANTHROPIC_MAX_TOKENS`, default 4096) since Anthropic requires it.
- `temperature`/`top_p` forwarded, except to models that reject them (Opus 4.7/4.8, Fable).
- `stop` → `stop_sequences`. Tools/multimodal are out of scope this phase.

**Response — Anthropic → OpenAI:** `content[].text` → `choices[0].message.content`;
`stop_reason` → `finish_reason`; `usage.input/output_tokens` → `prompt/completion_tokens`.
So a client gets an OpenAI-shaped `chat.completion` even when Anthropic served it.

## Routing & failover

- **Routing:** `pricing.yaml`'s `provider` field is the registry; unknown models
  fall back to a prefix heuristic (`claude*`→anthropic, `gpt*`/`o*`→openai) then
  `DEFAULT_PROVIDER`.
- **Failover:** on a **5xx or transport/timeout** (not 4xx — that's a client
  error), the request is retried on `FAILOVER_PROVIDER` using `FAILOVER_MODEL`.
  The failed primary attempt and the successful fallback are logged as **separate**
  `request_logs` rows, so analytics show the failover.
- **Streaming** to a provider that can't passthrough (Anthropic) returns a clean
  400 for now; OpenAI streaming passthrough still works. (SSE translation → Phase 6.)

## How to run / verify

```bash
docker compose up -d --build
KEY=$(curl -s -X POST localhost:8080/admin/keys -H "Authorization: Bearer change-me" \
  -H "Content-Type: application/json" -d '{"name":"demo","rate_limit_rpm":100}' \
  | grep -o '"key":"[^"]*"' | sed 's/"key":"//; s/"$//')

# Routes to OpenAI:
curl -s -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"max_tokens":10}'

# Routes to Anthropic (needs ANTHROPIC_API_KEY in .env):
curl -s -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" \
  -d '{"model":"claude-haiku-4-5","messages":[{"role":"user","content":"hi"}],"max_tokens":10}'
```

**Verified live (OpenAI key only):** `gpt-4o-mini` returns a 200 OpenAI completion;
`claude-haiku-4-5` routes to Anthropic and returns a clean
`500 "anthropic provider not configured"` (routing + guard correct).

### ⏳ Pending live verification (when `ANTHROPIC_API_KEY` is added)

1. `claude-haiku-4-5` → real Anthropic completion, returned in OpenAI shape, logged with `provider=anthropic` + cost.
2. **Forced-outage failover:** set `OPENAI_BASE_URL` to a dead address + `FAILOVER_PROVIDER=anthropic` / `FAILOVER_MODEL=claude-haiku-4-5`, send a `gpt-4o-mini` request, confirm failover to Anthropic with two `request_logs` rows, then restore.

The failover *logic* is already covered by `TestFailoverOnPrimary5xx`; the pending
step is exercising it against the live Anthropic API.

## Key decisions

- **Unified OpenAI shape on both edges** (LiteLLM-style) — clients never change code to switch providers.
- **`pricing.yaml` is the routing registry** — one source of truth for both cost and routing.
- **Failover only on 5xx/timeout**, not 4xx; cross-provider with a configured fallback model; both attempts logged.
- **Interface expanded beyond BUILD.md's sketch** (`Translate`/`ParseUsage`/`Endpoint`) to also own response translation + auth, since real failover needs a consistent response shape regardless of which provider served it.

## Deferred to later phases

- **Dashboard** (`/admin/stats/*`) → Phase 5.
- **Semantic caching, SSE streaming translation + token accounting, budget alerts, load test** → Phase 6.
- **Tool-use / multimodal translation** for Anthropic (text chat is handled now).
- **Per-route/symmetric failover** (currently a single configured fallback target).
