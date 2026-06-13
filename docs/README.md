# Phase docs

Per-phase notes so you can see what each phase actually did without re-reading
the whole codebase. Each doc covers: what was built, which files changed, how
the request flow works, how to run/verify it, key decisions, and what's deferred.

The full spec is in [BUILD.md](../BUILD.md).

| Phase | Doc | Summary |
|-------|-----|---------|
| 0 | [phase-0.md](phase-0.md) | Scaffold: repo layout, Docker Compose (redis + postgres), config loader, migrations, `GET /healthz`. |
| 1 | [phase-1.md](phase-1.md) | Core proxy: `POST /v1/chat/completions` → OpenAI, async `request_logs` with tokens/cost/latency. |
| 2 | _pending_ | Virtual keys + Redis token-bucket rate limiting. |
| 3 | _pending_ | Response caching. |
| 4 | _pending_ | Multi-provider (OpenAI + Anthropic) + routing/failover. |
| 5 | _pending_ | Next.js dashboard. |
| 6 | _pending_ | Stretch: semantic cache, SSE streaming token accounting, budget alerts, load test. |
