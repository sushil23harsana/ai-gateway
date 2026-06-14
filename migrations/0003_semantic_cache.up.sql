-- 0003_semantic_cache: pgvector-backed semantic response cache.
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS semantic_cache (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at   timestamptz   NOT NULL DEFAULT now(),
    api_key_id   uuid REFERENCES api_keys (id),
    provider     text          NOT NULL,
    model        text          NOT NULL,
    embedding    vector(1536)  NOT NULL, -- text-embedding-3-small dimensionality
    content_type text          NOT NULL DEFAULT 'application/json',
    body         text          NOT NULL, -- the cached unified (OpenAI-shaped) response
    tokens_in    int           NOT NULL DEFAULT 0,
    tokens_out   int           NOT NULL DEFAULT 0
);

-- Approximate nearest-neighbour over cosine distance (the <=> operator).
CREATE INDEX IF NOT EXISTS idx_semantic_cache_embedding
    ON semantic_cache USING hnsw (embedding vector_cosine_ops);

-- Scope filter (provider + model + key) applied before the vector search.
CREATE INDEX IF NOT EXISTS idx_semantic_cache_scope
    ON semantic_cache (provider, model, api_key_id);
