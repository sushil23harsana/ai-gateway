# Phase 3 — Response caching

**Goal (from BUILD.md):** build a stable cache key from the normalized request,
return cached responses on a hit (`cache_hit=true`, **skip the provider call**),
store on miss with a configurable TTL, and add a per-key toggle. Done when:
identical requests show a measurable latency drop and `cache_hit=true` in the logs.

**Status:** ✅ done & verified — a cache hit returned in **4.8 ms vs 1.80 s** for the
miss (~375× faster), at **zero** cost.

---

## What was built

An exact-match response cache in Redis that sits between rate-limiting and the
provider call. On a hit the gateway returns the stored response and never touches
OpenAI. The request pipeline is now: **auth → rate limit → cache → (miss) provider → store**.

## Files

| File | What it does |
|------|--------------|
| [internal/cache/cache.go](../internal/cache/cache.go) | The cache: builds the key (canonicalized request, volatile fields stripped, optionally namespaced per key), `Get`/`Set` in Redis with TTL, size cap. |
| [internal/proxy/handler.go](../internal/proxy/handler.go) | Cache lookup before forwarding; serves hits (`X-Cache: HIT`, logs `cache_hit=true`, cost 0) and stores 2xx misses (`X-Cache: MISS`). |
| [migrations/0002_cache_toggle.up.sql](../migrations/0002_cache_toggle.up.sql) | Adds `api_keys.cache_enabled` (default true) — the per-key toggle. |
| [internal/store/store.go](../internal/store/store.go) | `APIKey.CacheEnabled` threaded through insert/get/list. |
| [internal/keys/keys.go](../internal/keys/keys.go) | `Identity.CacheEnabled` (set at auth) so the proxy knows whether to cache for this key. |
| [internal/api/admin.go](../internal/api/admin.go) | `POST /admin/keys` accepts optional `cache_enabled`; list/responses include it. |
| [internal/config/config.go](../internal/config/config.go) | `CACHE_ENABLED` (global on/off), `CACHE_SCOPE` (`key`/`global`), `CACHE_MAX_BYTES`; `CACHE_TTL_SECONDS` already existed. |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Builds the cache from config and passes it to the proxy. |

Tests: [cache_test.go](../internal/cache/cache_test.go) (key stable across field
order + volatile fields, differs on temperature, scope isolation, rejects
non-JSON), and proxy [hit-skips-upstream / miss-stores](../internal/proxy/handler_test.go).

## How the cache key works

`sha256( provider · [api_key_id if scope=key] · canonical(request) )`, where
`canonical` = the request JSON with volatile fields removed (`stream`,
`stream_options`, `user`, `metadata`, `store`) and re-marshaled with sorted object
keys. So:

- Reordering JSON fields or changing `user`/`stream` → **same** key (correct).
- Changing anything that affects output (`temperature`, `messages`, `tools`,
  `seed`, …) → **different** key (correct), with no hand-maintained field list.

**Scope (default `key`):** the cache is namespaced per virtual key, so one team's
cached completion is never served to another. Set `CACHE_SCOPE=global` for a
single shared cache (higher hit rate, but cross-key sharing) — see the decision
table below.

## What gets cached

- Only **successful (2xx), non-streaming** responses. Errors are never cached;
  streaming requests bypass the cache (token accounting for streams is Phase 6).
- Responses larger than `CACHE_MAX_BYTES` (default 1 MiB) are skipped.
- **On a hit:** `cache_hit=true`, tokens copied from the cached entry, **`cost_usd=0`**
  (no provider charge — this is the $ saved), `X-Cache: HIT`.
- **Determinism caveat:** identical requests return the identical cached body, so
  `temperature>0` becomes effectively deterministic for repeats. Temperature is
  part of the key, so different temperatures don't collide.

## How to run / verify

```bash
docker compose up -d --build   # applies migration 0002 (cache_enabled)

KEY=$(curl -s -X POST localhost:8080/admin/keys -H "Authorization: Bearer change-me" \
  -H "Content-Type: application/json" -d '{"name":"demo","rate_limit_rpm":100}' \
  | grep -o '"key":"[^"]*"' | sed 's/"key":"//; s/"$//')

BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Reply: hi"}],"max_tokens":10,"temperature":0}'
# 1st: X-Cache: MISS (calls OpenAI). 2nd identical: X-Cache: HIT (skips OpenAI).
curl -s -D - -o /dev/null -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" -d "$BODY" | grep -i x-cache
curl -s -D - -o /dev/null -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" -d "$BODY" | grep -i x-cache
```

Verified: `MISS` at 1.80 s → `HIT` at 4.8 ms, and `request_logs` shows the second
row `cache_hit=t, cost_usd=0, latency_ms=0`.

## Key decisions

- **Hash the full request minus a volatile denylist** (not an allowlist of fields). Any future output-affecting field is included automatically — robust by default.
- **Per-key cache scope by default** (tenant isolation); `CACHE_SCOPE=global` opt-in for max hit rate. (Your call on 2026-06-14.)
- **Cost = 0 on a hit**, tokens preserved — `request_logs` becomes a record of cache savings, ready for the Phase 5 dashboard.
- **Store after the client already has the body** so caching adds nothing to perceived latency.
- **Cache stampede** (N identical concurrent misses all calling the provider) is a known limitation; single-flight/locking is a future optimization.

## Deferred to later phases

- **Anthropic + routing/failover** → Phase 4 (cache key already folds in `provider`).
- **Dashboard** surfacing cache-hit rate and $ saved → Phase 5.
- **Semantic caching** (near-duplicate prompts, not just identical) → Phase 6.
- **Streaming response caching** + single-flight → Phase 6.
