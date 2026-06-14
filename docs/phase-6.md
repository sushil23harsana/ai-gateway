# Phase 6 — Semantic caching (stretch)

**Goal (from BUILD.md §6, the headline stretch feature):** embed each request,
cosine-match it against recent requests (pgvector), and serve near-duplicate
prompts from cache — going beyond Phase 3's exact-match cache.

**Status:** ✅ done & verified live — a paraphrase is served from cache (skipping
the provider), while a structurally-similar prompt with a *different answer* is
correctly rejected.

---

## What was built

When the exact-match cache misses, the gateway embeds the prompt, looks for the
nearest stored response within a cosine-distance threshold, and serves it if
close enough. On a miss it stores the fresh response (reusing the embedding).

Pipeline is now: **exact cache → semantic cache → provider → store (exact + semantic)**.

## Files

| File | What it does |
|------|--------------|
| [migrations/0003_semantic_cache.up.sql](../migrations/0003_semantic_cache.up.sql) | `CREATE EXTENSION vector` + `semantic_cache` table with an HNSW cosine index. |
| [internal/store/semantic.go](../internal/store/semantic.go) | `SemanticInsert` / `SemanticNearest` (pgvector `<=>` cosine search, scoped by provider+model+key, thresholded). |
| [internal/cache/embedder.go](../internal/cache/embedder.go) | OpenAI embeddings client (`text-embedding-3-small`, 1536-dim). |
| [internal/cache/semantic.go](../internal/cache/semantic.go) | Orchestrator: embed → nearest → serve/store; `PromptText()` flattens messages to embed. |
| [internal/proxy/handler.go](../internal/proxy/handler.go) | Semantic lookup after an exact miss (`X-Cache: SEMANTIC`); stores on miss reusing the lookup embedding. |
| [internal/config/config.go](../internal/config/config.go) | `SEMANTIC_CACHE_ENABLED`, `SEMANTIC_THRESHOLD`, `EMBEDDING_MODEL`. |
| [docker-compose.yml](../docker-compose.yml) | Postgres image → `pgvector/pgvector:pg16`; gateway env wired. |

Tests: [cache/semantic_test.go](../internal/cache/semantic_test.go) (threshold
hit/miss, embedding reuse, prompt flattening) and proxy
[semantic hit/miss](../internal/proxy/handler_test.go).

## How it works

1. **Embed** the flattened prompt via OpenAI `text-embedding-3-small` (1536-dim).
2. **Search** `semantic_cache` for the nearest vector (`embedding <=> $query`),
   filtered to the same provider + model (+ key, when scope = `key`).
3. **Serve** the stored response if `distance <= SEMANTIC_THRESHOLD`
   (`X-Cache: SEMANTIC`, `cache_hit=true`, cost 0, provider skipped).
4. On a **miss**, call the provider, relay, then store the response under the
   embedding already computed in step 1 (one embedding call per request).

Opt-in (`SEMANTIC_CACHE_ENABLED`, default off) and gated by the per-key cache
toggle. Streaming requests bypass it.

## Threshold calibration (the correctness story)

Semantic caching's risk is serving a *similar-but-different* prompt's answer.
Measured cosine distances (text-embedding-3-small) made the safe band obvious:

| Pair | Distance | Match at 0.25? |
|------|---------:|:--------------:|
| "capital of France" ↔ "which city is the capital of France" (paraphrase) | **0.13** | ✅ hit |
| "capital of France" ↔ "capital of **Germany**" (different answer) | **0.42** | ❌ miss |
| "capital of France" ↔ "boiling point of water" (unrelated) | **0.87** | ❌ miss |

`SEMANTIC_THRESHOLD=0.25` sits comfortably between the paraphrase (0.13) and the
dangerous lookalike (0.42), so true paraphrases hit while different-answer
prompts don't. Tune per workload; smaller = stricter.

## How to run / verify

```bash
# .env: SEMANTIC_CACHE_ENABLED=true, SEMANTIC_THRESHOLD=0.25, plus OPENAI_API_KEY
docker compose up -d --build      # pgvector + migration 0003

KEY=...    # mint via POST /admin/keys
# A: MISS (calls OpenAI, stores).  B (paraphrase): SEMANTIC (skips OpenAI).
curl -s -D - -o /dev/null -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"What is the capital of France?"}],"max_tokens":16}' | grep -i x-cache
curl -s -D - -o /dev/null -X POST localhost:8080/v1/chat/completions -H "Authorization: Bearer $KEY" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Which city is the capital of France?"}],"max_tokens":16}' | grep -i x-cache
```

Verified live: paraphrase → `X-Cache: SEMANTIC` at **0.46s** vs **1.67s** for the
miss; "capital of Germany" correctly returned `MISS` (no wrong-answer match).

> ⚠️ **Postgres image change:** Phase 6 switches the Postgres image to
> `pgvector/pgvector:pg16`. Because that image is glibc-based (vs the previous
> musl/alpine), the cleanest switch on an existing cluster is a fresh volume
> (`docker compose down -v`). A real deployment would provision pgvector from day one.

## Key decisions

- **pgvector + HNSW** over an in-memory cosine scan — real ANN, scales, and shows the production pattern.
- **Embedding reused** between lookup and store — one embeddings call per request, not two.
- **Conservative, calibrated threshold** with the France/Germany safety case documented — semantic caching's correctness risk is taken seriously, and it's opt-in + off by default.
- **Cost = 0 on a semantic hit** (the cheap embeddings call aside), like the exact cache.

## Latency note

A semantic hit (~0.46s here) is slower than an exact hit (~5ms) because it pays
for the embedding call + vector search, but far faster (and free) vs a full LLM
call. The embedding round-trip is also added to every semantic *miss* — the cost
of the feature, which is why it's opt-in.

## Remaining Phase 6 stretch (not done)

- **SSE streaming + mid-stream token accounting** (streaming is OpenAI-passthrough, logs 0 tokens).
- **Budget enforcement / alerts** on `monthly_budget_usd`.
- **Load test** (`k6`/`vegeta`) for p99 numbers in the README.
- **Anthropic live checks** (Phase 4) once `ANTHROPIC_API_KEY` is added.
