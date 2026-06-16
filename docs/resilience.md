# Upstream resilience — retry + circuit breaker

Janus wraps every provider call in two independent layers so a provider's bad
minute (or bad hour) doesn't become your outage:

1. **Retry** smooths over *transient* failures within a single request.
2. **Circuit breaker** tracks a provider's health *across* requests and fails
   fast when it's persistently down — which lets failover kick in instantly
   instead of every request paying the full timeout.

These compose with the existing [multi-provider failover](phase-4.md): retry →
breaker → failover, in that order. Code: [internal/resilience/](../internal/resilience/),
wired in [internal/proxy/handler.go](../internal/proxy/handler.go).

---

## Retry

Each upstream attempt is retried on a **transient** failure:

| Outcome | Retried? | Why |
|---------|----------|-----|
| transport error / timeout | ✅ | network blip; a second try often works |
| `5xx` | ✅ | provider-side fault |
| `429` | ✅ | provider rate-limit; back off and retry |
| `2xx` / `3xx` | ❌ | success |
| `4xx` (except 429) | ❌ | **your** request is malformed — retrying won't help |

Backoff is **exponential with full jitter**: `BaseDelay × 2^(n-1)`, capped at
`MaxDelay`, then randomized in `[0, that)`. Jitter spreads retries out so a
provider recovering from a blip doesn't get a synchronized thundering herd.

Retries respect the request context — if the client disconnects or the deadline
passes mid-backoff, the retry loop stops immediately.

> Only the **final** outcome of a request (after its retries) is reported to the
> breaker. A provider that fails once but succeeds on retry is *not* counted as
> unhealthy — so retry and the breaker don't double-penalize a brief blip.

## Circuit breaker (per provider)

A classic three-state breaker, one instance **per provider**:

```
         threshold consecutive failures
 CLOSED ───────────────────────────────► OPEN
   ▲                                       │ cooldown elapses
   │ probe succeeds                        ▼
   └────────────── HALF-OPEN ◄─── (admit up to N trial requests)
                       │
                       └── probe fails ──► OPEN (another cooldown)
```

- **Closed** — requests flow; consecutive failures are counted. A success
  resets the count to zero.
- **Open** — requests are rejected *without touching the upstream* (returns
  `ErrCircuitOpen`, surfaced to the client as `503` if there's no fallback).
  This is the point: stop hammering a dead provider.
- **Half-open** — after the cooldown, a limited number of trial requests are
  admitted. All succeed → **closed** (recovered). Any fails → back to **open**.

Breakers are isolated: OpenAI tripping has no effect on Anthropic's breaker.

## How it interacts with failover

On a primary failure — transport error, `5xx`, **or an open breaker** — the
gateway fails over to `FAILOVER_PROVIDER` (if configured), and the fallback call
gets its *own* retry + breaker treatment. Both the failed primary attempt and
the successful fallback are logged as separate `request_logs` rows, so the
dashboard shows exactly what happened.

Once the primary's breaker is **open**, failover is essentially instant — the
gateway skips the dead provider entirely rather than waiting on it to time out.

> **Streaming** requests (`stream: true`) are a passthrough and are **not**
> retried — a partially-sent SSE stream can't be safely replayed.

## Configuration

All knobs are env vars (see [.env.example](../.env.example)); defaults are
production-sane and on by default.

| Env var | Default | Meaning |
|---------|---------|---------|
| `RETRY_MAX_ATTEMPTS` | `3` | total attempts per provider (`1` = no retry) |
| `RETRY_BASE_DELAY_MS` | `200` | backoff before the first retry |
| `RETRY_MAX_DELAY_MS` | `2000` | backoff cap |
| `BREAKER_ENABLED` | `true` | master switch for circuit breaking |
| `BREAKER_THRESHOLD` | `5` | consecutive failures that open a breaker |
| `BREAKER_COOLDOWN_SECONDS` | `30` | how long it stays open before a probe |
| `BREAKER_HALFOPEN_MAX` | `1` | trial requests admitted while half-open |

## Verification

Unit tests ([breaker_test.go](../internal/resilience/breaker_test.go),
[policy_test.go](../internal/resilience/policy_test.go)) cover open-after-threshold,
success-resets, half-open recover/re-open, provider isolation, retry of each
transient class, no-retry of `4xx`, context cancellation, and breaker
short-circuiting.

**Live-verified (2026-06-16):** with OpenAI pointed at a dead address and
failover to Anthropic, requests showed `retry → fail over` for the first few,
then `circuit breaker opened` after the threshold, then instant
`circuit breaker open → fail over` (no dial). After the cooldown a half-open
probe fired and (OpenAI still dead) re-opened. The client received a valid
Claude completion on every request.
