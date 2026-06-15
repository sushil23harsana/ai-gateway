# Phase 2 — Virtual keys + rate limiting

**Goal (from BUILD.md):** issue gateway *virtual keys* (store only their hash),
authenticate `Authorization: Bearer <virtual-key>` on the proxy, and rate-limit
per key with a Redis token bucket. Done when: unknown key → 401, exceeding the
limit → 429 with `Retry-After`, valid key within limit → proxied.

**Status:** ✅ done & verified end-to-end.

---

## What was built

A control plane in front of the Phase 1 proxy:

- **Admin API** to mint and list virtual keys (guarded by a separate `ADMIN_TOKEN`).
- **Auth middleware** that validates the Bearer virtual key against the stored hash.
- **Redis token-bucket limiter** keyed per `api_key_id`, enforced as middleware.
- The proxy now stamps each `request_logs` row with the authenticated `api_key_id`.

Request pipeline for the proxy is now: **auth → rate limit → proxy**.

## Files

| File | What it does |
|------|--------------|
| [internal/keys/keys.go](../internal/keys/keys.go) | Key generation (`sk-gw-` + random) + SHA-256 hashing; the auth middleware (401 unknown, 403 disabled); `Identity` carried on request context. |
| [internal/ratelimit/ratelimit.go](../internal/ratelimit/ratelimit.go) | Redis token-bucket limiter (atomic Lua refill+take) + the enforcing middleware (429 + `Retry-After`, fails open on Redis error). |
| [internal/api/admin.go](../internal/api/admin.go) | `POST /admin/keys` (mint, returns raw key once), `GET /admin/keys` (list, no secrets), and the `ADMIN_TOKEN` guard (constant-time compare; 503 if unset). |
| [internal/store/store.go](../internal/store/store.go) | Added `InsertAPIKey`, `GetAPIKeyByHash`, `ListAPIKeys`. |
| [internal/proxy/handler.go](../internal/proxy/handler.go) | Reads `keys.Identity` from context → sets `request_logs.api_key_id`. |
| [cmd/gateway/main.go](../cmd/gateway/main.go) | Adds a Redis client (required), wires auth + rate-limit middleware onto the proxy route, and registers the admin routes. |

Tests: [keys_test.go](../internal/keys/keys_test.go) (gen/hash + 401/403/200 middleware),
[ratelimit_test.go](../internal/ratelimit/ratelimit_test.go) (429 + Retry-After, fail-open),
[admin_test.go](../internal/api/admin_test.go) (raw-once, hash-not-raw stored, list hides hash, token guard).

## How the pieces work

**Virtual keys.** `keys.Generate()` returns `sk-gw-<random>` and its SHA-256. Only
the hash is stored (`api_keys.key_hash`); the raw key is returned by the admin
endpoint exactly once. On each request the presented key is hashed and looked up
by hash — the raw key is never persisted or logged.

**Token bucket (Redis).** Per key, a bucket holds up to `rate_limit_rpm` tokens
and refills at `rpm/60` tokens/sec; each request costs one. Refill + take happen
in one Lua script (`ratelimit:{api_key_id}`) so it's atomic under concurrency.
When empty, the middleware returns `429` with `Retry-After` = seconds until one
token is available. If Redis is unreachable, it **fails open** (logs + allows) so
an infra blip doesn't take down the proxy.

**Admin auth.** `/admin/*` is guarded by `ADMIN_TOKEN` (env), compared in constant
time. This is separate from virtual keys — admins manage keys; callers use keys.

## Public API added

```
POST /admin/keys   {"name": "...", "rate_limit_rpm": 120, "monthly_budget_usd": 50}
                   → 201 {"id","name","key":"sk-gw-...","rate_limit_rpm", "warning"}   (raw key once)
GET  /admin/keys   → 200 {"keys":[{"id","name","rate_limit_rpm","monthly_budget_usd","disabled","created_at"}]}
```

## How to run / verify

```bash
docker compose up -d --build
ADMIN=change-me   # = ADMIN_TOKEN from .env

# Mint a key (rpm=1 to demo the limit)
curl -s -X POST localhost:8080/admin/keys -H "Authorization: Bearer $ADMIN" \
  -H "Content-Type: application/json" -d '{"name":"demo","rate_limit_rpm":1}'
KEY=sk-gw-...   # copy the "key" from the response (shown once)

BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Reply: ok"}],"max_tokens":10}'
curl -s -o /dev/null -w "%{http_code}\n" -X POST localhost:8080/v1/chat/completions -d "$BODY"                                   # 401 (no key)
curl -s -o /dev/null -w "%{http_code}\n" -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" -d "$BODY"   # 200
curl -s -D - -o /dev/null              -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" -d "$BODY"     # 429 + Retry-After
```

Verified results: `401 / 401 / 200 / 429 (Retry-After: 58) / admin-wrong-token 401`,
and the successful request logged with `api_key_id` joining to the key name.

`go test ./...` covers auth, rate-limit, and admin behavior without Redis/DB.

## Key decisions

- **Store only the hash.** Raw keys are shown once and never persisted (BUILD.md §8). Lookup is by hash.
- **Token bucket in one Lua script** for atomic refill+take — no read-modify-write race across concurrent requests (BUILD.md §2).
- **Rate limiter fails open** on Redis errors. Protecting providers matters, but a Redis hiccup shouldn't 500 every request; the error is logged.
- **Admin token is separate from virtual keys** and compared in constant time; admin API is disabled (503) if `ADMIN_TOKEN` is unset.
- **`monthly_budget_usd` is stored here; enforcement landed later** as a Phase 6 stretch item (a Redis spend counter + a 402 budget middleware — see [phase-6.md](phase-6.md)).

## Deferred to later phases

- **Response caching** (`cache_hit` still always false) → Phase 3.
- **Budget enforcement** (the column exists; nothing checks it yet) → Phase 6.
- **Key disable/revoke endpoint** — the `disabled` flag is honored on auth, but there's no admin endpoint to toggle it yet (easy add when needed).
- **Anthropic + routing/failover** → Phase 4.
