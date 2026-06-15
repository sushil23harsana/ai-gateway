# Janus — phase docs

Per-phase notes for **Janus** (the self-hosted LLM gateway) so you can see what
each phase actually did without re-reading the whole codebase. Each doc covers:
what was built, which files changed, how the request flow works, how to
run/verify it, key decisions, and what's deferred.

The full spec is in [BUILD.md](../BUILD.md). (The codebase/module still uses the
original `ai-gateway` slug; the product brand is Janus.)

| Phase | Doc | Summary |
|-------|-----|---------|
| 0 | [phase-0.md](phase-0.md) | Scaffold: repo layout, Docker Compose (redis + postgres), config loader, migrations, `GET /healthz`. |
| 1 | [phase-1.md](phase-1.md) | Core proxy: `POST /v1/chat/completions` → OpenAI, async `request_logs` with tokens/cost/latency. |
| 2 | [phase-2.md](phase-2.md) | Virtual keys (hashed) + admin API to mint them + Redis token-bucket rate limiting. |
| 3 | [phase-3.md](phase-3.md) | Exact-match response caching in Redis (per-key scope, TTL, toggle) — skips the provider on a hit. |
| 4 | [phase-4.md](phase-4.md) | `Provider` interface + native Anthropic (OpenAI⇄Messages translation) + model routing + 5xx/timeout failover. (Anthropic live check pending key.) |
| 5 | [phase-5.md](phase-5.md) | `/admin/stats/*` Go API + Redis live counter, and a Next.js 14 + Recharts dashboard that renders it. |
| 6 | [phase-6.md](phase-6.md) | **Semantic caching** (pgvector + embeddings) — serves near-duplicate prompts, rejects different-answer lookalikes. (Other stretch items: SSE tokens, budget alerts, load test.) |
| 7 | [phase-7.md](phase-7.md) | **Control plane** — key CRUD from the dashboard (create/edit/disable/delete) via token-safe write routes + write-guard, and secure-by-default self-host packaging. See also [security.md](security.md). |
