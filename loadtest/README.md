# Load test

A small [k6](https://k6.io/) script that measures Janus's own overhead on the
**cache-hit path** — the repeatable, zero-cost way to check the latency bar
("cache hits return in < 10 ms"). It sends one fixed prompt, so after a single
warm-up call every request is a cache hit and **no provider tokens are spent**.

## Run

```bash
# 1. bring up the stack and mint a load-test key with NO rate limit:
docker compose up -d --build
curl -s -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"loadtest","rate_limit_rpm":0}'      # rate_limit_rpm 0 = unlimited
# copy the returned "key" value

# 2. run k6 (install: https://k6.io/docs/get-started/installation/)
VK=sk-gw-xxxx k6 run loadtest/cache-hit.js
# heavier:  VUS=50 DURATION=1m VK=sk-gw-xxxx k6 run loadtest/cache-hit.js
```

k6 prints `http_req_duration` percentiles (p50/p90/p95/p99). Put the **p99** in
the README. The threshold in the script fails the run if cache-hit p99 ≥ 10 ms,
so a green run *is* the acceptance check.

> A `make loadtest` shortcut runs the same command.
